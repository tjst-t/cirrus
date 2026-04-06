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
	"github.com/tjst-t/cirrus/internal/az"
	"github.com/tjst-t/cirrus/internal/client"
	"github.com/tjst-t/cirrus/internal/compute"
	"github.com/tjst-t/cirrus/internal/flavor"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/identity"
	"github.com/tjst-t/cirrus/internal/network"
	"github.com/tjst-t/cirrus/internal/quota"
	"github.com/tjst-t/cirrus/internal/storage"
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
	rootCmd.AddCommand(app.newGroupCmd())
	rootCmd.AddCommand(app.newPolicyCmd())
	rootCmd.AddCommand(app.newVolumeTypeCmd())
	rootCmd.AddCommand(app.newVolumeCmd())
	rootCmd.AddCommand(app.newFlavorCmd())
	rootCmd.AddCommand(app.newVMCmd())
	rootCmd.AddCommand(app.newQuotaCmd())
	rootCmd.AddCommand(app.newAdminCmd())
	rootCmd.AddCommand(app.newEgressCmd())
	rootCmd.AddCommand(app.newIngressCmd())

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
	var tenant, org, cidr string
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
			n, err := c.CreateNetwork(ctx, tenantID, args[0], cidr)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "TENANT_ID", "NAME", "CIDR", "VNI", "STATUS", "CREATED"},
				[][]string{networkRow(n)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&cidr, "cidr", "", "Network CIDR (auto-assigned if not specified)")
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
				[]string{"ID", "TENANT_ID", "NAME", "CIDR", "VNI", "STATUS", "CREATED"},
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
				[]string{"ID", "TENANT_ID", "NAME", "CIDR", "VNI", "STATUS", "CREATED"},
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

// --- Group commands ---

func (app *cli) newGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "group",
		Aliases: []string{"grp"},
		Short:   "Manage groups within a network",
	}
	cmd.AddCommand(app.newGroupCreateCmd())
	cmd.AddCommand(app.newGroupListCmd())
	cmd.AddCommand(app.newGroupShowCmd())
	cmd.AddCommand(app.newGroupDeleteCmd())
	return cmd
}

func (app *cli) newGroupCreateCmd() *cobra.Command {
	var tenant, org, nw string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, nw, tenantID)
			if err != nil {
				return err
			}
			g, err := c.CreateGroup(ctx, tenantID, networkID, args[0])
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NETWORK_ID", "NAME"},
				[][]string{groupRow(g)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&nw, "network", "", "Network (ID or name) (required)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("network")
	return cmd
}

func (app *cli) newGroupListCmd() *cobra.Command {
	var tenant, org, nw string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List groups in a network",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, nw, tenantID)
			if err != nil {
				return err
			}
			groups, err := c.ListGroups(ctx, tenantID, networkID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(groups))
			for i := range groups {
				rows[i] = groupRow(&groups[i])
			}
			return app.printTable(
				[]string{"ID", "NETWORK_ID", "NAME"},
				rows,
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&nw, "network", "", "Network (ID or name) (required)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("network")
	return cmd
}

func (app *cli) newGroupShowCmd() *cobra.Command {
	var tenant, org, nw string
	cmd := &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show group details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, nw, tenantID)
			if err != nil {
				return err
			}
			groupID, err := c.ResolveGroup(ctx, args[0], tenantID, networkID)
			if err != nil {
				return err
			}
			g, err := c.GetGroup(ctx, networkID, groupID)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NETWORK_ID", "NAME"},
				[][]string{groupRow(g)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&nw, "network", "", "Network (ID or name) (required)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("network")
	return cmd
}

func (app *cli) newGroupDeleteCmd() *cobra.Command {
	var tenant, org, nw string
	cmd := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, nw, tenantID)
			if err != nil {
				return err
			}
			groupID, err := c.ResolveGroup(ctx, args[0], tenantID, networkID)
			if err != nil {
				return err
			}
			if err := c.DeleteGroup(ctx, networkID, groupID); err != nil {
				return err
			}
			return app.printStatus("Deleted", "group", groupID.String())
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&nw, "network", "", "Network (ID or name) (required)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("network")
	return cmd
}

// --- Policy commands ---

func (app *cli) newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage policies within a network",
	}
	cmd.AddCommand(app.newPolicyCreateCmd())
	cmd.AddCommand(app.newPolicyListCmd())
	cmd.AddCommand(app.newPolicyDeleteCmd())
	return cmd
}

func (app *cli) newPolicyCreateCmd() *cobra.Command {
	var tenant, org, nw, srcGroup, dstGroup, protocol, action string
	var dstPort, priority int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new policy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, nw, tenantID)
			if err != nil {
				return err
			}
			srcGroupID, err := c.ResolveGroup(ctx, srcGroup, tenantID, networkID)
			if err != nil {
				return err
			}
			dstGroupID, err := c.ResolveGroup(ctx, dstGroup, tenantID, networkID)
			if err != nil {
				return err
			}
			spec := network.PolicySpec{
				SrcGroupID: srcGroupID,
				DstGroupID: dstGroupID,
				Protocol:   protocol,
			}
			if cmd.Flags().Changed("dst-port") {
				spec.DstPort = &dstPort
			}
			if priority > 0 {
				spec.Priority = priority
			}
			if action != "" {
				spec.Action = action
			}
			p, err := c.CreatePolicy(ctx, tenantID, networkID, spec)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "SRC_GROUP", "DST_GROUP", "PROTOCOL", "DST_PORT", "PRIORITY", "ACTION"},
				[][]string{policyRow(p)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&nw, "network", "", "Network (ID or name) (required)")
	cmd.Flags().StringVar(&srcGroup, "src-group", "", "Source group (ID or name) (required)")
	cmd.Flags().StringVar(&dstGroup, "dst-group", "", "Destination group (ID or name) (required)")
	cmd.Flags().StringVar(&protocol, "protocol", "", "Protocol (tcp, udp, icmp, any) (required)")
	cmd.Flags().IntVar(&dstPort, "dst-port", 0, "Destination port")
	cmd.Flags().IntVar(&priority, "priority", 0, "Priority (default: 1000)")
	cmd.Flags().StringVar(&action, "action", "", "Action (default: allow)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("src-group")
	_ = cmd.MarkFlagRequired("dst-group")
	_ = cmd.MarkFlagRequired("protocol")
	return cmd
}

