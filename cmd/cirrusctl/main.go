package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/identity"
)

// cli holds the shared state for all commands.
type cli struct {
	endpoint string
	token    string
	output   string
}

func main() {
	app := &cli{}

	rootCmd := &cobra.Command{
		Use:   "cirrusctl",
		Short: "CLI client for the Cirrus IaaS platform",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch app.output {
			case "table", "json":
				return nil
			default:
				return fmt.Errorf("invalid output format %q: must be table or json", app.output)
			}
		},
	}

	rootCmd.PersistentFlags().StringVar(&app.endpoint, "endpoint", envOrDefault("CIRRUS_ENDPOINT", "http://localhost:8080"), "API server URL")
	rootCmd.PersistentFlags().StringVar(&app.token, "token", os.Getenv("CIRRUS_TOKEN"), "Bearer token for authentication")
	rootCmd.PersistentFlags().StringVarP(&app.output, "output", "o", "table", "Output format (table, json)")

	rootCmd.AddCommand(app.newOrgCmd())
	rootCmd.AddCommand(app.newTenantCmd())
	rootCmd.AddCommand(app.newRoleCmd())
	rootCmd.AddCommand(app.newAdminCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func (app *cli) newClient() *client.Client {
	return client.New(app.endpoint, app.token)
}

func (app *cli) cmdContext() context.Context {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)
	return ctx
}

// resolveTenant resolves a tenant identifier (UUID or name) to a UUID.
// If the identifier is not a UUID, org must be provided for name-scoped lookup.
func (app *cli) resolveTenant(ctx context.Context, c *client.Client, idOrName, org string) (uuid.UUID, error) {
	var orgID *uuid.UUID
	if org != "" {
		resolved, err := c.ResolveOrganization(ctx, org)
		if err != nil {
			return uuid.Nil, err
		}
		orgID = &resolved
	}
	return c.ResolveTenant(ctx, idOrName, orgID)
}

// --- Organization commands ---

func (app *cli) newOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage organizations",
	}
	cmd.AddCommand(app.newOrgCreateCmd())
	cmd.AddCommand(app.newOrgListCmd())
	cmd.AddCommand(app.newOrgShowCmd())
	return cmd
}

func (app *cli) newOrgCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			org, err := app.newClient().CreateOrganization(ctx, args[0])
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "CREATED"},
				[][]string{orgRow(org)},
			)
		},
	}
}

func (app *cli) newOrgListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all organizations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			orgs, err := app.newClient().ListOrganizations(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(orgs))
			for i := range orgs {
				rows[i] = orgRow(&orgs[i])
			}
			return app.printTable(
				[]string{"ID", "NAME", "CREATED"},
				rows,
			)
		},
	}
}

func (app *cli) newOrgShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show organization details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveOrganization(ctx, args[0])
			if err != nil {
				return err
			}
			org, err := c.GetOrganization(ctx, id)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "CREATED"},
				[][]string{orgRow(org)},
			)
		},
	}
}

// --- Tenant commands ---

func (app *cli) newTenantCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Manage tenants",
	}
	cmd.AddCommand(app.newTenantCreateCmd())
	cmd.AddCommand(app.newTenantListCmd())
	cmd.AddCommand(app.newTenantShowCmd())
	return cmd
}

func (app *cli) newTenantCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <org-id-or-name> <name>",
		Short: "Create a new tenant within an organization",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			orgID, err := c.ResolveOrganization(ctx, args[0])
			if err != nil {
				return err
			}
			tenant, err := c.CreateTenant(ctx, orgID, args[1])
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "ORG_ID", "NAME", "CREATED"},
				[][]string{tenantRow(tenant)},
			)
		},
	}
}

func (app *cli) newTenantListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <org-id-or-name>",
		Short: "List tenants in an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			orgID, err := c.ResolveOrganization(ctx, args[0])
			if err != nil {
				return err
			}
			tenants, err := c.ListTenants(ctx, orgID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(tenants))
			for i := range tenants {
				rows[i] = tenantRow(&tenants[i])
			}
			return app.printTable(
				[]string{"ID", "ORG_ID", "NAME", "CREATED"},
				rows,
			)
		},
	}
}

