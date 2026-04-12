package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const libvirtSchemaSQL = `
CREATE TABLE IF NOT EXISTS libvirt_sim_hosts (
	host_id              TEXT    PRIMARY KEY,
	libvirt_port         INTEGER NOT NULL,
	cpu_model            TEXT    NOT NULL DEFAULT '',
	cpu_sockets          INTEGER NOT NULL DEFAULT 1,
	cores_per_socket     INTEGER NOT NULL DEFAULT 1,
	threads_per_core     INTEGER NOT NULL DEFAULT 1,
	memory_mb            BIGINT  NOT NULL DEFAULT 0,
	cpu_overcommit_ratio FLOAT   NOT NULL DEFAULT 4.0,
	mem_overcommit_ratio FLOAT   NOT NULL DEFAULT 1.5,
	state                TEXT    NOT NULL DEFAULT 'online'
);

CREATE TABLE IF NOT EXISTS libvirt_sim_domains (
	uuid             TEXT        PRIMARY KEY,
	host_id          TEXT        NOT NULL,
	name             TEXT        NOT NULL,
	domain_id        INTEGER     NOT NULL DEFAULT -1,
	state            INTEGER     NOT NULL DEFAULT 5,
	vcpus            INTEGER     NOT NULL DEFAULT 1,
	memory_kib       BIGINT      NOT NULL DEFAULT 0,
	xml              TEXT        NOT NULL DEFAULT '',
	interface_ids    JSONB       NOT NULL DEFAULT '[]',
	migration_state  INTEGER     NOT NULL DEFAULT 0,
	migration_cookie TEXT        NOT NULL DEFAULT '',
	created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
	started_at       TIMESTAMPTZ
);
`

// SetupSchema creates the libvirt sim tables if they don't exist.
func SetupSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, libvirtSchemaSQL); err != nil {
		return fmt.Errorf("libvirt sim: setup schema: %w", err)
	}
	return nil
}

// SetDB attaches a postgres pool for persistence. Must be called before LoadFromDB.
func (s *Store) SetDB(pool *pgxpool.Pool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = pool
}

// LoadFromDB loads hosts and domains from the database into the in-memory maps.
// After calling this, callers should restart RPC listeners for each restored host
// by calling ListHosts() and starting a listener per host.
func (s *Store) LoadFromDB(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}

	// ── Load hosts ─────────────────────────────────────────────────────────
	rows, err := s.db.Query(ctx, `
		SELECT host_id, libvirt_port, cpu_model, cpu_sockets, cores_per_socket,
		       threads_per_core, memory_mb, cpu_overcommit_ratio, mem_overcommit_ratio, state
		FROM libvirt_sim_hosts`)
	if err != nil {
		return fmt.Errorf("load hosts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var h Host
		var st string
		if err := rows.Scan(
			&h.HostID, &h.LibvirtPort, &h.CPUModel, &h.CPUSockets, &h.CoresPerSocket,
			&h.ThreadsPerCore, &h.MemoryMB, &h.CPUOvercommitRatio, &h.MemOvercommitRatio, &st,
		); err != nil {
			return fmt.Errorf("scan host: %w", err)
		}
		h.State = HostState(st)
		h.Domains = make(map[string]*Domain)
		stored := h
		s.hosts[h.HostID] = &stored
		s.ports[h.LibvirtPort] = h.HostID
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load hosts rows: %w", err)
	}
	rows.Close()

	// ── Load domains ────────────────────────────────────────────────────────
	rows, err = s.db.Query(ctx, `
		SELECT uuid, host_id, name, domain_id, state, vcpus, memory_kib, xml,
		       interface_ids, migration_state, migration_cookie, created_at, started_at
		FROM libvirt_sim_domains`)
	if err != nil {
		return fmt.Errorf("load domains: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var d Domain
		var uuidStr, hostID string
		var ifaceJSON []byte
		var startedAt *time.Time
		var domainState, migState int32

		if err := rows.Scan(
			&uuidStr, &hostID, &d.Name, &d.ID, &domainState, &d.VCPUs, &d.MemoryKiB, &d.XML,
			&ifaceJSON, &migState, &d.MigrationCookie, &d.CreatedAt, &startedAt,
		); err != nil {
			return fmt.Errorf("scan domain: %w", err)
		}
		d.State = DomainState(domainState)
		d.MigrationState = MigrationState(migState)
		if startedAt != nil {
			d.StartedAt = *startedAt
		}
		if ifaceJSON != nil {
			json.Unmarshal(ifaceJSON, &d.InterfaceIDs) //nolint:errcheck
		}
		if d.InterfaceIDs == nil {
			d.InterfaceIDs = []string{}
		}
		d.UUID = parseUUIDBytes(uuidStr)

		h, ok := s.hosts[hostID]
		if !ok {
			continue // orphan domain; skip
		}
		stored := d
		h.Domains[uuidStr] = &stored
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load domains rows: %w", err)
	}

	domainCount := 0
	for _, h := range s.hosts {
		domainCount += len(h.Domains)
	}
	slog.Info("libvirt-sim: state loaded from DB",
		"hosts", len(s.hosts),
		"domains", domainCount,
	)
	return nil
}

// parseUUIDBytes converts a UUID string "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" to [16]byte.
func parseUUIDBytes(s string) [16]byte {
	var b [16]byte
	if len(s) != 36 {
		return b
	}
	hexStr := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	for i := 0; i < 16 && i*2+1 < len(hexStr); i++ {
		hi := hexNibble(hexStr[i*2])
		lo := hexNibble(hexStr[i*2+1])
		b[i] = hi<<4 | lo
	}
	return b
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// dbSaveHost upserts a host.
func (s *Store) dbSaveHost(h *Host) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `
		INSERT INTO libvirt_sim_hosts
			(host_id, libvirt_port, cpu_model, cpu_sockets, cores_per_socket,
			 threads_per_core, memory_mb, cpu_overcommit_ratio, mem_overcommit_ratio, state)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (host_id) DO UPDATE SET
			libvirt_port         = EXCLUDED.libvirt_port,
			cpu_model            = EXCLUDED.cpu_model,
			cpu_sockets          = EXCLUDED.cpu_sockets,
			cores_per_socket     = EXCLUDED.cores_per_socket,
			threads_per_core     = EXCLUDED.threads_per_core,
			memory_mb            = EXCLUDED.memory_mb,
			cpu_overcommit_ratio = EXCLUDED.cpu_overcommit_ratio,
			mem_overcommit_ratio = EXCLUDED.mem_overcommit_ratio,
			state                = EXCLUDED.state`,
		h.HostID, h.LibvirtPort, h.CPUModel, h.CPUSockets, h.CoresPerSocket,
		h.ThreadsPerCore, h.MemoryMB, h.CPUOvercommitRatio, h.MemOvercommitRatio, string(h.State),
	)
	if err != nil {
		slog.Error("libvirt-sim: persist host", "host_id", h.HostID, "error", err)
	}
}

