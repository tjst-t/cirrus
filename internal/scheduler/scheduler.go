package scheduler

import (
	"context"
	"fmt"

	"github.com/tjst-t/cirrus/internal/state"
)

type Scheduler struct {
	db *state.DB
}

func New(db *state.DB) *Scheduler {
	return &Scheduler{db: db}
}

// Schedule picks the best worker for a VM based on available resources.
// Uses a simple spread strategy: pick the worker with the most free resources.
func (s *Scheduler) Schedule(ctx context.Context, vcpus int, ramMB int, diskGB int) (*state.Worker, error) {
	caps, err := s.db.GetWorkerCapacities(ctx)
	if err != nil {
		return nil, fmt.Errorf("get worker capacities: %w", err)
	}
	if len(caps) == 0 {
		return nil, fmt.Errorf("no online workers available")
	}

	var best *state.WorkerCapacity
	bestScore := -1.0

	for i := range caps {
		wc := &caps[i]
		freeVCPUs := wc.Worker.TotalVCPUs - wc.UsedVCPUs
		freeRamMB := wc.Worker.TotalRamMB - wc.UsedRamMB
		freeDiskGB := wc.Worker.TotalDiskGB - wc.UsedDiskGB

		if freeVCPUs < vcpus || freeRamMB < ramMB || freeDiskGB < diskGB {
			continue
		}

		// Score: sum of free resource ratios (higher = more free resources)
		score := float64(freeVCPUs)/float64(wc.Worker.TotalVCPUs) +
			float64(freeRamMB)/float64(wc.Worker.TotalRamMB) +
			float64(freeDiskGB)/float64(wc.Worker.TotalDiskGB)

		if score > bestScore {
			bestScore = score
			best = wc
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no worker with sufficient resources (need %d vCPUs, %d MB RAM, %d GB disk)", vcpus, ramMB, diskGB)
	}

	return &best.Worker, nil
}