func (app *cli) newTenantShowCmd() *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show tenant details (name resolution requires --org)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, args[0], org)
			if err != nil {
				return err
			}
			tenant, err := c.GetTenant(ctx, tenantID)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "ORG_ID", "NAME", "CREATED"},
				[][]string{tenantRow(tenant)},
			)
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	return cmd
}

// --- Role commands ---

func (app *cli) newRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage role assignments",
	}
	cmd.AddCommand(app.newRoleAssignCmd())
	cmd.AddCommand(app.newRoleListCmd())
	cmd.AddCommand(app.newRoleDeleteCmd())
	return cmd
}

func (app *cli) newRoleAssignCmd() *cobra.Command {
	var org string
	var userID string
	var role string

	cmd := &cobra.Command{
		Use:   "assign <tenant-id-or-name>",
		Short: "Assign a role to a user in a tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, args[0], org)
			if err != nil {
				return err
			}
			uid, err := uuid.Parse(userID)
			if err != nil {
				return fmt.Errorf("invalid user ID: %w", err)
			}
			ra, err := c.AssignRole(ctx, tenantID, uid, identity.Role(role))
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "USER_ID", "SCOPE_TYPE", "SCOPE_ID", "ROLE", "CREATED"},
				[][]string{roleAssignmentRow(ra)},
			)
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&userID, "user-id", "", "User ID to assign the role to (required)")
	cmd.Flags().StringVar(&role, "role", "", "Role to assign (required)")
	_ = cmd.MarkFlagRequired("user-id")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}

func (app *cli) newRoleListCmd() *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "list <tenant-id-or-name>",
		Short: "List role assignments for a tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, args[0], org)
			if err != nil {
				return err
			}
			assignments, err := c.ListRoleAssignments(ctx, tenantID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(assignments))
			for i := range assignments {
				rows[i] = roleAssignmentRow(&assignments[i])
			}
			return app.printTable(
				[]string{"ID", "USER_ID", "SCOPE_TYPE", "SCOPE_ID", "ROLE", "CREATED"},
				rows,
			)
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	return cmd
}

func (app *cli) newRoleDeleteCmd() *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "delete <tenant-id-or-name> <assignment-id>",
		Short: "Delete a role assignment",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, args[0], org)
			if err != nil {
				return err
			}
			assignmentID, err := uuid.Parse(args[1])
			if err != nil {
				return fmt.Errorf("invalid assignment ID: %w", err)
			}
			if err := c.DeleteRoleAssignment(ctx, tenantID, assignmentID); err != nil {
				return err
			}
			return app.printStatus("Deleted", "role-assignment", assignmentID.String())
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	return cmd
}

// --- Admin commands ---

func (app *cli) newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Infrastructure administration commands",
	}
	cmd.AddCommand(app.newAdminHostCmd())
	return cmd
}

// --- Admin: Host commands ---

func (app *cli) newAdminHostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Manage hosts",
	}
	cmd.AddCommand(app.newHostCreateCmd())
	cmd.AddCommand(app.newHostListCmd())
	cmd.AddCommand(app.newHostShowCmd())
	cmd.AddCommand(app.newHostActivateCmd())
	cmd.AddCommand(app.newHostMaintenanceCmd())
	cmd.AddCommand(app.newHostDrainCmd())
	cmd.AddCommand(app.newHostRetireCmd())
	return cmd
}

func (app *cli) newHostCreateCmd() *cobra.Command {
	var address string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Register a new host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			h, err := app.newClient().CreateHost(ctx, nil, args[0], address)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "ADDRESS", "STATE", "LAST_HEARTBEAT"},
				[][]string{hostRow(h)},
			)
		},
	}
	cmd.Flags().StringVar(&address, "address", "", "Host address")
	return cmd
}

func (app *cli) newHostListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all hosts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			hosts, err := app.newClient().ListHosts(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(hosts))
			for i := range hosts {
				rows[i] = hostRow(&hosts[i])
			}
			return app.printTable(
				[]string{"ID", "NAME", "ADDRESS", "STATE", "LAST_HEARTBEAT"},
				rows,
			)
		},
	}
}

