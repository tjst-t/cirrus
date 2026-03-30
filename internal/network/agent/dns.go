package agent

import (
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/miekg/dns"
)

// DNSServer responds to DNS queries from VMs with Network-scoped records.
type DNSServer struct {
	cache          *StateCache
	logger         *slog.Logger
	forwardAddr    string // upstream DNS for external queries (e.g. "8.8.8.8:53")
	server         *dns.Server
}

// NewDNSServer creates a new DNS server.
func NewDNSServer(cache *StateCache, logger *slog.Logger, forwardAddr string) *DNSServer {
	if forwardAddr == "" {
		forwardAddr = "8.8.8.8:53"
	}
	return &DNSServer{
		cache:       cache,
		logger:      logger,
		forwardAddr: forwardAddr,
	}
}

// ListenAndServe starts the DNS server on the given address (e.g. "0.0.0.0:53").
func (s *DNSServer) ListenAndServe(addr string) error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleQuery)

	s.server = &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: mux,
	}
	return s.server.ListenAndServe()
}

// Close stops the DNS server.
func (s *DNSServer) Close() {
	if s.server != nil {
		_ = s.server.Shutdown()
	}
}

// handleQuery processes a DNS query.
func (s *DNSServer) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		return
	}

	// Determine source network from client IP
	clientIP := extractClientIP(w.RemoteAddr())
	port := s.cache.GetPortByIP(clientIP)
	if port == nil {
		// Unknown source — forward to upstream
		s.forwardQuery(w, r)
		return
	}

	networkID := port.NetworkId
	q := r.Question[0]

	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	switch q.Qtype {
	case dns.TypeA:
		answers := s.resolveA(q.Name, networkID)
		if len(answers) > 0 {
			msg.Answer = answers
			if err := w.WriteMsg(msg); err != nil {
				s.logger.Debug("dns: write response failed", "error", err)
			}
			return
		}
	case dns.TypePTR:
		answers := s.resolvePTR(q.Name, networkID)
		if len(answers) > 0 {
			msg.Answer = answers
			if err := w.WriteMsg(msg); err != nil {
				s.logger.Debug("dns: write response failed", "error", err)
			}
			return
		}
	}

	// No internal match — forward to upstream
	s.forwardQuery(w, r)
}

// resolveA looks up A records for the given name within the given network.
func (s *DNSServer) resolveA(name, networkID string) []dns.RR {
	fqdn := strings.ToLower(strings.TrimSuffix(name, "."))
	records := s.cache.GetDNSRecordsForNetwork(networkID)

	var answers []dns.RR
	for _, rec := range records {
		recName := strings.ToLower(rec.Name)
		if recName == fqdn {
			answers = append(answers, &dns.A{
				Hdr: dns.RR_Header{
					Name:   name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP(rec.Ip),
			})
		}
	}

	// Also check group-level records: group.network.internal → all VMs in group
	// Name format: {group}.{network}.internal
	// Record format: {vm}.{group}.{network}.internal
	// If query is "web.prod.internal", match all records ending in ".web.prod.internal"
	if len(answers) == 0 {
		suffix := "." + fqdn
		for _, rec := range records {
			recName := strings.ToLower(rec.Name)
			if strings.HasSuffix(recName, suffix) {
				answers = append(answers, &dns.A{
					Hdr: dns.RR_Header{
						Name:   name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: net.ParseIP(rec.Ip),
				})
			}
		}
	}

	return answers
}

// resolvePTR handles reverse DNS lookups.
func (s *DNSServer) resolvePTR(name, networkID string) []dns.RR {
	// Convert PTR name to IP: "1.0.64.100.in-addr.arpa." → "100.64.0.1"
	ip := ptrToIP(name)
	if ip == "" {
		return nil
	}

	records := s.cache.GetDNSRecordsForNetwork(networkID)
	for _, rec := range records {
		if rec.Ip == ip {
			return []dns.RR{
				&dns.PTR{
					Hdr: dns.RR_Header{
						Name:   name,
						Rrtype: dns.TypePTR,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					Ptr: rec.Name + ".",
				},
			}
		}
	}
	return nil
}

// forwardQuery sends the query to the upstream DNS resolver.
func (s *DNSServer) forwardQuery(w dns.ResponseWriter, r *dns.Msg) {
	client := &dns.Client{Net: "udp"}
	resp, _, err := client.Exchange(r, s.forwardAddr)
	if err != nil {
		s.logger.Debug("dns: forward failed", "error", err)
		msg := new(dns.Msg)
		msg.SetRcode(r, dns.RcodeServerFailure)
		_ = w.WriteMsg(msg)
		return
	}
	_ = w.WriteMsg(resp)
}

// extractClientIP gets the IP address from the remote address.
func extractClientIP(addr net.Addr) string {
	switch a := addr.(type) {
	case *net.UDPAddr:
		return a.IP.String()
	case *net.TCPAddr:
		return a.IP.String()
	default:
		host, _, _ := net.SplitHostPort(addr.String())
		return host
	}
}

// ptrToIP converts a PTR query name to an IP address string.
// "1.0.64.100.in-addr.arpa." → "100.64.0.1"
func ptrToIP(name string) string {
	name = strings.TrimSuffix(name, ".")
	suffix := ".in-addr.arpa"
	if !strings.HasSuffix(strings.ToLower(name), suffix) {
		return ""
	}
	name = name[:len(name)-len(suffix)]
	parts := strings.Split(name, ".")
	if len(parts) != 4 {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s.%s", parts[3], parts[2], parts[1], parts[0])
}

// ResolveA is exported for testing.
func (s *DNSServer) ResolveA(name, networkID string) []dns.RR {
	return s.resolveA(name, networkID)
}

// ResolvePTR is exported for testing.
func (s *DNSServer) ResolvePTR(name, networkID string) []dns.RR {
	return s.resolvePTR(name, networkID)
}
