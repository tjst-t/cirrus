package fault

import (
	"context"
	"testing"
	"time"
)

func TestAddAndGetRules(t *testing.T) {
	e := New()
	ctx := context.Background()

	id := e.AddRule(ctx, FaultRule{
		Target:  Target{Simulator: "libvirt-sim", Operation: "migrate"},
		Trigger: Trigger{Type: TriggerProbabilistic, Probability: 1.0},
		Fault:   Fault{Type: FaultError, ErrorCode: -1, ErrorMessage: "fail"},
	})

	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	rules := e.GetRules(ctx)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].ID != id {
		t.Errorf("ID = %q, want %q", rules[0].ID, id)
	}
}

func TestDeleteRule(t *testing.T) {
	e := New()
	ctx := context.Background()

	id := e.AddRule(ctx, FaultRule{
		Trigger: Trigger{Type: TriggerProbabilistic, Probability: 1.0},
		Fault:   Fault{Type: FaultError},
	})

	if err := e.DeleteRule(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(e.GetRules(ctx)) != 0 {
		t.Error("expected empty rules after delete")
	}

	if err := e.DeleteRule(ctx, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent rule")
	}
}

func TestClearRules(t *testing.T) {
	e := New()
	ctx := context.Background()

	e.AddRule(ctx, FaultRule{Trigger: Trigger{Type: TriggerProbabilistic, Probability: 1.0}, Fault: Fault{Type: FaultError}})
	e.AddRule(ctx, FaultRule{Trigger: Trigger{Type: TriggerProbabilistic, Probability: 1.0}, Fault: Fault{Type: FaultDelay}})

	e.ClearRules(ctx)
	if len(e.GetRules(ctx)) != 0 {
		t.Error("expected empty rules after clear")
	}
}

func TestCheckProbabilistic(t *testing.T) {
	tests := []struct {
		name        string
		probability float64
		expectFault bool
	}{
		{"always", 1.0, true},
		{"never", 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			ctx := context.Background()

			e.AddRule(ctx, FaultRule{
				Target:  Target{Simulator: "storage-sim"},
				Trigger: Trigger{Type: TriggerProbabilistic, Probability: tt.probability},
				Fault:   Fault{Type: FaultError, ErrorCode: 42, ErrorMessage: "test error"},
			})

			result := e.Check(ctx, "storage-sim", "", "create")
			if tt.expectFault && result == nil {
				t.Error("expected fault, got nil")
			}
			if !tt.expectFault && result != nil {
				t.Errorf("expected no fault, got %+v", result)
			}
			if tt.expectFault && result != nil {
				if result.Type != FaultError {
					t.Errorf("type = %q, want %q", result.Type, FaultError)
				}
				if result.ErrorCode != 42 {
					t.Errorf("error_code = %d, want 42", result.ErrorCode)
				}
			}
		})
	}
}

func TestCheckTargetMatching(t *testing.T) {
	e := New()
	ctx := context.Background()

	e.AddRule(ctx, FaultRule{
		Target:  Target{Simulator: "libvirt-sim", HostID: "host-1"},
		Trigger: Trigger{Type: TriggerProbabilistic, Probability: 1.0},
		Fault:   Fault{Type: FaultError, ErrorMessage: "fail"},
	})

	// Matching
	if r := e.Check(ctx, "libvirt-sim", "host-1", "start"); r == nil {
		t.Error("expected fault for matching target")
	}

	// Wrong simulator
	if r := e.Check(ctx, "storage-sim", "host-1", "start"); r != nil {
		t.Error("expected no fault for wrong simulator")
	}

	// Wrong host
	if r := e.Check(ctx, "libvirt-sim", "host-2", "start"); r != nil {
		t.Error("expected no fault for wrong host")
	}
}

func TestCheckDelayFault(t *testing.T) {
	e := New()
	ctx := context.Background()

	e.AddRule(ctx, FaultRule{
		Trigger: Trigger{Type: TriggerProbabilistic, Probability: 1.0},
		Fault:   Fault{Type: FaultDelay, MinMs: 100, MaxMs: 200},
	})

	result := e.Check(ctx, "", "", "")
	if result == nil {
		t.Fatal("expected fault")
	}
	if result.Type != FaultDelay {
		t.Errorf("type = %q, want delay", result.Type)
	}
	if result.DelayMs < 100 || result.DelayMs >= 200 {
		t.Errorf("delay = %d, expected [100, 200)", result.DelayMs)
	}
}

func TestCheckCountTrigger(t *testing.T) {
	e := New()
	ctx := context.Background()

	e.AddRule(ctx, FaultRule{
		Trigger: Trigger{Type: TriggerCount, AfterCount: 3, Repeat: false},
		Fault:   Fault{Type: FaultError, ErrorMessage: "count"},
	})

	// First two calls: no fault
	for i := 0; i < 2; i++ {
		if r := e.Check(ctx, "", "", ""); r != nil {
			t.Errorf("call %d: expected no fault", i+1)
		}
	}

	// Third call: fault
	if r := e.Check(ctx, "", "", ""); r == nil {
		t.Error("call 3: expected fault")
	}

	// Fourth call: no fault (repeat=false)
	if r := e.Check(ctx, "", "", ""); r != nil {
		t.Error("call 4: expected no fault (repeat=false)")
	}
}

func TestCheckTimeTrigger(t *testing.T) {
	e := New()
	ctx := context.Background()

	now := time.Now()
	past := now.Add(-1 * time.Hour)
	e.AddRule(ctx, FaultRule{
		Trigger: Trigger{Type: TriggerTime, ActivateAt: &past, DurationSec: 7200},
		Fault:   Fault{Type: FaultTimeout, AfterMs: 5000},
	})

	result := e.Check(ctx, "", "", "")
	if result == nil {
		t.Fatal("expected fault within time window")
	}
	if result.Type != FaultTimeout {
		t.Errorf("type = %q, want timeout", result.Type)
	}
}

func TestHitCountTracking(t *testing.T) {
	e := New()
	ctx := context.Background()

	e.AddRule(ctx, FaultRule{
		Trigger: Trigger{Type: TriggerProbabilistic, Probability: 1.0},
		Fault:   Fault{Type: FaultError},
	})

	e.Check(ctx, "", "", "")
	e.Check(ctx, "", "", "")

	rules := e.GetRules(ctx)
	if rules[0].HitCount != 2 {
		t.Errorf("hit_count = %d, want 2", rules[0].HitCount)
	}
}