func (app *cli) newPolicyListCmd() *cobra.Command {
	var tenant, org, nw string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List policies in a network",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, nw, tenantID)
			if err != nil {
				return err
			}
			policies, err := c.ListPolicies(ctx, tenantID, networkID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(policies))
			for i := range policies {
				rows[i] = policyRow(&policies[i])
			}
			return app.printTable(
				[]string{"ID", "SRC_GROUP", "DST_GROUP", "PROTOCOL", "DST_PORT", "PRIORITY", "ACTION"},
				rows,
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&nw, "network", "", "Network (ID or name) (required)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("network")
	return cmd
}

func (app *cli) newPolicyDeleteCmd() *cobra.Command {
	var tenant, org, nw string
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, nw, tenantID)
			if err != nil {
				return err
			}
			policyID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("policy ID must be a UUID: %w", err)
			}
			if err := c.DeletePolicy(ctx, networkID, policyID); err != nil {
				return err
			}
			return app.printStatus("Deleted", "policy", policyID.String())
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().StringVar(&nw, "network", "", "Network (ID or name) (required)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("network")
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
	cmd.AddCommand(app.newAdminStorageBackendCmd())
	cmd.AddCommand(app.newAdminVolumeTypeCmd())
	cmd.AddCommand(app.newAdminFlavorCmd())
	cmd.AddCommand(app.newAdminVMCmd())
	cmd.AddCommand(app.newAdminGatewayNodeCmd())
	cmd.AddCommand(app.newAdminIPPoolCmd())
	return cmd
}

// --- Admin: VM commands ---

func (app *cli) newAdminVMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "Manage virtual machines (admin)",
	}
	cmd.AddCommand(app.newAdminVMRepairCmd())
	return cmd
}

func (app *cli) newAdminVMRepairCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair VM",
		Short: "Repair a VM in error state (transition error → stopped)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			vmID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid vm UUID: %w", err)
			}
			return c.RepairVM(ctx, vmID)
		},
	}
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
	return []string{n.ID.String(), n.TenantID.String(), n.Name, n.CIDR, fmt.Sprintf("%d", n.VNI), string(n.Status), n.CreatedAt.Format("2006-01-02 15:04:05")}
}

func groupRow(g *network.Group) []string {
	return []string{g.ID.String(), g.NetworkID.String(), g.Name}
}

func egressRow(e *network.Egress) []string {
	detail := e.Config.PublicIP
	switch e.Type {
	case network.EgressTypeVPNIPsec:
		if e.Config.VPNIPsec != nil {
			detail = fmt.Sprintf("peer=%s", e.Config.VPNIPsec.PeerIP)
		}
	case network.EgressTypeVPNWireGuard:
		if e.Config.VPNWireGuard != nil {
			detail = fmt.Sprintf("pubkey=%s", e.Config.VPNWireGuard.PublicKey)
		}
	case network.EgressTypeDirectConnect:
		if e.Config.DirectConnect != nil {
			detail = fmt.Sprintf("vlan=%d port=%s", e.Config.DirectConnect.VLANID, e.Config.DirectConnect.UplinkPort)
		}
	}
	return []string{e.ID.String(), e.NetworkID.String(), e.Type, detail}
}

func policyRow(p *network.Policy) []string {
	dstPort := "-"
	if p.DstPort != nil {
		dstPort = fmt.Sprintf("%d", *p.DstPort)
	}
	return []string{p.ID.String(), p.SrcGroupID.String(), p.DstGroupID.String(), p.Protocol, dstPort, fmt.Sprintf("%d", p.Priority), p.Action}
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

// --- Admin: Storage Backend commands ---

func (app *cli) newAdminStorageBackendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage-backend",
		Short: "Manage storage backends",
	}
	cmd.AddCommand(app.newStorageBackendCreateCmd())
	cmd.AddCommand(app.newStorageBackendListCmd())
	cmd.AddCommand(app.newStorageBackendShowCmd())
	cmd.AddCommand(app.newStorageBackendDrainCmd())
	return cmd
}

func storageBackendRow(b *storage.Backend) []string {
	return []string{b.ID.String(), b.Name, b.Driver, string(b.State), b.Endpoint}
}

func (app *cli) newStorageBackendCreateCmd() *cobra.Command {
	var (
		domainID     string
		driver       string
		endpoint     string
		capacityGB   int64
		iops         int64
		capabilities []string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Register a new storage backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()

			sdID, err := c.ResolveStorageDomain(ctx, domainID)
			if err != nil {
				return err
			}
			b, err := c.RegisterStorageBackend(ctx, client.RegisterBackendRequest{
				StorageDomainID: sdID,
				Name:            args[0],
				Driver:          driver,
				Endpoint:        endpoint,
				TotalCapacityGB: capacityGB,
				TotalIOPS:       iops,
				Capabilities:    capabilities,
			})
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "DRIVER", "STATE", "ENDPOINT"},
				[][]string{storageBackendRow(b)},
			)
		},
	}
	cmd.Flags().StringVar(&domainID, "storage-domain", "", "Storage domain name or ID (required)")
	cmd.Flags().StringVar(&driver, "driver", "sim", "Driver type (sim, iscsi, rbd, ...)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Backend management API endpoint (required)")
	cmd.Flags().Int64Var(&capacityGB, "capacity-gb", 0, "Total capacity in GB")
	cmd.Flags().Int64Var(&iops, "iops", 0, "Total IOPS")
	cmd.Flags().StringSliceVar(&capabilities, "capabilities", nil, "Capabilities (ssd,encryption,...)")
	_ = cmd.MarkFlagRequired("storage-domain")
	_ = cmd.MarkFlagRequired("endpoint")
	return cmd
}

