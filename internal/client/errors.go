package client

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tjst-t/cirrus/internal/apierror"
)

// APIError は構造化 API エラーレスポンスを表します。
type APIError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Detail  json.RawMessage `json:"detail,omitempty"`
}

func (e *APIError) Error() string {
	return FormatAPIError(e)
}

// APIErrorDetail は detail フィールドの内容です。
// クォータ超過時は Resource/Limit/Requested/Current が設定される。
// ERR_INVALID_STATE 時は Reason が設定される。
type APIErrorDetail struct {
	Resource  string `json:"resource,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Requested int    `json:"requested,omitempty"`
	Current   int    `json:"current,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// FormatAPIError は APIError を日本語の人間可読メッセージに変換します。
// メッセージ文字列は internal/apierror/messages.json から読み込まれます。
func FormatAPIError(e *APIError) string {
	switch e.Code {
	case "ERR_NO_HOST":
		return apierror.Messages.ERRNoHost

	case "ERR_INSUFFICIENT_RESOURCES":
		return apierror.Messages.ERRInsufficientResources

	case "ERR_QUOTA_VCPU", "ERR_QUOTA_MEMORY", "ERR_QUOTA_VOLUME_GB",
		"ERR_QUOTA_VM_COUNT", "ERR_QUOTA_VOLUME_COUNT", "ERR_QUOTA_SNAPSHOT_COUNT",
		"ERR_QUOTA_NETWORK_COUNT", "ERR_QUOTA_EGRESS_COUNT", "ERR_QUOTA_INGRESS_COUNT",
		"ERR_QUOTA_EXCEEDED":
		var d APIErrorDetail
		if e.Detail != nil && json.Unmarshal(e.Detail, &d) == nil && d.Resource != "" {
			msg := apierror.Messages.ERRQuota.WithDetail
			msg = strings.ReplaceAll(msg, "{resource}", d.Resource)
			msg = strings.ReplaceAll(msg, "{current}", fmt.Sprintf("%d", d.Current))
			msg = strings.ReplaceAll(msg, "{limit}", fmt.Sprintf("%d", d.Limit))
			return msg
		}
		return apierror.Messages.ERRQuota.Fallback

	case "ERR_INVALID_STATE":
		var d APIErrorDetail
		if e.Detail != nil {
			_ = json.Unmarshal(e.Detail, &d)
		}
		if msg, ok := apierror.Messages.ERRInvalidState[d.Reason]; ok {
			return msg
		}
		return apierror.Messages.ERRInvalidState["default"]

	case "ERR_CONFLICT":
		return apierror.Messages.ERRConflict

	case "ERR_NOT_FOUND":
		return apierror.Messages.ERRNotFound

	case "ERR_UNAUTHORIZED":
		return apierror.Messages.ERRUnauthorized.CLI

	case "ERR_FORBIDDEN":
		return apierror.Messages.ERRForbidden

	case "ERR_BAD_REQUEST":
		if e.Message != "" {
			return strings.ReplaceAll(apierror.Messages.ERRBadRequest.WithMessage, "{message}", e.Message)
		}
		return apierror.Messages.ERRBadRequest.Fallback

	default:
		if e.Message != "" {
			return e.Message
		}
		return fmt.Sprintf("エラーが発生しました（コード: %s）", e.Code)
	}
}
