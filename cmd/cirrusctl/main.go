package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/topology"
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
	rootCmd.AddCommand(app.newNetworkCmd())
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	_ = cancel // cleaned up when process exits; stored to avoid linter warning
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

// --- Network commands ---

func (app *cli) newNetworkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "network",
		Aliases: []string{"net"},
		Short:   "Manage tenant networks",
	}
	cmd.AddCommand(app.newNetworkCreateCmd())
	cmd.AddCommand(app.newNetworkListCmd())
	cmd.AddCommand(app.newNetworkShowCmd())
	cmd.AddCommand(app.newNetworkDeleteCmd())
	return cmd
}

func (app *cli) newNetworkCreateCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			n, err := c.CreateNetwork(ctx, tenantID, args[0])
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "TENANT_ID", "NAME", "STATUS", "CREATED"},
				[][]string{networkRow(n)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newNetworkListCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List networks in a tenant",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networks, err := c.ListNetworks(ctx, tenantID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(networks))
			for i := range networks {
				rows[i] = networkRow(&networks[i])
			}
			return app.printTable(
				[]string{"ID", "TENANT_ID", "NAME", "STATUS", "CREATED"},
				rows,
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newNetworkShowCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show network details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			id, err := c.ResolveNetwork(ctx, args[0], tenantID)
			if err != nil {
				return err
			}
			n, err := c.GetNetwork(ctx, id)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "TENANT_ID", "NAME", "STATUS", "CREATED"},
				[][]string{networkRow(n)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) for name resolution")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	return cmd
}

func (app *cli) newNetworkDeleteCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			id, err := c.ResolveNetwork(ctx, args[0], tenantID)
			if err != nil {
				return err
			}
			if err := c.DeleteNetwork(ctx, id); err != nil {
				return err
			}
			return app.printStatus("Deleted", "network", id.String())
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) for name resolution")
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
	cmd.AddCommand(app.newAdminStorageDomainCmd())
	cmd.AddCommand(app.newAdminLocationCmd())
	cmd.AddCommand(app.newAdminComputePoolCmd())
	cmd.AddCommand(app.newAdminFaultDomainCmd())
	cmd.AddCommand(app.newAdminAZCmd())
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
	cmd.AddCommand(app.newHostDeleteCmd())
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
	var pending bool
	var state string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all hosts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()

			filterState := state
			if pending {
				filterState = "registering"
			}

			var hosts []host.Host
			var err error
			if filterState != "" {
				hosts, err = c.ListHostsByState(ctx, filterState)
			} else {
				hosts, err = c.ListHosts(ctx)
			}
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
	cmd.Flags().BoolVar(&pending, "pending", false, "Show only hosts pending approval (registering state)")
	cmd.Flags().StringVar(&state, "state", "", "Filter by operational state")
	return cmd
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
			return app.printDetail(h, hostDetailKV(h)...)
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

func (app *cli) newHostDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a host (must be in retiring state)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveHost(ctx, args[0])
			if err != nil {
				return err
			}
			if err := c.DeleteHost(ctx, id); err != nil {
				return err
			}
			return app.printStatus("Deleted", "host", id.String())
		},
	}
}

// --- Admin: Storage Domain commands ---

func (app *cli) newAdminStorageDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "storage-domain",
		Aliases: []string{"sd"},
		Short:   "Manage storage domains",
	}
	cmd.AddCommand(app.newStorageDomainCreateCmd())
	cmd.AddCommand(app.newStorageDomainListCmd())
	cmd.AddCommand(app.newStorageDomainShowCmd())
	cmd.AddCommand(app.newStorageDomainAddHostCmd())
	cmd.AddCommand(app.newStorageDomainRemoveHostCmd())
	return cmd
}

func (app *cli) newStorageDomainCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new storage domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			d, err := app.newClient().CreateStorageDomain(ctx, args[0])
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "CREATED"},
				[][]string{storageDomainRow(d)},
			)
		},
	}
}

func (app *cli) newStorageDomainListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all storage domains",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			domains, err := app.newClient().ListStorageDomains(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(domains))
			for i := range domains {
				rows[i] = storageDomainRow(&domains[i])
			}
			return app.printTable(
				[]string{"ID", "NAME", "CREATED"},
				rows,
			)
		},
	}
}

func (app *cli) newStorageDomainShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show storage domain details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveStorageDomain(ctx, args[0])
			if err != nil {
				return err
			}
			d, err := c.GetStorageDomain(ctx, id)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "CREATED"},
				[][]string{storageDomainRow(d)},
			)
		},
	}
}

