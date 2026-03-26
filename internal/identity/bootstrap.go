package identity

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
)

// BootstrapUsers ensures that users referenced by static auth tokens exist in the DB.
// For each externalID, it creates the user if not found, and assigns infra_admin if
// the user has no role assignments (first-time bootstrap).
func BootstrapUsers(ctx context.Context, svc Service, externalIDs []string, logger *slog.Logger) {
	for _, eid := range externalIDs {
		user, err := svc.GetUserByExternalID(ctx, eid)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				logger.Warn("bootstrap: failed to look up user", "external_id", eid, "error", err)
				continue
			}
			// Create user
			user, err = svc.CreateUser(ctx, eid, eid, "")
			if err != nil {
				logger.Warn("bootstrap: failed to create user", "external_id", eid, "error", err)
				continue
			}
			logger.Info("bootstrap: created user", "id", user.ID, "external_id", eid)
		}

		// If user has no roles, assign infra_admin for dev convenience
		roles, err := svc.ListRoleAssignments(ctx, user.ID)
		if err != nil {
			logger.Warn("bootstrap: failed to list roles", "user_id", user.ID, "error", err)
			continue
		}
		if len(roles) == 0 {
			var nilScope *uuid.UUID
			_, err := svc.AssignRole(ctx, user.ID, ScopeGlobal, nilScope, RoleInfraAdmin)
			if err != nil {
				logger.Warn("bootstrap: failed to assign infra_admin", "user_id", user.ID, "error", err)
				continue
			}
			logger.Info("bootstrap: assigned infra_admin", "user_id", user.ID, "external_id", eid)
		}
	}
}
