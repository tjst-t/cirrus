package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ExecOVSClient implements OVSClient using ovs-ofctl/ovs-vsctl CLI commands.
// This is the production implementation used when OVS is available on the host.
// For testing, use the mock client in test/mock/ovs/.
type ExecOVSClient struct {
	bridge string
	logger *slog.Logger
}

// NewExecOVSClient creates a new OVS client that executes CLI commands.
func NewExecOVSClient(bridge string, logger *slog.Logger) *ExecOVSClient {
	return &ExecOVSClient{bridge: bridge, logger: logger}
}

func (c *ExecOVSClient) AddFlow(table int, priority int, match string, actions string) error {
	flow := fmt.Sprintf("table=%d,priority=%d", table, priority)
	if match != "" {
		flow += "," + match
	}
	flow += ",actions=" + actions
	_, err := c.run("ovs-ofctl", "add-flow", c.bridge, flow)
	return err
}

func (c *ExecOVSClient) DeleteFlow(table int, match string) error {
	flow := fmt.Sprintf("table=%d", table)
	if match != "" {
		flow += "," + match
	}
	_, err := c.run("ovs-ofctl", "del-flows", c.bridge, flow)
	return err
}

func (c *ExecOVSClient) AddFlowBundle(flows []FlowEntry) error {
	for _, f := range flows {
		if err := c.AddFlow(f.Table, f.Priority, f.Match, f.Actions); err != nil {
			return fmt.Errorf("bundle add flow (table=%d): %w", f.Table, err)
		}
	}
	return nil
}

func (c *ExecOVSClient) DeleteFlowBundle(flows []FlowEntry) error {
	for _, f := range flows {
		if err := c.DeleteFlow(f.Table, f.Match); err != nil {
			return fmt.Errorf("bundle del flow (table=%d): %w", f.Table, err)
		}
	}
	return nil
}

func (c *ExecOVSClient) AddPort(bridge string, port string) error {
	_, err := c.run("ovs-vsctl", "--may-exist", "add-port", bridge, port)
	return err
}

func (c *ExecOVSClient) DeletePort(bridge string, port string) error {
	_, err := c.run("ovs-vsctl", "--if-exists", "del-port", bridge, port)
	return err
}

func (c *ExecOVSClient) AddTunnelPort(bridge string, port string, remoteIP string, key int) error {
	// key=0 means "use flow-based VNI" (tun_id set per-flow)
	keyOpt := "flow"
	if key > 0 {
		keyOpt = strconv.Itoa(key)
	}
	_, err := c.run("ovs-vsctl", "--may-exist", "add-port", bridge, port,
		"--", "set", "interface", port,
		"type=geneve",
		fmt.Sprintf("options:remote_ip=%s", remoteIP),
		fmt.Sprintf("options:key=%s", keyOpt),
	)
	return err
}

func (c *ExecOVSClient) GetOfPort(port string) (int, error) {
	// Retry once if ofport is -1 (port not yet ready)
	for attempt := 0; attempt < 2; attempt++ {
		out, err := c.run("ovs-vsctl", "get", "Interface", port, "ofport")
		if err != nil {
			return 0, err
		}
		val := strings.TrimSpace(out)
		n, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("parse ofport %q: %w", val, err)
		}
		if n > 0 {
			return n, nil
		}
		// ofport is -1 or 0, wait briefly and retry
		if attempt == 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return 0, fmt.Errorf("ofport not ready for port %s", port)
}

func (c *ExecOVSClient) FindPortByExternalID(portID string) (string, error) {
	// Use ovs-vsctl to find Interface whose external_ids:iface-id matches portID.
	out, err := c.run("ovs-vsctl", "--format=csv", "--columns=name",
		"find", "Interface", fmt.Sprintf("external_ids:iface-id=%s", portID))
	if err != nil {
		return "", nil // not found is not an error
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name := strings.TrimSpace(line)
		if name != "" && name != "name" {
			return name, nil
		}
	}
	return "", nil
}