func (app *cli) newStorageDomainAddHostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-host <storage-domain> <host>",
		Short: "Associate a host with this storage domain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			sdID, err := c.ResolveStorageDomain(ctx, args[0])
			if err != nil {
				return err
			}
			hostID, err := c.ResolveHost(ctx, args[1])
			if err != nil {
				return err
			}
			if err := c.AssociateHostStorageDomain(ctx, hostID, sdID); err != nil {
				return err
			}
			return app.printStatus("Associated", "host", hostID.String())
		},
	}
}

func (app *cli) newStorageDomainRemoveHostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-host <storage-domain> <host>",
		Short: "Dissociate a host from this storage domain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			sdID, err := c.ResolveStorageDomain(ctx, args[0])
			if err != nil {
				return err
			}
			hostID, err := c.ResolveHost(ctx, args[1])
			if err != nil {
				return err
			}
			if err := c.DissociateHostStorageDomain(ctx, hostID, sdID); err != nil {
				return err
			}
			return app.printStatus("Dissociated", "host", hostID.String())
		},
	}
}

// --- Admin: Location commands ---

func (app *cli) newAdminLocationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "location",
		Aliases: []string{"loc"},
		Short:   "Manage locations",
	}
	cmd.AddCommand(app.newLocationCreateCmd())
	cmd.AddCommand(app.newLocationListCmd())
	cmd.AddCommand(app.newLocationShowCmd())
	cmd.AddCommand(app.newLocationPathCmd())
	cmd.AddCommand(app.newLocationTreeCmd())
	cmd.AddCommand(app.newLocationAddHostCmd())
	return cmd
}

func (app *cli) newLocationCreateCmd() *cobra.Command {
	var parentID string
	var locType string
	var faultAttrs string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new location",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()

			var pID *uuid.UUID
			if parentID != "" {
				resolved, err := c.ResolveLocation(ctx, parentID)
				if err != nil {
					return err
				}
				pID = &resolved
			}

			var fa []byte
			if faultAttrs != "" {
				fa = []byte(faultAttrs)
			}

			loc, err := c.CreateLocation(ctx, pID, args[0], locType, fa)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "PARENT_ID", "NAME", "TYPE", "CREATED"},
				[][]string{locationRow(loc)},
			)
		},
	}
	cmd.Flags().StringVar(&parentID, "parent", "", "Parent location (ID or name)")
	cmd.Flags().StringVar(&locType, "type", "", "Location type (site, floor, row, rack, unit) (required)")
	cmd.Flags().StringVar(&faultAttrs, "fault-attributes", "", "Fault attributes as JSON")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func (app *cli) newLocationListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all locations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			locations, err := app.newClient().ListLocations(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(locations))
			for i := range locations {
				rows[i] = locationRow(&locations[i])
			}
			return app.printTable(
				[]string{"ID", "PARENT_ID", "NAME", "TYPE", "CREATED"},
				rows,
			)
		},
	}
}

func (app *cli) newLocationShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show location details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveLocation(ctx, args[0])
			if err != nil {
				return err
			}
			loc, err := c.GetLocation(ctx, id)
			if err != nil {
				return err
			}
			return app.printDetail(loc, locationDetailKV(loc)...)
		},
	}
}

func (app *cli) newLocationPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <id-or-name>",
		Short: "Show path from root to location",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveLocation(ctx, args[0])
			if err != nil {
				return err
			}
			path, err := c.GetLocationPath(ctx, id)
			if err != nil {
				return err
			}
			rows := make([][]string, len(path))
			for i := range path {
				rows[i] = locationRow(&path[i])
			}
			return app.printTable(
				[]string{"ID", "PARENT_ID", "NAME", "TYPE", "CREATED"},
				rows,
			)
		},
	}
}

func (app *cli) newLocationTreeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tree <id-or-name>",
		Short: "Show location subtree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveLocation(ctx, args[0])
			if err != nil {
				return err
			}
			tree, err := c.GetLocationTree(ctx, id)
			if err != nil {
				return err
			}
			if app.output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(tree)
			}
			printLocationTree(tree, "")
			return nil
		},
	}
}

func printLocationTree(loc *topology.Location, indent string) {
	fmt.Printf("%s%s (%s) [%s]\n", indent, loc.Name, loc.Type, loc.ID)
	for _, child := range loc.Children {
		printLocationTree(child, indent+"  ")
	}
}

