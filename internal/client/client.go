package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Client is an HTTP API client for the Cirrus controller.
type Client struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

// New creates a new Cirrus API client.
func New(endpoint, token string) *Client {
	return &Client{
		endpoint:   endpoint,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// do performs an HTTP request with authentication and returns the response.
func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	return c.doWithHeaders(ctx, method, path, body, nil)
}

// doWithHeaders performs an HTTP request with additional headers.
func (c *Client) doWithHeaders(ctx context.Context, method, path string, body any, headers map[string]string) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", method, path, err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

		var payload struct {
			Code    string          `json:"code"`
			Message string          `json:"message"`
			Detail  json.RawMessage `json:"detail,omitempty"`
			Error   string          `json:"error"` // 旧形式フォールバック
		}
		if json.Unmarshal(respBody, &payload) == nil {
			if payload.Code != "" {
				return nil, &APIError{Code: payload.Code, Message: payload.Message, Detail: payload.Detail}
			}
			if payload.Error != "" {
				return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, payload.Error)
			}
		}

		return nil, fmt.Errorf("API error (%d)", resp.StatusCode)
	}

	return resp, nil
}

// doWithTenant performs an HTTP request with the X-Tenant-ID header set.
func (c *Client) doWithTenant(ctx context.Context, method, path string, body any, tenantID uuid.UUID) (*http.Response, error) {
	return c.doWithHeaders(ctx, method, path, body, map[string]string{
		"X-Tenant-ID": tenantID.String(),
	})
}

// decodeResponse reads and decodes a JSON response body.
func decodeResponse[T any](resp *http.Response) (T, error) {
	defer resp.Body.Close()
	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// decodePagedResponse decodes a PagedResponse envelope and returns all items,
// collecting pages automatically until next_cursor is empty.
func decodePagedResponse[T any](resp *http.Response) ([]T, error) {
	defer resp.Body.Close()
	var envelope struct {
		Items      []T    `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return envelope.Items, nil
}
