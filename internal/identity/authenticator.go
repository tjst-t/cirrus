package identity

import (
	"context"
	"strings"
)

// Authenticator resolves a bearer token to a User.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*User, error)
}

// StaticTokenAuth maps static tokens to user external IDs for development use.
// The Service is used to resolve the external ID to a full User record.
type StaticTokenAuth struct {
	// tokens maps bearer token → user external_id
	tokens  map[string]string
	service Service
}

// NewStaticTokenAuth creates an authenticator from a token→externalID map.
func NewStaticTokenAuth(tokens map[string]string, svc Service) *StaticTokenAuth {
	return &StaticTokenAuth{tokens: tokens, service: svc}
}

func (a *StaticTokenAuth) Authenticate(ctx context.Context, token string) (*User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrUnauthenticated
	}
	externalID, ok := a.tokens[token]
	if !ok {
		return nil, ErrTokenInvalid
	}
	user, err := a.service.GetUserByExternalID(ctx, externalID)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	return user, nil
}