func (app *cli) newStorageBackendListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List storage backends",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			bs, err := app.newClient().ListStorageBackends(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(bs))
			for i := range bs {
				rows[i] = storageBackendRow(&bs[i])
			}
			return app.printTable([]string{"ID", "NAME", "DRIVER", "STATE", "ENDPOINT"}, rows)
		},
	}
}

func (app *cli) newStorageBackendShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show storage backend details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveStorageBackend(ctx, args[0])
			if err != nil {
				return err
			}
			b, err := c.GetStorageBackend(ctx, id)
			if err != nil {
				return err
			}
			return app.printDetail(b,
				"ID", b.ID.String(),
				"Name", b.Name,
				"Driver", b.Driver,
				"State", string(b.State),
				"Endpoint", b.Endpoint,
				"CapacityGB", fmt.Sprintf("%d", b.TotalCapacityGB),
				"IOPS", fmt.Sprintf("%d", b.TotalIOPS),
			)
		},
	}
}

func (app *cli) newStorageBackendDrainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drain <id-or-name>",
		Short: "Set storage backend to draining state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveStorageBackend(ctx, args[0])
			if err != nil {
				return err
			}
			if err := c.DrainStorageBackend(ctx, id); err != nil {
				return err
			}
			return app.printStatus("drained", "storage-backend", id.String())
		},
	}
}

// --- Admin: Volume Type commands (create) ---

func (app *cli) newAdminVolumeTypeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume-type",
		Short: "Manage volume types (admin)",
	}
	cmd.AddCommand(app.newAdminVolumeTypeCreateCmd())
	return cmd
}

func (app *cli) newAdminVolumeTypeCreateCmd() *cobra.Command {
	var (
		description  string
		capabilities []string
		isPublic     bool
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new volume type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			vt, err := app.newClient().CreateVolumeType(ctx, client.CreateVolumeTypeRequest{
				Name:                 args[0],
				Description:          description,
				RequiredCapabilities: capabilities,
				IsPublic:             isPublic,
			})
			if err != nil {
				return err
			}
			return app.printDetail(vt,
				"ID", vt.ID.String(),
				"Name", vt.Name,
				"Description", vt.Description,
				"IsPublic", fmt.Sprintf("%v", vt.IsPublic),
			)
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringSliceVar(&capabilities, "capabilities", nil, "Required backend capabilities")
	cmd.Flags().BoolVar(&isPublic, "public", true, "Make volume type public")
	return cmd
}

// --- Volume Type commands (tenant: list/show) ---

func (app *cli) newVolumeTypeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume-type",
		Short: "List and show volume types",
	}
	cmd.AddCommand(app.newVolumeTypeListCmd())
	cmd.AddCommand(app.newVolumeTypeShowCmd())
	return cmd
}

func (app *cli) newVolumeTypeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available volume types",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			vts, err := app.newClient().ListVolumeTypes(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(vts))
			for i, vt := range vts {
				rows[i] = []string{vt.ID.String(), vt.Name, vt.Description, fmt.Sprintf("%v", vt.IsPublic)}
			}
			return app.printTable([]string{"ID", "NAME", "DESCRIPTION", "PUBLIC"}, rows)
		},
	}
}

func (app *cli) newVolumeTypeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show volume type details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveVolumeType(ctx, args[0])
			if err != nil {
				return err
			}
			vt, err := c.GetVolumeType(ctx, id)
			if err != nil {
				return err
			}
			return app.printDetail(vt,
				"ID", vt.ID.String(),
				"Name", vt.Name,
				"Description", vt.Description,
				"IsPublic", fmt.Sprintf("%v", vt.IsPublic),
			)
		},
	}
}

// --- Flavor commands ---

func (app *cli) newAdminFlavorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flavor",
		Short: "Manage VM flavors (admin)",
	}
	cmd.AddCommand(app.newAdminFlavorCreateCmd())
	cmd.AddCommand(app.newAdminFlavorDeleteCmd())
	return cmd
}

func (app *cli) newAdminFlavorCreateCmd() *cobra.Command {
	var vcpus int
	var ramMB, diskGB int64
	var isPublic bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new VM flavor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			f, err := app.newClient().CreateFlavor(ctx, client.CreateFlavorRequest{
				Name:   args[0],
				VCPUs:  vcpus,
				RAMMB:  ramMB,
				DiskGB: diskGB,
				IsPublic: func() *bool { v := isPublic; return &v }(),
			})
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "VCPUS", "RAM_MB", "DISK_GB", "PUBLIC"},
				[][]string{flavorRow(f)},
			)
		},
	}
	cmd.Flags().IntVar(&vcpus, "vcpus", 1, "Number of vCPUs")
	cmd.Flags().Int64Var(&ramMB, "ram-mb", 1024, "RAM in MB")
	cmd.Flags().Int64Var(&diskGB, "disk-gb", 0, "Disk size in GB")
	cmd.Flags().BoolVar(&isPublic, "public", true, "Make flavor public")
	return cmd
}

func (app *cli) newAdminFlavorDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a flavor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveFlavor(ctx, args[0])
			if err != nil {
				return err
			}
			return c.DeleteFlavor(ctx, id)
		},
	}
}

func (app *cli) newFlavorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flavor",
		Short: "List and show VM flavors",
	}
	cmd.AddCommand(app.newFlavorListCmd())
	cmd.AddCommand(app.newFlavorShowCmd())
	return cmd
}

