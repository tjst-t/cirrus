// Package fault provides a fault injection engine for simulators.
package fault

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// FaultType represents the type of fault to inject.
type FaultType string

const (
	// FaultError causes an API call to return an error.
	FaultError FaultType = "error"
	// FaultDelay adds latency to an API call.
	FaultDelay FaultType = "delay"
	// FaultTimeout causes an API call to not respond.
	FaultTimeout FaultType = "timeout"
	// FaultPartialFailure causes an operation to fail midway.
	FaultPartialFailure FaultType = "partial_failure"
)

// TriggerType represents when a fault should fire.
type TriggerType string

const (
	// TriggerProbabilistic fires with a given probability.
	TriggerProbabilistic TriggerType = "probabilistic"
	// TriggerCount fires after N calls.
	TriggerCount TriggerType = "count"
	// TriggerTime fires during a time window.
	TriggerTime TriggerType = "time"
)

// Target specifies which operations a fault rule applies to.
type Target struct {
	Simulator string `json:"simulator,omitempty"`
	HostID    string `json:"host_id,omitempty"`
	Operation string `json:"operation,omitempty"`
}

// Trigger defines when a fault fires.
type Trigger struct {
	Type        TriggerType `json:"type"`
	Probability float64     `json:"probability,omitempty"`
	AfterCount  int         `json:"after_count,omitempty"`
	Repeat      bool        `json:"repeat,omitempty"`
	ActivateAt  *time.Time  `json:"activate_at,omitempty"`
	DurationSec int         `json:"duration_sec,omitempty"`
}

// Fault defines what fault to inject.
type Fault struct {
	Type         FaultType `json:"type"`
	ErrorCode    int       `json:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	MinMs        int       `json:"min_ms,omitempty"`
	MaxMs        int       `json:"max_ms,omitempty"`
	AfterMs      int       `json:"after_ms,omitempty"`
	FailPercent  int       `json:"fail_at_percent,omitempty"`
}

// FaultRule is a complete fault injection rule.
type FaultRule struct {
	ID        string    `json:"id"`
	Target    Target    `json:"target"`
	Trigger   Trigger   `json:"trigger"`
	Fault     Fault     `json:"fault"`
	HitCount  int       `json:"hit_count"`
	CallCount int       `json:"call_count"`
	CreatedAt time.Time `json:"created_at"`
}

// FaultResult is the result of checking a fault rule.
type FaultResult struct {
	Type         FaultType `json:"type"`
	ErrorCode    int       `json:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	DelayMs      int       `json:"delay_ms,omitempty"`
	TimeoutMs    int       `json:"timeout_ms,omitempty"`
	FailPercent  int       `json:"fail_percent,omitempty"`
}

// Engine manages fault injection rules and evaluates them.
type Engine struct {
	mu      sync.RWMutex
	rules   map[string]*FaultRule
	counter int
	rng     *rand.Rand
}

// New creates a new fault injection Engine.
func New() *Engine {
	return &Engine{
		rules: make(map[string]*FaultRule),
		//nolint:gosec // G404: weak random is fine for fault injection simulation
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// AddRule adds a fault rule and returns its assigned ID.
func (e *Engine) AddRule(_ context.Context, rule FaultRule) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.counter++
	rule.ID = fmt.Sprintf("fault-%03d", e.counter)
	rule.CreatedAt = time.Now().UTC()
	rule.HitCount = 0
	stored := rule
	e.rules[rule.ID] = &stored
	return rule.ID
}

// GetRules returns all fault rules.
func (e *Engine) GetRules(_ context.Context) []FaultRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]FaultRule, 0, len(e.rules))
	for _, r := range e.rules {
		result = append(result, *r)
	}
	return result
}

// DeleteRule removes a fault rule by ID.
func (e *Engine) DeleteRule(_ context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.rules[id]; !ok {
		return fmt.Errorf("delete rule %q: rule not found", id)
	}
	delete(e.rules, id)
	return nil
}

// ClearRules removes all fault rules.
func (e *Engine) ClearRules(_ context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = make(map[string]*FaultRule)
}

// Check evaluates all matching rules and returns the first fault to apply, or nil.
func (e *Engine) Check(_ context.Context, simulator, hostID, operation string) *FaultResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	for _, rule := range e.rules {
		if !matchTarget(rule.Target, simulator, hostID, operation) {
			continue
		}

		rule.CallCount++
		if !e.shouldTrigger(rule, now) {
			continue
		}

		rule.HitCount++
		return toResult(rule.Fault, e.rng)
	}

	return nil
}

func matchTarget(t Target, simulator, hostID, operation string) bool {
	if t.Simulator != "" && t.Simulator != simulator {
		return false
	}
	if t.HostID != "" && t.HostID != hostID {
		return false
	}
	if t.Operation != "" && t.Operation != operation {
		return false
	}
	return true
}

func (e *Engine) shouldTrigger(rule *FaultRule, now time.Time) bool {
	switch rule.Trigger.Type {
	case TriggerProbabilistic:
		return e.rng.Float64() < rule.Trigger.Probability
	case TriggerCount:
		if rule.CallCount >= rule.Trigger.AfterCount {
			if rule.CallCount == rule.Trigger.AfterCount {
				return true
			}
			return rule.Trigger.Repeat
		}
		return false
	case TriggerTime:
		if rule.Trigger.ActivateAt == nil {
			return false
		}
		start := *rule.Trigger.ActivateAt
		end := start.Add(time.Duration(rule.Trigger.DurationSec) * time.Second)
		return now.After(start) && now.Before(end)
	default:
		return false
	}
}

func toResult(f Fault, rng *rand.Rand) *FaultResult {
	result := &FaultResult{Type: f.Type}
	switch f.Type {
	case FaultError:
		result.ErrorCode = f.ErrorCode
		result.ErrorMessage = f.ErrorMessage
	case FaultDelay:
		if f.MaxMs > f.MinMs {
			result.DelayMs = f.MinMs + rng.Intn(f.MaxMs-f.MinMs)
		} else {
			result.DelayMs = f.MinMs
		}
	case FaultTimeout:
		result.TimeoutMs = f.AfterMs
	case FaultPartialFailure:
		result.FailPercent = f.FailPercent
	}
	return result
}