// dbDeleteHost removes a host and its domains from the DB.
func (s *Store) dbDeleteHost(hostID string) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.db.Exec(ctx, `DELETE FROM libvirt_sim_domains WHERE host_id = $1`, hostID) //nolint:errcheck
	if _, err := s.db.Exec(ctx, `DELETE FROM libvirt_sim_hosts WHERE host_id = $1`, hostID); err != nil {
		slog.Error("libvirt-sim: delete host from DB", "host_id", hostID, "error", err)
	}
}

// dbSaveDomain upserts a domain. hostID is the owning host.
func (s *Store) dbSaveDomain(hostID string, d *Domain) {
	if s.db == nil {
		return
	}
	ifaceJSON, _ := json.Marshal(d.InterfaceIDs)

	var startedAt *time.Time
	if !d.StartedAt.IsZero() {
		t := d.StartedAt
		startedAt = &t
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `
		INSERT INTO libvirt_sim_domains
			(uuid, host_id, name, domain_id, state, vcpus, memory_kib, xml,
			 interface_ids, migration_state, migration_cookie, created_at, started_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (uuid) DO UPDATE SET
			host_id          = EXCLUDED.host_id,
			name             = EXCLUDED.name,
			domain_id        = EXCLUDED.domain_id,
			state            = EXCLUDED.state,
			vcpus            = EXCLUDED.vcpus,
			memory_kib       = EXCLUDED.memory_kib,
			xml              = EXCLUDED.xml,
			interface_ids    = EXCLUDED.interface_ids,
			migration_state  = EXCLUDED.migration_state,
			migration_cookie = EXCLUDED.migration_cookie,
			created_at       = EXCLUDED.created_at,
			started_at       = EXCLUDED.started_at`,
		d.UUIDString(), hostID, d.Name, d.ID, int32(d.State), d.VCPUs, d.MemoryKiB, d.XML,
		ifaceJSON, int32(d.MigrationState), d.MigrationCookie, d.CreatedAt, startedAt,
	)
	if err != nil {
		slog.Error("libvirt-sim: persist domain", "uuid", d.UUIDString(), "error", err)
	}
}

// dbDeleteDomain removes a domain from the DB.
func (s *Store) dbDeleteDomain(uuid string) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `DELETE FROM libvirt_sim_domains WHERE uuid = $1`, uuid); err != nil {
		slog.Error("libvirt-sim: delete domain from DB", "uuid", uuid, "error", err)
	}
}

// dbClearAll truncates all libvirt sim tables.
func (s *Store) dbClearAll() {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `TRUNCATE libvirt_sim_hosts, libvirt_sim_domains`); err != nil {
		slog.Error("libvirt-sim: clear tables", "error", err)
	}
}