func (app *cli) newFlavorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available flavors",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			flavors, err := app.newClient().ListFlavors(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(flavors))
			for i := range flavors {
				rows[i] = flavorRow(&flavors[i])
			}
			return app.printTable([]string{"ID", "NAME", "VCPUS", "RAM_MB", "DISK_GB", "PUBLIC"}, rows)
		},
	}
}

func (app *cli) newFlavorShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show flavor details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveFlavor(ctx, args[0])
			if err != nil {
				return err
			}
			f, err := c.GetFlavor(ctx, id)
			if err != nil {
				return err
			}
			return app.printDetail(f,
				"ID", f.ID.String(),
				"Name", f.Name,
				"VCPUs", fmt.Sprintf("%d", f.VCPUs),
				"RAM_MB", fmt.Sprintf("%d", f.RAMMB),
				"Disk_GB", fmt.Sprintf("%d", f.DiskGB),
				"Public", fmt.Sprintf("%v", f.IsPublic),
			)
		},
	}
}

func flavorRow(f *flavor.Flavor) []string {
	return []string{
		f.ID.String(),
		f.Name,
		fmt.Sprintf("%d", f.VCPUs),
		fmt.Sprintf("%d", f.RAMMB),
		fmt.Sprintf("%d", f.DiskGB),
		fmt.Sprintf("%v", f.IsPublic),
	}
}

// --- Volume commands (tenant-scoped) ---

func (app *cli) newVolumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage volumes",
	}
	cmd.AddCommand(app.newVolumeCreateCmd())
	cmd.AddCommand(app.newVolumeListCmd())
	cmd.AddCommand(app.newVolumeShowCmd())
	cmd.AddCommand(app.newVolumeDeleteCmd())
	cmd.AddCommand(app.newVolumeResizeCmd())
	return cmd
}

func volumeRow(v *storage.Volume) []string {
	vtID := ""
	if v.VolumeTypeID != nil {
		vtID = v.VolumeTypeID.String()
	}
	return []string{v.ID.String(), v.Name, fmt.Sprintf("%d", v.SizeGB), string(v.State), vtID}
}

func (app *cli) newVolumeCreateCmd() *cobra.Command {
	var (
		tenant       string
		org          string
		volumeTypeID string
		sizeGB       int64
		azID         string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new volume",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()

			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}

			req := client.CreateVolumeRequest{
				Name:   args[0],
				SizeGB: sizeGB,
			}
			if volumeTypeID != "" {
				vtID, err := c.ResolveVolumeType(ctx, volumeTypeID)
				if err != nil {
					return err
				}
				req.VolumeTypeID = &vtID
			}
			if azID != "" {
				id, err := uuid.Parse(azID)
				if err != nil {
					return fmt.Errorf("invalid az-id: %w", err)
				}
				req.AZID = &id
			}

			v, err := c.CreateVolume(ctx, tenantID, req)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "SIZE_GB", "STATE", "VOLUME_TYPE"},
				[][]string{volumeRow(v)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID (required when --tenant is a name)")
	cmd.Flags().StringVar(&volumeTypeID, "volume-type", "", "Volume type name or ID")
	cmd.Flags().Int64Var(&sizeGB, "size-gb", 0, "Size in GB (required)")
	cmd.Flags().StringVar(&azID, "az", "", "Availability zone ID")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("size-gb")
	return cmd
}

func (app *cli) newVolumeListCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List volumes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			vs, err := c.ListVolumes(ctx, tenantID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(vs))
			for i := range vs {
				rows[i] = volumeRow(&vs[i])
			}
			return app.printTable([]string{"ID", "NAME", "SIZE_GB", "STATE", "VOLUME_TYPE"}, rows)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID (required when --tenant is a name)")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newVolumeShowCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show volume details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			id, err := c.ResolveVolume(ctx, tenantID, args[0])
			if err != nil {
				return err
			}
			v, err := c.GetVolume(ctx, tenantID, id)
			if err != nil {
				return err
			}
			return app.printDetail(v,
				"ID", v.ID.String(),
				"Name", v.Name,
				"SizeGB", fmt.Sprintf("%d", v.SizeGB),
				"State", string(v.State),
				"TenantID", v.TenantID.String(),
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID (required when --tenant is a name)")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newVolumeDeleteCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a volume",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			id, err := c.ResolveVolume(ctx, tenantID, args[0])
			if err != nil {
				return err
			}
			if err := c.DeleteVolume(ctx, tenantID, id); err != nil {
				return err
			}
			return app.printStatus("deleted", "volume", id.String())
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID (required when --tenant is a name)")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newVolumeResizeCmd() *cobra.Command {
	var tenant, org string
	var newSizeGB int64
	cmd := &cobra.Command{
		Use:   "resize <id-or-name>",
		Short: "Resize a volume (increase only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if newSizeGB <= 0 {
				return fmt.Errorf("--size must be a positive integer (got %d)", newSizeGB)
			}
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			id, err := c.ResolveVolume(ctx, tenantID, args[0])
			if err != nil {
				return err
			}
			v, err := c.ResizeVolume(ctx, tenantID, id, newSizeGB)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "SIZE_GB", "STATE"},
				[][]string{volumeRow(v)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID (required when --tenant is a name)")
	cmd.Flags().Int64Var(&newSizeGB, "size", 0, "New size in GB (required, must be larger than current)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("size")
	return cmd
}

// --- VM commands (tenant-scoped) ---

func (app *cli) newVMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "Manage virtual machines",
	}
	cmd.AddCommand(app.newVMCreateCmd())
	cmd.AddCommand(app.newVMListCmd())
	cmd.AddCommand(app.newVMShowCmd())
	cmd.AddCommand(app.newVMDeleteCmd())
	cmd.AddCommand(app.newVMStartCmd())
	cmd.AddCommand(app.newVMStopCmd())
	cmd.AddCommand(app.newVMForceStopCmd())
	cmd.AddCommand(app.newVMRebootCmd())
	return cmd
}