func (c *ExecOVSClient) GetFlows(table int) ([]FlowEntry, error) {
	out, err := c.run("ovs-ofctl", "dump-flows", c.bridge, fmt.Sprintf("table=%d", table))
	if err != nil {
		return nil, err
	}
	var flows []FlowEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NXST_FLOW") || strings.HasPrefix(line, "OFPST_FLOW") {
			continue
		}
		f, err := parseFlowLine(line)
		if err != nil {
			c.logger.Debug("skip unparseable flow line", "line", line, "error", err)
			continue
		}
		flows = append(flows, f)
	}
	return flows, nil
}

func (c *ExecOVSClient) SetInterfaceExternalIDs(port string, externalIDs map[string]string) error {
	args := []string{"set", "Interface", port}
	for k, v := range externalIDs {
		args = append(args, fmt.Sprintf("external_ids:%s=%s", k, v))
	}
	_, err := c.run("ovs-vsctl", args...)
	return err
}

func (c *ExecOVSClient) AddGroup(groupSpec string) error {
	_, err := c.run("ovs-ofctl", "-O", "OpenFlow13", "add-group", c.bridge, groupSpec)
	return err
}

func (c *ExecOVSClient) ModifyGroup(groupSpec string) error {
	_, err := c.run("ovs-ofctl", "-O", "OpenFlow13", "mod-group", c.bridge, groupSpec)
	return err
}

func (c *ExecOVSClient) DeleteGroup(groupID uint32) error {
	_, err := c.run("ovs-ofctl", "-O", "OpenFlow13", "del-groups", c.bridge,
		fmt.Sprintf("group_id=%d", groupID))
	return err
}

// run executes a command with a 10-second timeout.
func (c *ExecOVSClient) run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	c.logger.Debug("ovs exec", "cmd", name, "args", args)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w (output: %s)", name, args, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// parseFlowLine parses a single line from ovs-ofctl dump-flows output.
// Example input:
//
//	cookie=0x0, duration=1.234s, table=0, n_packets=0, n_bytes=0, priority=100,ip,in_port=1 actions=resubmit(,1)
func parseFlowLine(line string) (FlowEntry, error) {
	// Split on " actions=" to separate match part from actions
	parts := strings.SplitN(line, " actions=", 2)
	if len(parts) != 2 {
		return FlowEntry{}, fmt.Errorf("no actions= found")
	}
	matchPart := parts[0]
	actions := parts[1]

	// Parse the match part: strip metadata fields (cookie, duration, n_packets, n_bytes, idle_age, hard_age)
	// and extract table, priority, and remaining match
	var table, priority int
	var matchFields []string

	for _, field := range strings.Split(matchPart, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if strings.HasPrefix(field, "cookie=") ||
			strings.HasPrefix(field, "duration=") ||
			strings.HasPrefix(field, "n_packets=") ||
			strings.HasPrefix(field, "n_bytes=") ||
			strings.HasPrefix(field, "idle_age=") ||
			strings.HasPrefix(field, "hard_age=") {
			continue
		}
		if strings.HasPrefix(field, "table=") {
			val := strings.TrimPrefix(field, "table=")
			n, err := strconv.Atoi(val)
			if err != nil {
				return FlowEntry{}, fmt.Errorf("parse table: %w", err)
			}
			table = n
			continue
		}
		if strings.HasPrefix(field, "priority=") {
			val := strings.TrimPrefix(field, "priority=")
			// priority may have additional match after it: "priority=100,ip"
			// but we've already split by comma, so this is just the number
			n, err := strconv.Atoi(val)
			if err != nil {
				return FlowEntry{}, fmt.Errorf("parse priority: %w", err)
			}
			priority = n
			continue
		}
		matchFields = append(matchFields, field)
	}

	return FlowEntry{
		Table:    table,
		Priority: priority,
		Match:    strings.Join(matchFields, ","),
		Actions:  actions,
	}, nil
}

// IsOVSAvailable checks if OVS is running by attempting ovs-vsctl show.
func IsOVSAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ovs-vsctl", "show").Run() == nil
}
