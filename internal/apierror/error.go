package apierror

// ErrorResponse は API エラーレスポンスの共通形式です。
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}