func vmRow(v *compute.VM) []string {
	flavorID := ""
	if v.FlavorID != nil {
		flavorID = v.FlavorID.String()
	}
	hostID := ""
	if v.HostID != nil {
		hostID = v.HostID.String()
	}
	return []string{v.ID.String(), v.Name, string(v.Status), flavorID, hostID}
}

func (app *cli) newVMCreateCmd() *cobra.Command {
	var tenant, org, flavorArg, azArg, networkArg, vtArg string
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a new virtual machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			flvID, err := c.ResolveFlavor(ctx, flavorArg)
			if err != nil {
				return fmt.Errorf("resolve flavor: %w", err)
			}
			req := client.CreateVMRequest{
				Name:      args[0],
				FlavorID:  flvID.String(),
				AZID:      azArg,
				NetworkID: networkArg,
			}
			if vtArg != "" {
				req.VolumeTypeID = &vtArg
			}
			vm, err := c.CreateVM(ctx, tenantID, req)
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "STATUS", "FLAVOR_ID", "HOST_ID"},
				[][]string{vmRow(vm)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID")
	cmd.Flags().StringVar(&flavorArg, "flavor", "", "Flavor name or ID (required)")
	cmd.Flags().StringVar(&azArg, "az", "", "Availability zone name or ID")
	cmd.Flags().StringVar(&networkArg, "network", "", "Network name or ID")
	cmd.Flags().StringVar(&vtArg, "volume-type", "", "Volume type name or ID")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("flavor")
	return cmd
}

func (app *cli) newVMListCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List virtual machines",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			vms, err := c.ListVMs(ctx, tenantID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(vms))
			for i := range vms {
				rows[i] = vmRow(&vms[i])
			}
			return app.printTable([]string{"ID", "NAME", "STATUS", "FLAVOR_ID", "HOST_ID"}, rows)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newVMShowCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "show VM",
		Short: "Show virtual machine details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			vm, err := c.ResolveVM(ctx, tenantID, args[0])
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "STATUS", "FLAVOR_ID", "HOST_ID"},
				[][]string{vmRow(vm)},
			)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newVMDeleteCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "delete VM",
		Short: "Delete a virtual machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			vm, err := c.ResolveVM(ctx, tenantID, args[0])
			if err != nil {
				return err
			}
			return c.DeleteVM(ctx, tenantID, vm.ID)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization name or ID")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func newVMActionCmdHelper(use, short, action string) func(app *cli) *cobra.Command {
	return func(app *cli) *cobra.Command {
		var tenant, org string
		cmd := &cobra.Command{
			Use:   use + " VM",
			Short: short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx := app.cmdContext()
				c := app.newClient()
				tenantID, err := app.resolveTenant(ctx, c, tenant, org)
				if err != nil {
					return err
				}
				vm, err := c.ResolveVM(ctx, tenantID, args[0])
				if err != nil {
					return err
				}
				return c.VMAction(ctx, tenantID, vm.ID, action)
			},
		}
		cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name or ID (required)")
		cmd.Flags().StringVar(&org, "org", "", "Organization name or ID")
		_ = cmd.MarkFlagRequired("tenant")
		return cmd
	}
}

func (app *cli) newVMStartCmd() *cobra.Command {
	return newVMActionCmdHelper("start", "Start a stopped virtual machine", "start")(app)
}

func (app *cli) newVMStopCmd() *cobra.Command {
	return newVMActionCmdHelper("stop", "Gracefully stop a running virtual machine", "stop")(app)
}

func (app *cli) newVMForceStopCmd() *cobra.Command {
	return newVMActionCmdHelper("force-stop", "Forcefully power off a virtual machine", "force-stop")(app)
}

func (app *cli) newVMRebootCmd() *cobra.Command {
	return newVMActionCmdHelper("reboot", "Reboot a running virtual machine", "reboot")(app)
}

// --- Quota ---

func (app *cli) newQuotaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Manage resource quotas",
	}
	cmd.AddCommand(app.newQuotaShowCmd())
	cmd.AddCommand(app.newQuotaSetCmd())
	return cmd
}