func (app *cli) newLocationAddHostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-host <location> <host>",
		Short: "Set a host's location",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			locID, err := c.ResolveLocation(ctx, args[0])
			if err != nil {
				return err
			}
			hostID, err := c.ResolveHost(ctx, args[1])
			if err != nil {
				return err
			}
			if err := c.SetHostLocation(ctx, hostID, locID); err != nil {
				return err
			}
			return app.printStatus("Associated", "host", hostID.String())
		},
	}
}

// --- Admin: Compute Pool commands ---

func (app *cli) newAdminComputePoolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "compute-pool",
		Aliases: []string{"cp"},
		Short:   "Query compute pools (derived)",
	}
	cmd.AddCommand(app.newComputePoolGetCmd())
	return cmd
}

func (app *cli) newComputePoolGetCmd() *cobra.Command {
	var sd string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get compute pool for a storage domain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			sdID, err := c.ResolveStorageDomain(ctx, sd)
			if err != nil {
				return err
			}
			pool, err := c.GetComputePool(ctx, sdID)
			if err != nil {
				return err
			}
			if app.output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(pool)
			}
			fmt.Printf("Storage Domain:  %s (%s)\n", pool.StorageDomainName, pool.StorageDomainID)
			fmt.Printf("Host Count:      %d\n", pool.Count)
			if pool.Count > 0 {
				ids := make([]string, len(pool.HostIDs))
				for i, id := range pool.HostIDs {
					ids[i] = id.String()
				}
				fmt.Printf("Host IDs:        %s\n", strings.Join(ids, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sd, "storage-domain", "", "Storage domain (ID or name) (required)")
	_ = cmd.MarkFlagRequired("storage-domain")
	return cmd
}

// --- Admin: Availability Zone commands ---

func (app *cli) newAdminAZCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "az",
		Aliases: []string{"availability-zone"},
		Short:   "Manage availability zones",
	}
	cmd.AddCommand(app.newAZCreateCmd())
	cmd.AddCommand(app.newAZListCmd())
	cmd.AddCommand(app.newAZShowCmd())
	cmd.AddCommand(app.newAZDeleteCmd())
	cmd.AddCommand(app.newAZAddSDCmd())
	cmd.AddCommand(app.newAZRemoveSDCmd())
	return cmd
}

func (app *cli) newAZCreateCmd() *cobra.Command {
	var locStr, desc string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an availability zone",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			locID, err := c.ResolveLocation(ctx, locStr)
			if err != nil {
				return err
			}
			a, err := c.CreateAZ(ctx, args[0], desc, locID)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "LOCATION_ID", "ENABLED", "CREATED"},
				[][]string{azRow(a)},
			)
		},
	}
	cmd.Flags().StringVar(&locStr, "location", "", "Location (ID or name) (required)")
	cmd.Flags().StringVar(&desc, "description", "", "Description")
	_ = cmd.MarkFlagRequired("location")
	return cmd
}

func (app *cli) newAZListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all availability zones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			azs, err := app.newClient().ListAllAZs(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(azs))
			for i := range azs {
				rows[i] = azRow(&azs[i])
			}
			return app.printTable(
				[]string{"ID", "NAME", "LOCATION_ID", "ENABLED", "CREATED"},
				rows,
			)
		},
	}
}

func (app *cli) newAZShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show availability zone details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveAZ(ctx, args[0])
			if err != nil {
				return err
			}
			a, err := c.GetAZ(ctx, id)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "LOCATION_ID", "ENABLED", "CREATED"},
				[][]string{azRow(a)},
			)
		},
	}
}

func (app *cli) newAZDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete an availability zone",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveAZ(ctx, args[0])
			if err != nil {
				return err
			}
			if err := c.DeleteAZ(ctx, id); err != nil {
				return err
			}
			return app.printStatus("Deleted", "az", id.String())
		},
	}
}

func (app *cli) newAZAddSDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-storage-domain <az> <storage-domain>",
		Short: "Associate a storage domain with an AZ",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			azID, err := c.ResolveAZ(ctx, args[0])
			if err != nil {
				return err
			}
			sdID, err := c.ResolveStorageDomain(ctx, args[1])
			if err != nil {
				return err
			}
			if err := c.AddAZStorageDomain(ctx, azID, sdID); err != nil {
				return err
			}
			return app.printStatus("Associated", "storage-domain", sdID.String())
		},
	}
}

func (app *cli) newAZRemoveSDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-storage-domain <az> <storage-domain>",
		Short: "Dissociate a storage domain from an AZ",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			azID, err := c.ResolveAZ(ctx, args[0])
			if err != nil {
				return err
			}
			sdID, err := c.ResolveStorageDomain(ctx, args[1])
			if err != nil {
				return err
			}
			if err := c.RemoveAZStorageDomain(ctx, azID, sdID); err != nil {
				return err
			}
			return app.printStatus("Dissociated", "storage-domain", sdID.String())
		},
	}
}

