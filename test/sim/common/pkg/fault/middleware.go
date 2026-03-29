package fault

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Middleware returns an HTTP middleware that applies fault injection.
// The simulator name identifies which simulator this middleware protects.
// The operation is derived from the HTTP method + path.
func Middleware(engine *Engine, simulator string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip management API paths
			if strings.HasPrefix(r.URL.Path, "/sim/") || r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			operation := r.Method + " " + r.URL.Path

			// Extract host_id from headers or path if available
			hostID := r.Header.Get("X-Host-Id")

			result := engine.Check(context.Background(), simulator, hostID, operation)
			if result == nil {
				next.ServeHTTP(w, r)
				return
			}

			switch result.Type {
			case FaultError:
				code := result.ErrorCode
				if code == 0 {
					code = http.StatusInternalServerError
				}
				msg := result.ErrorMessage
				if msg == "" {
					msg = "injected fault"
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				json.NewEncoder(w).Encode(map[string]string{"error": msg, "fault": "injected"})
				return

			case FaultDelay:
				if result.DelayMs > 0 {
					time.Sleep(time.Duration(result.DelayMs) * time.Millisecond)
				}
				next.ServeHTTP(w, r)
				return

			case FaultTimeout:
				delay := result.TimeoutMs
				if delay <= 0 {
					delay = 30000
				}
				time.Sleep(time.Duration(delay) * time.Millisecond)
				return

			case FaultPartialFailure:
				// TODO: Implement partial failure by wrapping ResponseWriter
			// to truncate the response body mid-stream. For now, pass through.
				next.ServeHTTP(w, r)
				return

			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}