func (app *cli) newQuotaShowCmd() *cobra.Command {
	var tenant, org string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show quota limits and usage for a tenant",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			qr, err := c.GetTenantQuota(ctx, tenantID)
			if err != nil {
				return err
			}
			if app.output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(qr)
			}
			return printQuota(qr)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newQuotaSetCmd() *cobra.Command {
	var tenant, org string
	var vcpus, ramMB, volumeGB, vms, volumes, snapshots, networks int
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set quota limits for a tenant (admin only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			limits := quota.Limits{
				Vcpus:     vcpus,
				RAMMB:     ramMB,
				VolumeGB:  volumeGB,
				VMs:       vms,
				Volumes:   volumes,
				Snapshots: snapshots,
				Networks:  networks,
			}
			qr, err := c.SetTenantQuota(ctx, tenantID, limits)
			if err != nil {
				return err
			}
			if app.output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(qr)
			}
			return printQuota(qr)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (ID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (ID or name) for tenant name resolution")
	cmd.Flags().IntVar(&vcpus, "vcpus", 0, "vCPU limit (0=unlimited)")
	cmd.Flags().IntVar(&ramMB, "ram-mb", 0, "RAM limit in MB (0=unlimited)")
	cmd.Flags().IntVar(&volumeGB, "volume-gb", 0, "Total volume capacity limit in GB (0=unlimited)")
	cmd.Flags().IntVar(&vms, "vms", 0, "VM count limit (0=unlimited)")
	cmd.Flags().IntVar(&volumes, "volumes", 0, "Volume count limit (0=unlimited)")
	cmd.Flags().IntVar(&snapshots, "snapshots", 0, "Snapshot count limit (0=unlimited)")
	cmd.Flags().IntVar(&networks, "networks", 0, "Network count limit (0=unlimited)")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func printQuota(qr *client.QuotaResponse) error {
	l := qr.Limits
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RESOURCE\tLIMIT\tUSED")
	vcpusUsed, ramUsed, volGBUsed, vmsUsed, volsUsed, snapsUsed, netsUsed := 0, 0, 0, 0, 0, 0, 0
	if qr.Usage != nil {
		vcpusUsed = qr.Usage.VcpusUsed
		ramUsed = qr.Usage.RAMMBUsed
		volGBUsed = qr.Usage.VolumeGBUsed
		vmsUsed = qr.Usage.VMsCount
		volsUsed = qr.Usage.VolumesCount
		snapsUsed = qr.Usage.SnapshotsCount
		netsUsed = qr.Usage.NetworksCount
	}
	fmt.Fprintf(w, "vcpus\t%s\t%d\n", limitStr(l.Vcpus), vcpusUsed)
	fmt.Fprintf(w, "ram_mb\t%s\t%d\n", limitStr(l.RAMMB), ramUsed)
	fmt.Fprintf(w, "volume_gb\t%s\t%d\n", limitStr(l.VolumeGB), volGBUsed)
	fmt.Fprintf(w, "vms\t%s\t%d\n", limitStr(l.VMs), vmsUsed)
	fmt.Fprintf(w, "volumes\t%s\t%d\n", limitStr(l.Volumes), volsUsed)
	fmt.Fprintf(w, "snapshots\t%s\t%d\n", limitStr(l.Snapshots), snapsUsed)
	fmt.Fprintf(w, "networks\t%s\t%d\n", limitStr(l.Networks), netsUsed)
	return w.Flush()
}

func limitStr(v int) string {
	if v == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", v)
}

// --- Admin: Gateway Node commands ---

func (app *cli) newAdminGatewayNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gateway-node",
		Aliases: []string{"gw"},
		Short:   "Manage gateway nodes",
	}
	cmd.AddCommand(app.newGatewayNodeListCmd())
	cmd.AddCommand(app.newGatewayNodeCreateCmd())
	cmd.AddCommand(app.newGatewayNodeDeleteCmd())
	cmd.AddCommand(app.newGatewayNodeAssignCmd())
	return cmd
}

func (app *cli) newGatewayNodeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all gateway nodes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			nodes, err := app.newClient().ListGatewayNodes(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(nodes))
			for i, n := range nodes {
				rows[i] = []string{n.ID.String(), n.HostID.String(), n.ExternalIP, n.InternalIP, n.Status, n.CreatedAt.Format("2006-01-02T15:04:05Z")}
			}
			return app.printTable([]string{"ID", "HOST_ID", "EXTERNAL_IP", "INTERNAL_IP", "STATUS", "CREATED"}, rows)
		},
	}
}

func (app *cli) newGatewayNodeCreateCmd() *cobra.Command {
	var hostStr, externalIP, internalIP, uplinkPort string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a gateway node",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			hostID, err := c.ResolveHost(ctx, hostStr)
			if err != nil {
				return err
			}
			gw, err := c.CreateGatewayNode(ctx, network.GatewayNodeSpec{
				HostID:     hostID,
				ExternalIP: externalIP,
				InternalIP: internalIP,
				UplinkPort: uplinkPort,
			})
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "HOST_ID", "EXTERNAL_IP", "INTERNAL_IP", "UPLINK_PORT", "STATUS"},
				[][]string{{gw.ID.String(), gw.HostID.String(), gw.ExternalIP, gw.InternalIP, gw.UplinkPort, gw.Status}},
			)
		},
	}
	cmd.Flags().StringVar(&hostStr, "host", "", "Host (UUID or name) (required)")
	cmd.Flags().StringVar(&externalIP, "external-ip", "", "External (public) IP address (required)")
	cmd.Flags().StringVar(&internalIP, "internal-ip", "", "Internal (fabric) IP address (required)")
	cmd.Flags().StringVar(&uplinkPort, "uplink-port", "", "Physical uplink port for Direct Connect VLAN trunk (optional)")
	_ = cmd.MarkFlagRequired("host")
	_ = cmd.MarkFlagRequired("external-ip")
	_ = cmd.MarkFlagRequired("internal-ip")
	return cmd
}

func (app *cli) newGatewayNodeDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a gateway node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid gateway-node ID: %w", err)
			}
			if err := c.DeleteGatewayNode(ctx, id); err != nil {
				return err
			}
			return app.printStatus("Deleted", "gateway-node", id.String())
		},
	}
}

func (app *cli) newGatewayNodeAssignCmd() *cobra.Command {
	var networkStr, org, tenant string
	var gwNodeStr string
	cmd := &cobra.Command{
		Use:   "assign",
		Short: "Assign a gateway node to a network",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, networkStr, tenantID)
			if err != nil {
				return err
			}
			gwID, err := uuid.Parse(gwNodeStr)
			if err != nil {
				return fmt.Errorf("invalid gateway-node UUID: %w", err)
			}
			return c.AssignGatewayNodeToNetwork(ctx, networkID, gwID)
		},
	}
	cmd.Flags().StringVar(&networkStr, "network", "", "Network (UUID or name) (required)")
	cmd.Flags().StringVar(&gwNodeStr, "gateway-node", "", "Gateway node UUID (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (UUID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization (UUID or name) for tenant name resolution")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("gateway-node")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

// --- Admin: IP Pool commands ---

func (app *cli) newAdminIPPoolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ip-pool",
		Short: "Manage IP pools",
	}
	cmd.AddCommand(app.newIPPoolListCmd())
	cmd.AddCommand(app.newIPPoolCreateCmd())
	cmd.AddCommand(app.newIPPoolDeleteCmd())
	return cmd
}

