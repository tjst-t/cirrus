package main

// admin_drs.go — cirrusctl admin drs subcommands.
//
// Commands:
//   cirrusctl admin drs run    — trigger a DRS cycle immediately
//   cirrusctl admin drs status — show current DRS status and last run report

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tjst-t/cirrus/internal/client"
)

func (app *cli) newAdminDRSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drs",
		Short: "Manage DRS (Distributed Resource Scheduler)",
	}
	cmd.AddCommand(app.newAdminDRSRunCmd())
	cmd.AddCommand(app.newAdminDRSStatusCmd())
	return cmd
}

func (app *cli) newAdminDRSRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Trigger a DRS cycle immediately (admin only)",
		Long: `Trigger a DRS cycle immediately.

The controller evaluates all enabled availability zones and performs live
migrations to reduce resource imbalance.  The call blocks until the cycle
completes; if a cycle is already in progress, the command exits with an error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			report, err := c.DRSRun(ctx)
			if err != nil {
				return err
			}
			if app.output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			return printDRSReport(report)
		},
	}
}

func (app *cli) newAdminDRSStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current DRS configuration and last run report",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			status, err := c.DRSStatus(ctx)
			if err != nil {
				return err
			}
			if app.output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(status)
			}
			// Human-readable
			enabled := "disabled"
			if status.Enabled {
				enabled = "enabled"
			}
			fmt.Printf("DRS:      %s\n", enabled)
			fmt.Printf("Interval: %ds\n", status.IntervalSeconds)
			if status.LastReport == nil {
				fmt.Println("Last run: (none)")
				return nil
			}
			fmt.Println("\nLast run:")
			return printDRSReport(status.LastReport)
		},
	}
}

// printDRSReport prints a DRS run report in human-readable tabwriter format.
func printDRSReport(r *client.DRSRunReport) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Started:\t%s\n", r.StartedAt)
	fmt.Fprintf(w, "Finished:\t%s\n", r.FinishedAt)
	fmt.Fprintf(w, "Duration:\t%dms\n", r.DurationMs)
	fmt.Fprintf(w, "Successes:\t%d\n", r.Successes)
	fmt.Fprintf(w, "Failures:\t%d\n", r.Failures)
	if err := w.Flush(); err != nil {
		return err
	}

	if len(r.AZResults) > 0 {
		fmt.Println()
		azW := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(azW, "AZ_ID\tSTDDEV_BEFORE\tPLANNED\tEVALUATED_HOSTS")
		for _, az := range r.AZResults {
			fmt.Fprintf(azW, "%s\t%.4f\t%d\t%d\n",
				az.AZID, az.StddevBefore, az.PlannedCount, az.EvaluatedHosts,
			)
		}
		if err := azW.Flush(); err != nil {
			return err
		}
	}

	if len(r.Errors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range r.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}
	return nil
}
