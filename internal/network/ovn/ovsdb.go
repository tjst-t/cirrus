package ovn

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// OVSDBClient implements the Client interface using the OVSDB JSON-RPC protocol.
type OVSDBClient struct {
	conn   net.Conn
	mu     sync.Mutex
	reader *bufio.Reader
	nextID atomic.Int64
}

// Dial connects to an OVN Northbound DB.
// addr format: "tcp:host:port" or "tcp://host:port"
func Dial(ctx context.Context, addr string) (*OVSDBClient, error) {
	tcpAddr := strings.TrimPrefix(addr, "tcp://")
	tcpAddr = strings.TrimPrefix(tcpAddr, "tcp:")

	var d net.Dialer
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := d.DialContext(dialCtx, "tcp", tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("ovn: dial %s: %w", tcpAddr, err)
	}

	return &OVSDBClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

func (c *OVSDBClient) Close() error {
	return c.conn.Close()
}

// jsonRPCRequest is a JSON-RPC 1.0 request.
type jsonRPCRequest struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
	ID     int64  `json:"id"`
}

// jsonRPCResponse is a JSON-RPC 1.0 response.
type jsonRPCResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

func (c *OVSDBClient) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID.Add(1)
	req := jsonRPCRequest{Method: method, Params: params, ID: id}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ovn: marshal request: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	c.conn.SetDeadline(deadline)

	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("ovn: write: %w", err)
	}

	// Read response (newline-delimited JSON)
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		// Some OVSDB servers don't add newline; try reading until valid JSON
		return nil, fmt.Errorf("ovn: read response: %w", err)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("ovn: unmarshal response: %w (raw: %s)", err, string(line))
	}

	if len(resp.Error) > 0 && string(resp.Error) != "null" {
		return nil, fmt.Errorf("ovn: rpc error: %s", string(resp.Error))
	}

	return resp.Result, nil
}

// transact sends an OVSDB transact request and returns the result rows.
func (c *OVSDBClient) transact(ctx context.Context, ops ...any) ([]json.RawMessage, error) {
	params := make([]any, 0, 1+len(ops))
	params = append(params, "OVN_Northbound")
	params = append(params, ops...)

	raw, err := c.call(ctx, "transact", params)
	if err != nil {
		return nil, err
	}

	var results []json.RawMessage
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, fmt.Errorf("ovn: unmarshal transact result: %w", err)
	}

	// Check for operation errors
	for i, r := range results {
		var opResult map[string]any
		if err := json.Unmarshal(r, &opResult); err == nil {
			if errMsg, ok := opResult["error"]; ok {
				return results, fmt.Errorf("ovn: transact op %d error: %v", i, errMsg)
			}
		}
	}

	return results, nil
}

// ovsdbMap builds an OVSDB map value from a Go map.
func ovsdbMap(m map[string]string) any {
	pairs := make([]any, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, []any{k, v})
	}
	return []any{"map", pairs}
}

// ovsdbSet builds an OVSDB set value from a Go slice.
func ovsdbSet(items []any) any {
	if len(items) == 1 {
		return items[0]
	}
	return []any{"set", items}
}

func (c *OVSDBClient) CreateLogicalSwitch(ctx context.Context, name string) error {
	op := map[string]any{
		"op":    "insert",
		"table": "Logical_Switch",
		"row": map[string]any{
			"name": name,
		},
	}
	_, err := c.transact(ctx, op)
	return err
}

func (c *OVSDBClient) DeleteLogicalSwitch(ctx context.Context, name string) error {
	op := map[string]any{
		"op":    "delete",
		"table": "Logical_Switch",
		"where": []any{[]any{"name", "==", name}},
	}
	_, err := c.transact(ctx, op)
	return err
}