func (app *cli) newIPPoolListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all IP pools",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			pools, err := app.newClient().ListIPPools(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, len(pools))
			for i, p := range pools {
				rows[i] = []string{p.ID.String(), p.Name, p.CIDR, p.Description}
			}
			return app.printTable([]string{"ID", "NAME", "CIDR", "DESCRIPTION"}, rows)
		},
	}
}

func (app *cli) newIPPoolCreateCmd() *cobra.Command {
	var description string
	cmd := &cobra.Command{
		Use:   "create <name> --cidr <cidr>",
		Short: "Create an IP pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			cidr, _ := cmd.Flags().GetString("cidr")
			pool, err := app.newClient().CreateIPPool(ctx, network.IPPoolSpec{
				Name:        args[0],
				CIDR:        cidr,
				Description: description,
			})
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NAME", "CIDR", "DESCRIPTION"},
				[][]string{{pool.ID.String(), pool.Name, pool.CIDR, pool.Description}},
			)
		},
	}
	cmd.Flags().String("cidr", "", "CIDR block (required)")
	cmd.Flags().StringVar(&description, "description", "", "Optional description")
	_ = cmd.MarkFlagRequired("cidr")
	return cmd
}

func (app *cli) newIPPoolDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an IP pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			id, err := c.ResolveIPPool(ctx, args[0])
			if err != nil {
				return err
			}
			if err := c.DeleteIPPool(ctx, id); err != nil {
				return err
			}
			return app.printStatus("Deleted", "ip-pool", id.String())
		},
	}
}

// --- Egress commands (tenant) ---

func (app *cli) newEgressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "egress",
		Short: "Manage network egress rules",
	}
	cmd.AddCommand(app.newEgressListCmd())
	cmd.AddCommand(app.newEgressCreateCmd())
	cmd.AddCommand(app.newEgressDeleteCmd())
	return cmd
}

func (app *cli) newEgressListCmd() *cobra.Command {
	var networkStr, tenant, org string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List egress rules for a network",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, networkStr, tenantID)
			if err != nil {
				return err
			}
			egresses, err := c.ListEgresses(ctx, tenantID, networkID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(egresses))
			for i := range egresses {
				rows[i] = egressRow(&egresses[i])
			}
			return app.printTable([]string{"ID", "NETWORK_ID", "TYPE", "DETAIL"}, rows)
		},
	}
	cmd.Flags().StringVar(&networkStr, "network", "", "Network (UUID or name) (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (UUID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization for tenant name resolution")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newEgressCreateCmd() *cobra.Command {
	var (
		networkStr, tenant, org, egressType, publicIP string
		// IPsec flags
		ipsecPeerIP, ipsecPSK, ipsecLocalCIDR, ipsecRemoteCIDR string
		// WireGuard flags
		wgPeerPublicKey, wgPeerEndpoint string
		wgAllowedIPs                    []string
		wgListenPort                    int
		// Direct Connect flags
		dcVLANID int
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an egress rule",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, networkStr, tenantID)
			if err != nil {
				return err
			}

			var spec network.EgressSpec
			switch egressType {
			case network.EgressTypeNATGateway:
				spec = network.EgressSpec{
					Type:   egressType,
					Config: network.EgressConfig{PublicIP: publicIP},
				}
			case network.EgressTypeVPNIPsec:
				spec = network.EgressSpec{
					Type: egressType,
					Config: network.EgressConfig{
						VPNIPsec: &network.VPNIPsecConfig{
							PeerIP:       ipsecPeerIP,
							PreSharedKey: ipsecPSK,
							LocalCIDR:    ipsecLocalCIDR,
							RemoteCIDR:   ipsecRemoteCIDR,
						},
					},
				}
			case network.EgressTypeVPNWireGuard:
				spec = network.EgressSpec{
					Type: egressType,
					Config: network.EgressConfig{
						VPNWireGuard: &network.VPNWireGuardConfig{
							PeerPublicKey: wgPeerPublicKey,
							PeerEndpoint:  wgPeerEndpoint,
							AllowedIPs:    wgAllowedIPs,
							ListenPort:    wgListenPort,
						},
					},
				}
			case network.EgressTypeDirectConnect:
				spec = network.EgressSpec{
					Type: egressType,
					Config: network.EgressConfig{
						DirectConnect: &network.DirectConnectConfig{
							VLANID: dcVLANID,
						},
					},
				}
			default:
				return fmt.Errorf("unknown egress type: %s", egressType)
			}

			e, err := c.CreateEgress(ctx, tenantID, networkID, spec)
			if err != nil {
				return err
			}

			return app.printTable(
				[]string{"ID", "NETWORK_ID", "TYPE", "DETAIL"},
				[][]string{egressRow(e)},
			)
		},
	}
	cmd.Flags().StringVar(&networkStr, "network", "", "Network (UUID or name) (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (UUID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization for tenant name resolution")
	cmd.Flags().StringVar(&egressType, "type", network.EgressTypeNATGateway, "Egress type (nat_gateway|vpn_ipsec|vpn_wireguard|direct_connect)")
	cmd.Flags().StringVar(&publicIP, "public-ip", "", "Public IP for SNAT (nat_gateway only)")
	// IPsec flags
	cmd.Flags().StringVar(&ipsecPeerIP, "ipsec-peer-ip", "", "Remote IPsec peer IP (vpn_ipsec only)")
	cmd.Flags().StringVar(&ipsecPSK, "ipsec-psk", "", "IKEv2 pre-shared key (vpn_ipsec only)")
	cmd.Flags().StringVar(&ipsecLocalCIDR, "ipsec-local-cidr", "", "Local CIDR for IPsec tunnel (vpn_ipsec only)")
	cmd.Flags().StringVar(&ipsecRemoteCIDR, "ipsec-remote-cidr", "", "Remote CIDR for IPsec tunnel (vpn_ipsec only)")
	// WireGuard flags
	cmd.Flags().StringVar(&wgPeerPublicKey, "wg-peer-public-key", "", "WireGuard peer public key (vpn_wireguard only)")
	cmd.Flags().StringVar(&wgPeerEndpoint, "wg-peer-endpoint", "", "WireGuard peer endpoint host:port (vpn_wireguard only)")
	cmd.Flags().StringArrayVar(&wgAllowedIPs, "wg-allowed-ips", nil, "Allowed IPs for WireGuard tunnel (vpn_wireguard only, repeatable)")
	cmd.Flags().IntVar(&wgListenPort, "wg-listen-port", 0, "WireGuard listen port (vpn_wireguard only)")
	// Direct Connect flags
	cmd.Flags().IntVar(&dcVLANID, "vlan-id", 0, "VLAN ID for Direct Connect trunk (direct_connect only, 1-4094)")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newEgressDeleteCmd() *cobra.Command {
	var networkStr, tenant, org string
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an egress rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, networkStr, tenantID)
			if err != nil {
				return err
			}
			egressID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid egress ID: %w", err)
			}
			if err := c.DeleteEgress(ctx, tenantID, networkID, egressID); err != nil {
				return err
			}
			return app.printStatus("Deleted", "egress", egressID.String())
		},
	}
	cmd.Flags().StringVar(&networkStr, "network", "", "Network (UUID or name) (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (UUID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization for tenant name resolution")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

