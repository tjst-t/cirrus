package fencing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SimFencingAgent implements FencingAgent using the cirrus-sim management API.
// Used in development and testing instead of real IPMI.
type SimFencingAgent struct {
	simURL     string
	httpClient *http.Client
}

// NewSimFencingAgent creates a SimFencingAgent targeting the given cirrus-sim URL.
// timeout is the maximum time to wait for fencing to complete.
func NewSimFencingAgent(simURL string, timeout time.Duration) *SimFencingAgent {
	return &SimFencingAgent{
		simURL:     simURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Fence calls POST /sim/hosts/{hostID}/power-off on the cirrus-sim management API.
// Returns an error if the request fails, times out, or the response status is not 200.
func (a *SimFencingAgent) Fence(ctx context.Context, hostID uuid.UUID) error {
	url := fmt.Sprintf("%s/sim/hosts/%s/power-off", a.simURL, hostID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("fencing: create request: %w", err)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fencing: power-off request failed: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fencing: power-off returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
