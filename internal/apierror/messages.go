package apierror

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed messages.json
var messagesJSON []byte

// Messages holds the parsed error message strings loaded from messages.json.
// Both the CLI client and the web frontend read from the same source file.
var Messages struct {
	ERRNoHost                string            `json:"ERR_NO_HOST"`
	ERRInsufficientResources string            `json:"ERR_INSUFFICIENT_RESOURCES"`
	ERRQuota                 struct {
		WithDetail string `json:"with_detail"`
		Fallback   string `json:"fallback"`
	} `json:"ERR_QUOTA"`
	ERRInvalidState map[string]string `json:"ERR_INVALID_STATE"`
	ERRConflict     string            `json:"ERR_CONFLICT"`
	ERRNotFound     string            `json:"ERR_NOT_FOUND"`
	ERRUnauthorized struct {
		CLI string `json:"cli"`
		Web string `json:"web"`
	} `json:"ERR_UNAUTHORIZED"`
	ERRForbidden  string `json:"ERR_FORBIDDEN"`
	ERRBadRequest struct {
		WithMessage string `json:"with_message"`
		Fallback    string `json:"fallback"`
	} `json:"ERR_BAD_REQUEST"`
}

func init() {
	if err := json.Unmarshal(messagesJSON, &Messages); err != nil {
		panic(fmt.Sprintf("apierror: failed to parse messages.json: %v", err))
	}
}
