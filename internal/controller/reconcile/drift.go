package reconcile

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DriftEvent represents a detected deviation between desired and actual state.
type DriftEvent struct {
	ID         uuid.UUID
	Layer      string // "compute", "network", "storage", "host"
	Type       string // "expected_missing", "unexpected_present", "state_mismatch", ...
	Severity   string // "critical", "high", "medium"
	Resource   string // "vm", "port", "volume", "flow", ...
	ResourceID string
	TenantID   string // empty for infra-level resources
	HostID     string // empty if not host-specific
	Expected   string // DB-side state
	Actual     string // observed state
	DetectedBy string // "heartbeat_reconciler", "network_reconciler", etc.
	Action     string // set by DriftHandler after processing
	Timestamp  time.Time
}

// Layer constants.
const (
	DriftLayerCompute = "compute"
	DriftLayerNetwork = "network"
	DriftLayerStorage = "storage"
	DriftLayerHost    = "host"
)

// Type constants.
const (
	DriftTypeExpectedMissing   = "expected_missing"
	DriftTypeUnexpectedPresent = "unexpected_present"
	DriftTypeStateMismatch     = "state_mismatch"
	DriftTypeHeartbeatTimeout  = "heartbeat_timeout"
	DriftTypeHostFaultCascade  = "host_fault_cascade"
)

// Severity constants.
const (
	DriftSeverityCritical = "critical"
	DriftSeverityHigh     = "high"
	DriftSeverityMedium   = "medium"
)

// Action constants.
const (
	DriftActionAlert    = "alert"
	DriftActionAutoHeal = "auto_heal"
)

// VMHealer is implemented by components that can auto-heal VM state.
type VMHealer interface {
	HealVM(ctx context.Context, vmID uuid.UUID, reason string) error
}

// VMRecoverer is implemented by components that can recover a VM from error
// back to an observed-actual state (e.g. when a restarted worker re-reports
// a VM that was previously marked error due to expected_missing).
type VMRecoverer interface {
	RecoverVM(ctx context.Context, vmID uuid.UUID, newStatus string) error
}

// NetworkHealer is implemented by components that can re-deliver network state.
type NetworkHealer interface {
	TriggerRefresh(hostID uuid.UUID)
}

// DriftHandlerConfig holds configuration for a DriftHandler.
type DriftHandlerConfig struct {
	Pool            *pgxpool.Pool
	Logger          *slog.Logger
	AutoHealEnabled bool
	DedupTTL        time.Duration // 0 = 10 minutes default
	VMHealer        VMHealer
	VMRecoverer     VMRecoverer
	NetworkHealer   NetworkHealer
}

// DriftHandler processes DriftEvents with deduplication, logging, DB persistence,
// and optional auto-heal actions.
type DriftHandler struct {
	pool            *pgxpool.Pool
	logger          *slog.Logger
	autoHealEnabled bool
	dedupTTL        time.Duration

	mu    sync.Mutex
	dedup map[string]time.Time // "resource_id:type" → last accepted time

	vmHealer      VMHealer
	vmRecoverer   VMRecoverer
	networkHealer NetworkHealer
}

// NewDriftHandler creates a new DriftHandler.
func NewDriftHandler(cfg DriftHandlerConfig) *DriftHandler {
	dedupTTL := cfg.DedupTTL
	if dedupTTL <= 0 {
		dedupTTL = 10 * time.Minute
	}
	return &DriftHandler{
		pool:            cfg.Pool,
		logger:          cfg.Logger.With("component", "drift-handler"),
		autoHealEnabled: cfg.AutoHealEnabled,
		dedupTTL:        dedupTTL,
		dedup:           make(map[string]time.Time),
		vmHealer:        cfg.VMHealer,
		vmRecoverer:     cfg.VMRecoverer,
		networkHealer:   cfg.NetworkHealer,
	}
}

