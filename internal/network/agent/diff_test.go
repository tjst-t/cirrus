package agent

import (
	"testing"
)

func TestDiffFlows_NoChange(t *testing.T) {
	flows := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "drop"},
		{Table: 1, Priority: 50, Match: "ct_state=-trk", Actions: "ct(table=1)"},
	}

	add, del := DiffFlows(flows, flows)
	if len(add) != 0 {
		t.Errorf("expected no additions, got %d", len(add))
	}
	if len(del) != 0 {
		t.Errorf("expected no deletions, got %d", len(del))
	}
}

func TestDiffFlows_AddNew(t *testing.T) {
	current := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "drop"},
	}
	desired := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "drop"},
		{Table: 0, Priority: 100, Match: "in_port=2", Actions: "drop"},
	}

	add, del := DiffFlows(current, desired)
	if len(add) != 1 {
		t.Errorf("expected 1 addition, got %d", len(add))
	}
	if len(del) != 0 {
		t.Errorf("expected no deletions, got %d", len(del))
	}
	if len(add) > 0 && add[0].Match != "in_port=2" {
		t.Errorf("expected add in_port=2, got %s", add[0].Match)
	}
}

func TestDiffFlows_RemoveOld(t *testing.T) {
	current := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "drop"},
		{Table: 0, Priority: 100, Match: "in_port=2", Actions: "drop"},
	}
	desired := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "drop"},
	}

	add, del := DiffFlows(current, desired)
	if len(add) != 0 {
		t.Errorf("expected no additions, got %d", len(add))
	}
	if len(del) != 1 {
		t.Errorf("expected 1 deletion, got %d", len(del))
	}
}

func TestDiffFlows_ModifyActions(t *testing.T) {
	current := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "output:2"},
	}
	desired := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "output:3"},
	}

	add, del := DiffFlows(current, desired)
	if len(add) != 1 {
		t.Errorf("expected 1 addition (overwrite), got %d", len(add))
	}
	if len(del) != 1 {
		t.Errorf("expected 1 deletion (old version), got %d", len(del))
	}
}

func TestDiffFlows_EmptyToFull(t *testing.T) {
	desired := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "drop"},
		{Table: 1, Priority: 50, Match: "ct_state=-trk", Actions: "ct(table=1)"},
	}

	add, del := DiffFlows(nil, desired)
	if len(add) != 2 {
		t.Errorf("expected 2 additions, got %d", len(add))
	}
	if len(del) != 0 {
		t.Errorf("expected no deletions, got %d", len(del))
	}
}

func TestDiffFlows_FullToEmpty(t *testing.T) {
	current := []FlowEntry{
		{Table: 0, Priority: 100, Match: "in_port=1", Actions: "drop"},
	}

	add, del := DiffFlows(current, nil)
	if len(add) != 0 {
		t.Errorf("expected no additions, got %d", len(add))
	}
	if len(del) != 1 {
		t.Errorf("expected 1 deletion, got %d", len(del))
	}
}