func (app *cli) newHostShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show host details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveHost(ctx, args[0])
			if err != nil {
				return err
			}
			h, err := c.GetHost(ctx, id)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "ADDRESS", "STATE", "LAST_HEARTBEAT"},
				[][]string{hostRow(h)},
			)
		},
	}
}

func (app *cli) newHostMaintenanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "maintenance <id-or-name>",
		Short: "Put a host into maintenance mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveHost(ctx, args[0])
			if err != nil {
				return err
			}
			h, err := c.HostAction(ctx, id, "maintenance")
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "ADDRESS", "STATE", "LAST_HEARTBEAT"},
				[][]string{hostRow(h)},
			)
		},
	}
}

func (app *cli) newHostActivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate <id-or-name>",
		Short: "Activate a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveHost(ctx, args[0])
			if err != nil {
				return err
			}
			h, err := c.HostAction(ctx, id, "activate")
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "ADDRESS", "STATE", "LAST_HEARTBEAT"},
				[][]string{hostRow(h)},
			)
		},
	}
}

func (app *cli) newHostDrainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drain <id-or-name>",
		Short: "Drain a host (prepare for maintenance)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveHost(ctx, args[0])
			if err != nil {
				return err
			}
			h, err := c.HostAction(ctx, id, "drain")
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "ADDRESS", "STATE", "LAST_HEARTBEAT"},
				[][]string{hostRow(h)},
			)
		},
	}
}

func (app *cli) newHostRetireCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retire <id-or-name>",
		Short: "Retire a host (mark for decommission)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveHost(ctx, args[0])
			if err != nil {
				return err
			}
			h, err := c.HostAction(ctx, id, "retire")
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "ADDRESS", "STATE", "LAST_HEARTBEAT"},
				[][]string{hostRow(h)},
			)
		},
	}
}

// --- Row builders ---

func hostRow(h *host.Host) []string {
	heartbeat := "-"
	if h.LastHeartbeat != nil {
		heartbeat = h.LastHeartbeat.Format("2006-01-02 15:04:05")
	}
	return []string{h.ID.String(), h.Name, h.Address, string(h.OperationalState), heartbeat}
}

func orgRow(org *identity.Organization) []string {
	return []string{org.ID.String(), org.Name, org.CreatedAt.Format("2006-01-02 15:04:05")}
}

func tenantRow(t *identity.Tenant) []string {
	return []string{t.ID.String(), t.OrganizationID.String(), t.Name, t.CreatedAt.Format("2006-01-02 15:04:05")}
}

func roleAssignmentRow(ra *identity.RoleAssignment) []string {
	scopeID := "<nil>"
	if ra.ScopeID != nil {
		scopeID = ra.ScopeID.String()
	}
	return []string{ra.ID.String(), ra.UserID.String(), string(ra.ScopeType), scopeID, string(ra.Role), ra.CreatedAt.Format("2006-01-02 15:04:05")}
}

// --- Output helpers ---

// printTable renders data as a table or JSON depending on the output flag.
// For JSON output with a single row, the item is printed directly (not wrapped in an array).
func (app *cli) printTable(headers []string, rows [][]string) error {
	if app.output == "json" {
		return app.printRowsJSON(headers, rows)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)
	for _, row := range rows {
		for i, col := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, col)
		}
		fmt.Fprintln(w)
	}
	return w.Flush()
}

func (app *cli) printRowsJSON(headers []string, rows [][]string) error {
	items := make([]map[string]string, len(rows))
	for i, row := range rows {
		item := make(map[string]string, len(headers))
		for j, h := range headers {
			if j < len(row) {
				item[h] = row[j]
			}
		}
		items[i] = item
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if len(items) == 1 {
		return enc.Encode(items[0])
	}
	return enc.Encode(items)
}

// printStatus outputs a status message for non-data operations (e.g. delete).
func (app *cli) printStatus(action, resource, id string) error {
	if app.output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{
			"status":   action,
			"resource": resource,
			"id":       id,
		})
	}
	fmt.Printf("%s %s %s\n", action, resource, id)
	return nil
}