// Handle processes a DriftEvent: deduplicates, executes action, logs, and persists.
func (h *DriftHandler) Handle(ctx context.Context, event DriftEvent) {
	event.ID = uuid.New()
	event.Timestamp = time.Now()

	// Deduplication: suppress if the same resource+type was seen within dedupTTL.
	dedupKey := event.ResourceID + ":" + event.Type
	h.mu.Lock()
	lastSeen, seen := h.dedup[dedupKey]
	if seen && time.Since(lastSeen) < h.dedupTTL {
		h.mu.Unlock()
		h.logger.Debug("drift event suppressed (duplicate)",
			"layer", event.Layer,
			"type", event.Type,
			"resource_id", event.ResourceID,
		)
		return
	}
	h.dedup[dedupKey] = time.Now()
	h.mu.Unlock()

	// Determine and execute action.
	event.Action = h.executeAction(ctx, event)

	// Log at warn level (drift is always noteworthy).
	h.logger.Warn("drift detected",
		"id", event.ID,
		"layer", event.Layer,
		"type", event.Type,
		"severity", event.Severity,
		"resource", event.Resource,
		"resource_id", event.ResourceID,
		"host_id", event.HostID,
		"expected", event.Expected,
		"actual", event.Actual,
		"detected_by", event.DetectedBy,
		"action", event.Action,
	)

	// Persist to drift_events table.
	if h.pool != nil {
		if err := h.persist(ctx, event); err != nil {
			h.logger.Error("drift: failed to persist event", "id", event.ID, "error", err)
		}
	}
}

func (h *DriftHandler) executeAction(ctx context.Context, event DriftEvent) string {
	if !h.autoHealEnabled {
		return DriftActionAlert
	}
	switch event.Layer {
	case DriftLayerCompute:
		return h.healCompute(ctx, event)
	case DriftLayerNetwork:
		return h.healNetwork(event)
	default:
		return DriftActionAlert
	}
}

func (h *DriftHandler) healCompute(ctx context.Context, event DriftEvent) string {
	if event.Resource != "vm" {
		return DriftActionAlert
	}

	vmID, err := uuid.Parse(event.ResourceID)
	if err != nil {
		return DriftActionAlert
	}

	// Recovery: DB=error but libvirt reports running/shutoff → the VM recovered
	// (e.g. worker restarted and restored state from DB). Heal back to actual.
	if h.vmRecoverer != nil &&
		event.Type == DriftTypeStateMismatch &&
		event.Expected == "error" &&
		(event.Actual == "running" || event.Actual == "shutoff") {
		newStatus := "running"
		if event.Actual == "shutoff" {
			newStatus = "stopped"
		}
		if err := h.vmRecoverer.RecoverVM(ctx, vmID, newStatus); err != nil {
			h.logger.Warn("drift: VM recovery failed", "vm_id", vmID, "error", err)
			return DriftActionAlert
		}
		return DriftActionAutoHeal
	}

	if h.vmHealer == nil {
		return DriftActionAlert
	}
	// Only auto-heal when the VM has definitively disappeared or crashed.
	// DB=running + libvirt=shutoff is alert-only: it may be an intentional
	// external stop, so we never overwrite the DB state automatically.
	shouldHeal := event.Type == DriftTypeExpectedMissing ||
		(event.Type == DriftTypeStateMismatch && event.Actual == "crashed")
	if !shouldHeal {
		return DriftActionAlert
	}
	reason := event.Type
	if event.Actual != "" {
		reason += ": " + event.Actual
	}
	if err := h.vmHealer.HealVM(ctx, vmID, reason); err != nil {
		h.logger.Warn("drift: VM auto-heal failed", "vm_id", vmID, "error", err)
		return DriftActionAlert
	}
	return DriftActionAutoHeal
}

func (h *DriftHandler) healNetwork(event DriftEvent) string {
	if h.networkHealer == nil || event.Type != DriftTypeStateMismatch || event.HostID == "" {
		return DriftActionAlert
	}
	hostID, err := uuid.Parse(event.HostID)
	if err != nil {
		return DriftActionAlert
	}
	h.networkHealer.TriggerRefresh(hostID)
	return DriftActionAutoHeal
}

func (h *DriftHandler) persist(ctx context.Context, event DriftEvent) error {
	toNullableString := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	_, err := h.pool.Exec(ctx,
		`INSERT INTO drift_events
		  (id, layer, type, severity, resource, resource_id, tenant_id, host_id,
		   expected, actual, detected_by, action, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		event.ID, event.Layer, event.Type, event.Severity, event.Resource,
		event.ResourceID,
		toNullableString(event.TenantID),
		toNullableString(event.HostID),
		toNullableString(event.Expected),
		toNullableString(event.Actual),
		event.DetectedBy, event.Action, event.Timestamp,
	)
	return err
}
