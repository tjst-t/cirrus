package client_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tjst-t/cirrus/internal/client"
)

func TestFormatAPIError(t *testing.T) {
	tests := []struct {
		name         string
		err          client.APIError
		wantContains string
	}{
		{
			name:         "ERR_NO_HOST",
			err:          client.APIError{Code: "ERR_NO_HOST", Message: "no host"},
			wantContains: "利用可能なホストがありません",
		},
		{
			name: "ERR_QUOTA_VCPU with detail",
			err: client.APIError{
				Code:    "ERR_QUOTA_VCPU",
				Message: "quota exceeded",
				Detail:  mustJSON(map[string]any{"resource": "vcpu", "limit": 8, "current": 7, "requested": 2}),
			},
			wantContains: "vcpu: 7/8",
		},
		{
			name:         "ERR_QUOTA_EXCEEDED without detail",
			err:          client.APIError{Code: "ERR_QUOTA_EXCEEDED", Message: "quota exceeded"},
			wantContains: "クォータ上限に達しています",
		},
		{
			name:         "ERR_CONFLICT",
			err:          client.APIError{Code: "ERR_CONFLICT", Message: "conflict"},
			wantContains: "同じ名前のリソース",
		},
		{
			name:         "ERR_NOT_FOUND",
			err:          client.APIError{Code: "ERR_NOT_FOUND", Message: "not found"},
			wantContains: "リソースが見つかりません",
		},
		{
			name:         "ERR_UNAUTHORIZED",
			err:          client.APIError{Code: "ERR_UNAUTHORIZED", Message: "unauthorized"},
			wantContains: "--token",
		},
		{
			name:         "ERR_FORBIDDEN",
			err:          client.APIError{Code: "ERR_FORBIDDEN", Message: "forbidden"},
			wantContains: "権限がありません",
		},
		{
			name:         "ERR_BAD_REQUEST with message",
			err:          client.APIError{Code: "ERR_BAD_REQUEST", Message: "invalid field"},
			wantContains: "invalid field",
		},
		{
			name:         "ERR_BAD_REQUEST without message",
			err:          client.APIError{Code: "ERR_BAD_REQUEST"},
			wantContains: "リクエストが不正です",
		},
		{
			name:         "ERR_INSUFFICIENT_RESOURCES",
			err:          client.APIError{Code: "ERR_INSUFFICIENT_RESOURCES", Message: "not enough"},
			wantContains: "リソースが不足",
		},
		{
			name:         "unknown code falls back to message",
			err:          client.APIError{Code: "ERR_UNKNOWN_FUTURE", Message: "some future error"},
			wantContains: "some future error",
		},
		{
			name:         "unknown code without message falls back to code",
			err:          client.APIError{Code: "ERR_UNKNOWN_FUTURE"},
			wantContains: "ERR_UNKNOWN_FUTURE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.FormatAPIError(&tt.err)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("FormatAPIError() = %q, want to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestAPIError_Error(t *testing.T) {
	e := &client.APIError{Code: "ERR_NO_HOST", Message: "no host available"}
	got := e.Error()
	if !strings.Contains(got, "利用可能なホストがありません") {
		t.Errorf("APIError.Error() = %q, expected Japanese message", got)
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
