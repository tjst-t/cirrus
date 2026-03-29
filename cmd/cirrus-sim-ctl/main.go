// Package main provides the cirrus-sim-ctl CLI tool for interacting
// with the aggregator and individual simulators.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
)

func main() {
	baseURL := os.Getenv("CIRRUS_SIM_URL")
	if baseURL == "" {
		baseURL = detectAggregatorURL()
	}
	if baseURL == "" {
		baseURL = "http://localhost:8090"
	}

	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "status":
		err = cmdStatus(baseURL)
	case "hosts":
		err = requireSub(args, func() error { return cmdHostsList(baseURL) })
	case "vms":
		err = requireSub(args, func() error { return cmdVMsList(baseURL) })
	case "backends":
		err = requireSub(args, func() error { return cmdBackendsList(baseURL) })
	case "volumes":
		err = requireSub(args, func() error { return cmdVolumesList(baseURL) })
	case "fault":
		err = cmdFault(baseURL, args[1:])
	case "snapshot":
		err = cmdSnapshot(baseURL, args[1:])
	case "reset":
		err = cmdReset(baseURL)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func requireSub(args []string, fn func() error) error {
	if len(args) < 2 || args[1] != "list" {
		return fmt.Errorf("usage: cirrus-sim-ctl %s list", args[0])
	}
	return fn()
}

func printUsage() {
	fmt.Println(`cirrus-sim-ctl — Cirrus Simulator Control

Usage:
  cirrus-sim-ctl status                      Show overview
  cirrus-sim-ctl hosts list                  List all hosts
  cirrus-sim-ctl vms list                    List all VMs (domains)
  cirrus-sim-ctl backends list               List storage backends
  cirrus-sim-ctl volumes list                List storage volumes
  cirrus-sim-ctl fault list                  List fault injection rules
  cirrus-sim-ctl fault inject [JSON]         Inject a fault rule
  cirrus-sim-ctl fault clear                 Clear all fault rules
  cirrus-sim-ctl snapshot list               List snapshots
  cirrus-sim-ctl snapshot save               Save state snapshot
  cirrus-sim-ctl snapshot restore <id>       Restore state snapshot
  cirrus-sim-ctl reset                       Reset all simulator state

Fault inject JSON example:
  cirrus-sim-ctl fault inject '{"target":{"simulator":"storage-sim"},"trigger":{"type":"probabilistic","probability":0.5},"fault":{"type":"error","error_code":500,"error_message":"injected"}}'

Environment:
  CIRRUS_SIM_URL   Base URL of aggregator (default: auto-detect from portman)`)
}

// ── status ──

func cmdStatus(base string) error {
	var ov map[string]any
	if err := getJSON(base+"/sim/overview", &ov); err != nil {
		return fmt.Errorf("fetch overview: %w", err)
	}

	fmt.Println("Cirrus-Sim Status")
	fmt.Println("─────────────────────────────")
	fmt.Printf("  Workers:     %.0f\n", ov["workers"])
	fmt.Printf("  Hosts:       %.0f (online: %.0f)\n", ov["total_hosts"], ov["online_hosts"])
	fmt.Printf("  Domains:     %.0f (running: %.0f)\n", ov["total_domains"], ov["running_domains"])
	fmt.Printf("  vCPUs Used:  %.0f\n", ov["total_vcpus_used"])
	memMB, _ := ov["total_memory_used_mb"].(float64)
	fmt.Printf("  Memory Used: %.1f GB\n", memMB/1024)
	return nil
}

// ── hosts list ──

func cmdHostsList(base string) error {
	var hosts []map[string]any
	if err := getJSON(base+"/sim/hosts", &hosts); err != nil {
		return fmt.Errorf("fetch hosts: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "HOST ID\tSTATE\tvCPU\tMEMORY\tVMs")
	for _, h := range hosts {
		sockets, _ := h["cpu_sockets"].(float64)
		cores, _ := h["cores_per_socket"].(float64)
		threads, _ := h["threads_per_core"].(float64)
		vcpu := int(sockets * cores * threads)
		memMB, _ := h["memory_mb"].(float64)
		state := "online"
		if s, ok := h["state"].(string); ok && s != "" {
			state = s
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%.0f GB\t%.0f\n",
			h["host_id"], state, vcpu, memMB/1024, h["domain_count"])
	}
	w.Flush()
	return nil
}

// ── vms list ──

func cmdVMsList(base string) error {
	var domains []map[string]any
	if err := getJSON(base+"/sim/domains", &domains); err != nil {
		return fmt.Errorf("fetch domains: %w", err)
	}
	if len(domains) == 0 {
		fmt.Println("No VMs.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tUUID\tSTATE\tvCPU\tMEMORY\tHOST")
	for _, d := range domains {
		stateNum, _ := d["state"].(float64)
		state := domState(int(stateNum))
		memKiB, _ := d["memory_kib"].(float64)
		uuid := truncUUID(d["uuid"])
		fmt.Fprintf(w, "%s\t%s\t%s\t%.0f\t%.0f MB\t%s\n",
			d["name"], uuid, state, d["vcpus"], memKiB/1024, d["host_id"])
	}
	w.Flush()
	return nil
}

// ── backends list ──

func cmdBackendsList(base string) error {
	var backends []map[string]any
	if err := getJSON(base+"/sim/storage/backends", &backends); err != nil {
		return fmt.Errorf("fetch backends: %w", err)
	}
	if len(backends) == 0 {
		fmt.Println("No storage backends.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BACKEND ID\tSTATE\tCAPACITY\tUSED\tVOLUMES")
	for _, b := range backends {
		totalGB, _ := b["total_capacity_gb"].(float64)
		usedGB, _ := b["used_capacity_gb"].(float64)
		volCount, _ := b["volume_count"].(float64)
		state, _ := b["state"].(string)
		fmt.Fprintf(w, "%s\t%s\t%.0f GB\t%.0f GB\t%.0f\n",
			b["backend_id"], state, totalGB, usedGB, volCount)
	}
	w.Flush()
	return nil
}

// ── volumes list ──

func cmdVolumesList(base string) error {
	// Volumes are accessible via storage-sim API directly
	var stats map[string]any
	if err := getJSON(base+"/sim/storage/stats", &stats); err != nil {
		return fmt.Errorf("fetch storage stats: %w", err)
	}

	volCount, _ := stats["volume_count"].(float64)
	exportCount, _ := stats["export_count"].(float64)
	backendCount, _ := stats["backend_count"].(float64)

	fmt.Println("Storage Volume Summary")
	fmt.Println("─────────────────────────────")
	fmt.Printf("  Backends: %.0f\n", backendCount)
	fmt.Printf("  Volumes:  %.0f\n", volCount)
	fmt.Printf("  Exports:  %.0f\n", exportCount)
	return nil
}

// ── fault ──

func cmdFault(base string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cirrus-sim-ctl fault [list|inject|clear]")
	}
	switch args[0] {
	case "list":
		return cmdFaultList(base)
	case "inject":
		if len(args) < 2 {
			return fmt.Errorf("usage: cirrus-sim-ctl fault inject '<json>'")
		}
		return cmdFaultInject(base, args[1])
	case "clear":
		return cmdFaultClear(base)
	default:
		return fmt.Errorf("unknown fault subcommand: %s", args[0])
	}
}

func cmdFaultList(base string) error {
	var faults []map[string]any
	if err := getJSON(base+"/sim/faults", &faults); err != nil {
		return fmt.Errorf("fetch faults: %w", err)
	}
	if len(faults) == 0 {
		fmt.Println("No active fault rules.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSIMULATOR\tOPERATION\tTYPE\tHITS")
	for _, f := range faults {
		target, _ := f["target"].(map[string]any)
		fault, _ := f["fault"].(map[string]any)
		sim, _ := target["simulator"].(string)
		op, _ := target["operation"].(string)
		if sim == "" {
			sim = "*"
		}
		if op == "" {
			op = "*"
		}
		ftype, _ := fault["type"].(string)
		hits, _ := f["hit_count"].(float64)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.0f\n", f["id"], sim, op, ftype, hits)
	}
	w.Flush()
	return nil
}

func cmdFaultInject(base string, jsonStr string) error {
	resp, err := http.Post(base+"/sim/faults", "application/json", bytes.NewBufferString(jsonStr))
	if err != nil {
		return fmt.Errorf("inject fault: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("inject fault: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	json.Unmarshal(body, &result)
	fmt.Printf("Fault rule created: %s\n", result["id"])
	return nil
}

func cmdFaultClear(base string) error {
	req, err := http.NewRequest(http.MethodDelete, base+"/sim/faults", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("clear faults: %w", err)
	}
	resp.Body.Close()
	fmt.Println("All fault rules cleared.")
	return nil
}

// ── snapshot ──

func cmdSnapshot(base string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cirrus-sim-ctl snapshot [list|save|restore <id>]")
	}
	switch args[0] {
	case "list":
		return cmdSnapshotList(base)
	case "save":
		return cmdSnapshotSave(base)
	case "restore":
		if len(args) < 2 {
			return fmt.Errorf("usage: cirrus-sim-ctl snapshot restore <id>")
		}
		return cmdSnapshotRestore(base, args[1])
	default:
		return fmt.Errorf("unknown snapshot subcommand: %s", args[0])
	}
}

func cmdSnapshotList(base string) error {
	var snapshots []map[string]any
	if err := getJSON(base+"/sim/snapshots", &snapshots); err != nil {
		return fmt.Errorf("fetch snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		fmt.Println("No snapshots.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCREATED AT")
	for _, s := range snapshots {
		fmt.Fprintf(w, "%s\t%s\n", s["id"], s["created_at"])
	}
	w.Flush()
	return nil
}

func cmdSnapshotSave(base string) error {
	resp, err := http.Post(base+"/sim/snapshots", "application/json", nil)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("save snapshot: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	json.Unmarshal(body, &result)
	fmt.Printf("Snapshot saved: %s\n", result["snapshot_id"])
	return nil
}

func cmdSnapshotRestore(base string, id string) error {
	resp, err := http.Post(base+"/sim/snapshots/"+id+"/restore", "application/json", nil)
	if err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("restore snapshot: HTTP %d: %s", resp.StatusCode, string(body))
	}
	fmt.Printf("Snapshot %s restored.\n", id)
	return nil
}

// ── reset ──

func cmdReset(base string) error {
	// Reset each simulator via aggregator
	fmt.Println("Resetting all simulators...")

	endpoints := []string{
		base + "/sim/storage/stats", // just check reachability
	}
	_ = endpoints

	// TODO: The aggregator should expose a /sim/reset endpoint
	// that resets all connected simulators. For now, print instructions.
	fmt.Println("Use individual simulator /sim/reset endpoints or restart with 'make fresh'.")
	return nil
}

// ── helpers ──

func getJSON(url string, out any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func detectAggregatorURL() string {
	envFile := "/tmp/cirrus-dev/portman.env"
	data, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "SIM_AGGREGATOR_PORT=") {
			port := strings.TrimPrefix(line, "SIM_AGGREGATOR_PORT=")
			port = strings.TrimSpace(port)
			if port != "" {
				return "http://localhost:" + port
			}
		}
	}
	return ""
}

func domState(s int) string {
	switch s {
	case 1:
		return "running"
	case 3:
		return "paused"
	case 5:
		return "shutoff"
	default:
		return "unknown"
	}
}

func truncUUID(v any) string {
	s, _ := v.(string)
	if len(s) > 8 {
		return s[:8] + "..."
	}
	return s
}
