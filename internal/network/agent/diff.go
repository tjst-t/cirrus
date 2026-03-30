package agent

import "fmt"

// flowKey uniquely identifies a flow entry by table, priority, and match.
func flowKey(f FlowEntry) string {
	return fmt.Sprintf("%d/%d/%s", f.Table, f.Priority, f.Match)
}

// DiffFlows computes the set of flows to add and delete to transition
// from current state to desired state.
func DiffFlows(current, desired []FlowEntry) (add, del []FlowEntry) {
	currentMap := make(map[string]FlowEntry, len(current))
	for _, f := range current {
		currentMap[flowKey(f)] = f
	}

	desiredMap := make(map[string]FlowEntry, len(desired))
	for _, f := range desired {
		desiredMap[flowKey(f)] = f
	}

	// Flows in desired but not in current → add
	// Flows in desired with different actions → add (overwrite)
	for key, df := range desiredMap {
		cf, exists := currentMap[key]
		if !exists || cf.Actions != df.Actions {
			add = append(add, df)
		}
	}

	// Flows in current but not in desired → delete
	// Flows in current with different actions from desired → delete (will be re-added)
	for key, cf := range currentMap {
		df, exists := desiredMap[key]
		if !exists || cf.Actions != df.Actions {
			del = append(del, cf)
		}
	}

	return add, del
}