// --- Ingress commands (tenant) ---

func (app *cli) newIngressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingress",
		Short: "Manage network ingress rules",
	}
	cmd.AddCommand(app.newIngressListCmd())
	cmd.AddCommand(app.newIngressCreateCmd())
	cmd.AddCommand(app.newIngressDeleteCmd())
	return cmd
}

func (app *cli) newIngressListCmd() *cobra.Command {
	var networkStr, tenant, org string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ingress rules for a network",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, networkStr, tenantID)
			if err != nil {
				return err
			}
			ingresses, err := c.ListIngresses(ctx, tenantID, networkID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(ingresses))
			for i, ing := range ingresses {
				rows[i] = []string{ing.ID.String(), ing.NetworkID.String(), ing.Type, ing.PublicIP, ing.Config.TargetIP}
			}
			return app.printTable([]string{"ID", "NETWORK_ID", "TYPE", "PUBLIC_IP", "TARGET_IP"}, rows)
		},
	}
	cmd.Flags().StringVar(&networkStr, "network", "", "Network (UUID or name) (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (UUID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization for tenant name resolution")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func (app *cli) newIngressCreateCmd() *cobra.Command {
	var networkStr, tenant, org, ingressType, publicIP, ipPoolStr, targetVMStr, targetIP string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an ingress rule",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, networkStr, tenantID)
			if err != nil {
				return err
			}
			poolID, err := c.ResolveIPPool(ctx, ipPoolStr)
			if err != nil {
				return err
			}
			// target-vm is stored as a string UUID in config; resolve or pass-through
			targetVMID := targetVMStr
			if _, parseErr := uuid.Parse(targetVMStr); parseErr != nil {
				return fmt.Errorf("--target-vm must be a valid UUID: %w", parseErr)
			}
			ing, err := c.CreateIngress(ctx, tenantID, networkID, network.IngressSpec{
				Type:     ingressType,
				PublicIP: publicIP,
				IPPoolID: poolID,
				Config: network.IngressConfig{
					TargetVMID: targetVMID,
					TargetIP:   targetIP,
				},
			})
			if err != nil {
				return err
			}
			return app.printTable(
				[]string{"ID", "NETWORK_ID", "TYPE", "PUBLIC_IP", "TARGET_IP"},
				[][]string{{ing.ID.String(), ing.NetworkID.String(), ing.Type, ing.PublicIP, ing.Config.TargetIP}},
			)
		},
	}
	cmd.Flags().StringVar(&networkStr, "network", "", "Network (UUID or name) (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (UUID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization for tenant name resolution")
	cmd.Flags().StringVar(&ingressType, "type", "direct_ip", "Ingress type (direct_ip)")
	cmd.Flags().StringVar(&publicIP, "public-ip", "", "Public IP for DNAT (required)")
	cmd.Flags().StringVar(&ipPoolStr, "ip-pool", "", "IP pool (UUID or name) (required)")
	cmd.Flags().StringVar(&targetVMStr, "target-vm", "", "Target VM UUID (required)")
	cmd.Flags().StringVar(&targetIP, "target-ip", "", "Target private IP (required)")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("public-ip")
	_ = cmd.MarkFlagRequired("ip-pool")
	_ = cmd.MarkFlagRequired("target-vm")
	_ = cmd.MarkFlagRequired("target-ip")
	return cmd
}

func (app *cli) newIngressDeleteCmd() *cobra.Command {
	var networkStr, tenant, org string
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an ingress rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := app.cmdContext()
			c := app.newClient()
			tenantID, err := app.resolveTenant(ctx, c, tenant, org)
			if err != nil {
				return err
			}
			networkID, err := c.ResolveNetwork(ctx, networkStr, tenantID)
			if err != nil {
				return err
			}
			ingressID, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid ingress ID: %w", err)
			}
			if err := c.DeleteIngress(ctx, tenantID, networkID, ingressID); err != nil {
				return err
			}
			return app.printStatus("Deleted", "ingress", ingressID.String())
		},
	}
	cmd.Flags().StringVar(&networkStr, "network", "", "Network (UUID or name) (required)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant (UUID or name) (required)")
	cmd.Flags().StringVar(&org, "org", "", "Organization for tenant name resolution")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}