func (c *OVSDBClient) ListLogicalSwitches(ctx context.Context) ([]string, error) {
	op := map[string]any{
		"op":      "select",
		"table":   "Logical_Switch",
		"where":   []any{},
		"columns": []string{"name"},
	}
	results, err := c.transact(ctx, op)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	var selectResult struct {
		Rows []struct {
			Name string `json:"name"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(results[0], &selectResult); err != nil {
		return nil, fmt.Errorf("ovn: unmarshal switches: %w", err)
	}

	names := make([]string, len(selectResult.Rows))
	for i, row := range selectResult.Rows {
		names[i] = row.Name
	}
	return names, nil
}

func (c *OVSDBClient) CreateLogicalSwitchPort(ctx context.Context, switchName string, port LogicalSwitchPort) error {
	address := fmt.Sprintf("%s %s", port.MACAddress, port.IPAddress)

	var portSecurity any
	if len(port.PortSecurity) > 0 {
		items := make([]any, len(port.PortSecurity))
		for i, ps := range port.PortSecurity {
			items[i] = ps
		}
		portSecurity = ovsdbSet(items)
	} else {
		portSecurity = ovsdbSet([]any{address})
	}

	// Insert LSP
	insertLSP := map[string]any{
		"op":    "insert",
		"table": "Logical_Switch_Port",
		"row": map[string]any{
			"name":          port.Name,
			"addresses":     address,
			"port_security": portSecurity,
		},
		"uuid-name": "new_lsp",
	}

	// Mutate LS to add the port reference
	mutateLS := map[string]any{
		"op":    "mutate",
		"table": "Logical_Switch",
		"where": []any{[]any{"name", "==", switchName}},
		"mutations": []any{
			[]any{"ports", "insert", []any{"named-uuid", "new_lsp"}},
		},
	}

	_, err := c.transact(ctx, insertLSP, mutateLS)
	return err
}

func (c *OVSDBClient) DeleteLogicalSwitchPort(ctx context.Context, portName string) error {
	op := map[string]any{
		"op":    "delete",
		"table": "Logical_Switch_Port",
		"where": []any{[]any{"name", "==", portName}},
	}
	_, err := c.transact(ctx, op)
	return err
}

func (c *OVSDBClient) ListAllLogicalSwitchPorts(ctx context.Context) ([]string, error) {
	op := map[string]any{
		"op":      "select",
		"table":   "Logical_Switch_Port",
		"where":   []any{},
		"columns": []string{"name"},
	}
	results, err := c.transact(ctx, op)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	var selectResult struct {
		Rows []struct {
			Name string `json:"name"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(results[0], &selectResult); err != nil {
		return nil, fmt.Errorf("ovn: unmarshal ports: %w", err)
	}

	names := make([]string, len(selectResult.Rows))
	for i, row := range selectResult.Rows {
		names[i] = row.Name
	}
	return names, nil
}

func (c *OVSDBClient) CreateDHCPOptions(ctx context.Context, opts DHCPOptions) (string, error) {
	externalIDs := map[string]string{
		"subnet_id": opts.ExternalID,
	}

	op := map[string]any{
		"op":    "insert",
		"table": "DHCP_Options",
		"row": map[string]any{
			"cidr":        opts.CIDR,
			"options":     ovsdbMap(opts.Options),
			"external_ids": ovsdbMap(externalIDs),
		},
		"uuid-name": "new_dhcp",
	}

	results, err := c.transact(ctx, op)
	if err != nil {
		return "", err
	}

	if len(results) > 0 {
		var insertResult struct {
			UUID []any `json:"uuid"`
		}
		if err := json.Unmarshal(results[0], &insertResult); err == nil && len(insertResult.UUID) == 2 {
			if uuidStr, ok := insertResult.UUID[1].(string); ok {
				return uuidStr, nil
			}
		}
	}

	return "", nil
}

func (c *OVSDBClient) DeleteDHCPOptions(ctx context.Context, externalID string) error {
	op := map[string]any{
		"op":    "delete",
		"table": "DHCP_Options",
		"where": []any{
			[]any{"external_ids", "includes", ovsdbMap(map[string]string{"subnet_id": externalID})},
		},
	}
	_, err := c.transact(ctx, op)
	return err
}
