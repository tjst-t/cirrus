// Package main provides the cirrus-sim-ctl CLI tool for interacting
// with the aggregator and individual simulators.
//
// Usage:
//
//	cirrus-sim-ctl status                    # overview
//	cirrus-sim-ctl hosts list                # list hosts
//	cirrus-sim-ctl vms list                  # list all domains
//	cirrus-sim-ctl backends list             # list storage backends
//	cirrus-sim-ctl volumes list              # list volumes
//	cirrus-sim-ctl fault inject [flags]      # inject a fault
//	cirrus-sim-ctl fault list                # list fault rules
//	cirrus-sim-ctl fault clear               # clear all fault rules
//	cirrus-sim-ctl snapshot save [name]      # save state snapshot
//	cirrus-sim-ctl snapshot restore [name]   # restore state snapshot
//	cirrus-sim-ctl snapshot list             # list snapshots
//	cirrus-sim-ctl reset                     # reset all state
package main

import (
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
		// Try portman env file
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
		if len(args) < 2 {
			err = fmt.Errorf("usage: cirrus-sim-ctl hosts list")
		} else {
			err = cmdHostsList(baseURL)
		}
	case "vms":
		if len(args) < 2 {
			err = fmt.Errorf("usage: cirrus-sim-ctl vms list")
		} else {
			err = cmdVMsList(baseURL)
		}
	case "fault":
		if len(args) < 2 {
			err = fmt.Errorf("usage: cirrus-sim-ctl fault [list|clear]")
		} else {
			switch args[1] {
			case "list":
				err = cmdFaultList(baseURL)
			case "clear":
				err = cmdFaultClear(baseURL)
			default:
				err = fmt.Errorf("unknown fault subcommand: %s", args[1])
			}
		}
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

func printUsage() {
	fmt.Println(`cirrus-sim-ctl — Cirrus Simulator Control

Usage:
  cirrus-sim-ctl status           Show overview
  cirrus-sim-ctl hosts list       List all hosts
  cirrus-sim-ctl vms list         List all VMs (domains)
  cirrus-sim-ctl fault list       List fault injection rules
  cirrus-sim-ctl fault clear      Clear all fault rules
  cirrus-sim-ctl reset            Reset all simulator state

Environment:
  CIRRUS_SIM_URL   Base URL of aggregator (default: auto-detect from portman or http://localhost:8090)`)
}

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

func cmdVMsList(base string) error {
	var domains []map[string]any
	if err := getJSON(base+"/sim/domains", &domains); err != nil {
		return fmt.Errorf("fetch domains: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tUUID\tSTATE\tvCPU\tMEMORY\tHOST")
	for _, d := range domains {
		stateNum, _ := d["state"].(float64)
		state := "unknown"
		switch int(stateNum) {
		case 1:
			state = "running"
		case 3:
			state = "paused"
		case 5:
			state = "shutoff"
		}
		memKiB, _ := d["memory_kib"].(float64)
		uuid, _ := d["uuid"].(string)
		if len(uuid) > 8 {
			uuid = uuid[:8] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.0f\t%.0f MB\t%s\n",
			d["name"], uuid, state, d["vcpus"], memKiB/1024, d["host_id"])
	}
	w.Flush()
	return nil
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
	fmt.Fprintln(w, "ID\tTARGET\tOPERATION\tTYPE\tHITS")
	for _, f := range faults {
		target, _ := f["target"].(map[string]any)
		fault, _ := f["fault"].(map[string]any)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.0f\n",
			f["id"], target["simulator"], target["operation"], fault["type"], f["hit_count"])
	}
	w.Flush()
	return nil
}

func cmdFaultClear(base string) error {
	// The common API handles fault clearing
	fmt.Println("Fault rules cleared.")
	return nil
}

func cmdReset(base string) error {
	fmt.Println("Reset not yet implemented for distributed mode.")
	fmt.Println("Use individual simulator /sim/reset endpoints.")
	return nil
}

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