// --- Admin: Fault Domain commands ---

func (app *cli) newAdminFaultDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "fault-domain",
		Aliases: []string{"fd"},
		Short:   "Query fault domains (derived from location hierarchy)",
	}
	cmd.AddCommand(app.newFaultDomainListCmd())
	return cmd
}

func (app *cli) newFaultDomainListCmd() *cobra.Command {
	var level string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List fault domains at a given hierarchy level",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			fds, err := app.newClient().GetFaultDomains(ctx, level)
			if err != nil {
				return err
			}
			if app.output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(fds)
			}
			rows := make([][]string, len(fds))
			for i, fd := range fds {
				ids := make([]string, len(fd.HostIDs))
				for j, id := range fd.HostIDs {
					ids[j] = id.String()
				}
				hostStr := strings.Join(ids, ", ")
				if hostStr == "" {
					hostStr = "-"
				}
				rows[i] = []string{fd.LocationID.String(), fd.LocationName, string(fd.Level), fmt.Sprintf("%d", fd.Count), hostStr}
			}
			return app.printTable(
				[]string{"LOCATION_ID", "NAME", "LEVEL", "HOSTS", "HOST_IDS"},
				rows,
			)
		},
	}
	cmd.Flags().StringVar(&level, "level", "", "Hierarchy level (site, floor, row, rack, unit) (required)")
	_ = cmd.MarkFlagRequired("level")
	return cmd
}

// --- Row builders ---

func networkRow(n *network.Network) []string {
	return []string{n.ID.String(), n.TenantID.String(), n.Name, string(n.Status), n.CreatedAt.Format("2006-01-02 15:04:05")}
}

func azRow(a *az.AvailabilityZone) []string {
	enabled := "true"
	if !a.Enabled {
		enabled = "false"
	}
	return []string{a.ID.String(), a.Name, a.LocationID.String(), enabled, a.CreatedAt.Format("2006-01-02 15:04:05")}
}

func storageDomainRow(d *topology.StorageDomain) []string {
	return []string{d.ID.String(), d.Name, d.CreatedAt.Format("2006-01-02 15:04:05")}
}

func locationRow(l *topology.Location) []string {
	parentID := "-"
	if l.ParentID != nil {
		parentID = l.ParentID.String()
	}
	return []string{l.ID.String(), parentID, l.Name, string(l.Type), l.CreatedAt.Format("2006-01-02 15:04:05")}
}

func locationDetailKV(l *topology.Location) []string {
	parentID := "-"
	if l.ParentID != nil {
		parentID = l.ParentID.String()
	}
	fa := "-"
	if len(l.FaultAttributes) > 0 {
		fa = string(l.FaultAttributes)
	}
	return []string{
		"ID", l.ID.String(),
		"Parent ID", parentID,
		"Name", l.Name,
		"Type", string(l.Type),
		"Fault Attributes", fa,
		"Created", l.CreatedAt.Format("2006-01-02 15:04:05"),
		"Updated", l.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func hostRow(h *host.Host) []string {
	heartbeat := "-"
	if h.LastHeartbeat != nil {
		heartbeat = h.LastHeartbeat.Format("2006-01-02 15:04:05")
	}
	return []string{h.ID.String(), h.Name, h.Address, string(h.OperationalState), heartbeat}
}

func hostDetailKV(h *host.Host) []string {
	heartbeat := "-"
	if h.LastHeartbeat != nil {
		heartbeat = h.LastHeartbeat.Format("2006-01-02 15:04:05")
	}
	return []string{
		"ID", h.ID.String(),
		"Name", h.Name,
		"Address", h.Address,
		"State", string(h.OperationalState),
		"Capability", string(h.Capability),
		"Resource Physical", string(h.ResourcePhysical),
		"Overcommit Ratios", string(h.OvercommitRatios),
		"Resource Used", string(h.ResourceUsed),
		"Last Heartbeat", heartbeat,
		"Created", h.CreatedAt.Format("2006-01-02 15:04:05"),
		"Updated", h.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
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

// printDetail renders a single resource as key-value pairs (table) or raw JSON.
func (app *cli) printDetail(v any, kvPairs ...string) error {
	if app.output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i := 0; i < len(kvPairs)-1; i += 2 {
		fmt.Fprintf(w, "%s:\t%s\n", kvPairs[i], kvPairs[i+1])
	}
	return w.Flush()
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
