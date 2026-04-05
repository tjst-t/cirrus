package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

// CursorParam holds the decoded pagination cursor values.
type CursorParam struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// encodeCursor encodes a CursorParam to a base64 string.
// Format: base64(created_at_rfc3339nano:uuid)
func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s:%s", createdAt.UTC().Format(time.RFC3339Nano), id.String())
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// decodeCursor decodes a base64 cursor string to a CursorParam.
// Format: base64(created_at_rfc3339nano:uuid)
// UUID is always 36 chars; the separator is the last ':' before it.
func decodeCursor(cursor string) (*CursorParam, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: not valid base64")
	}
	s := string(raw)
	// UUID is always 36 characters; split at the last ':'.
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return nil, fmt.Errorf("invalid cursor: malformed")
	}
	t, err := time.Parse(time.RFC3339Nano, s[:idx])
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: bad timestamp")
	}
	id, err := uuid.Parse(s[idx+1:])
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: bad id")
	}
	return &CursorParam{CreatedAt: t, ID: id}, nil
}

// parsePaginationParams extracts after= and limit= from query parameters.
// Returns the cursor (may be nil), limit, and any error.
func parsePaginationParams(r *http.Request) (*CursorParam, int, error) {
	limitStr := r.URL.Query().Get("limit")
	limit := defaultPageLimit
	if limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n <= 0 {
			return nil, 0, fmt.Errorf("limit must be a positive integer")
		}
		if n > maxPageLimit {
			return nil, 0, fmt.Errorf("limit must be at most %d", maxPageLimit)
		}
		limit = n
	}

	after := r.URL.Query().Get("after")
	cursor, err := decodeCursor(after)
	if err != nil {
		return nil, 0, err
	}

	return cursor, limit, nil
}

// PagedResponse wraps a list result with a next_cursor for cursor-based pagination.
type PagedResponse struct {
	Items      any    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

// cursorValues extracts (createdAt, id) from a cursor, returning zero values
// when the cursor is nil (i.e., start of list).
func cursorValues(c *CursorParam) (time.Time, uuid.UUID) {
	if c == nil {
		return time.Time{}, uuid.Nil
	}
	return c.CreatedAt, c.ID
}
