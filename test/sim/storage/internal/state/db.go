package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS storage_sim_backends (
	backend_id          TEXT PRIMARY KEY,
	total_capacity_gb   BIGINT  NOT NULL,
	total_iops          BIGINT  NOT NULL DEFAULT 0,
	capabilities        JSONB   NOT NULL DEFAULT '[]',
	overprovision_ratio FLOAT   NOT NULL DEFAULT 1.0,
	state               TEXT    NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS storage_sim_volumes (
	volume_id           TEXT      PRIMARY KEY,
	backend_id          TEXT      NOT NULL,
	size_gb             BIGINT    NOT NULL,
	consumed_gb         BIGINT    NOT NULL DEFAULT 0,
	thin_provisioned    BOOLEAN   NOT NULL DEFAULT false,
	state               TEXT      NOT NULL DEFAULT 'available',
	qos_policy          JSONB,
	metadata            JSONB,
	export_host_id      TEXT,
	export_protocol     TEXT,
	parent_snapshot_id  TEXT      NOT NULL DEFAULT '',
	snapshots           JSONB     NOT NULL DEFAULT '[]',
	created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS storage_sim_snapshots (
	snapshot_id   TEXT      PRIMARY KEY,
	volume_id     TEXT      NOT NULL,
	size_gb       BIGINT    NOT NULL,
	consumed_gb   BIGINT    NOT NULL DEFAULT 0,
	state         TEXT      NOT NULL DEFAULT 'available',
	child_clones  JSONB     NOT NULL DEFAULT '[]',
	metadata      JSONB,
	created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// SetupSchema creates the storage sim tables if they don't exist.
func SetupSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("storage sim: setup schema: %w", err)
	}
	return nil
}

// SetDB attaches a postgres pool for persistence. Must be called before LoadFromDB.
func (s *Store) SetDB(pool *pgxpool.Pool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = pool
}

// LoadFromDB loads backends, volumes, and snapshots from the database into the
// in-memory maps. Backend capacity counters are recomputed from volumes.
func (s *Store) LoadFromDB(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}

	// ── Load backends ───────────────────────────────────────────────────────
	rows, err := s.db.Query(ctx,
		`SELECT backend_id, total_capacity_gb, total_iops, capabilities, overprovision_ratio, state
		 FROM storage_sim_backends`)
	if err != nil {
		return fmt.Errorf("load backends: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var b Backend
		var caps []byte
		if err := rows.Scan(&b.BackendID, &b.TotalCapacityGB, &b.TotalIOPS, &caps, &b.OverprovisionRatio, &b.State); err != nil {
			return fmt.Errorf("scan backend: %w", err)
		}
		if err := json.Unmarshal(caps, &b.Capabilities); err != nil {
			b.Capabilities = []string{}
		}
		stored := b
		s.backends[b.BackendID] = &stored
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load backends rows: %w", err)
	}
	rows.Close()

	// ── Load volumes ────────────────────────────────────────────────────────
	rows, err = s.db.Query(ctx,
		`SELECT volume_id, backend_id, size_gb, consumed_gb, thin_provisioned, state,
		        qos_policy, metadata, export_host_id, export_protocol,
		        parent_snapshot_id, snapshots, created_at
		 FROM storage_sim_volumes`)
	if err != nil {
		return fmt.Errorf("load volumes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var v Volume
		var qosJSON, metaJSON, snapsJSON []byte
		var exportHostID, exportProtocol *string
		if err := rows.Scan(
			&v.VolumeID, &v.BackendID, &v.SizeGB, &v.ConsumedGB, &v.ThinProvisioned, &v.State,
			&qosJSON, &metaJSON, &exportHostID, &exportProtocol,
			&v.ParentSnapshotID, &snapsJSON, &v.CreatedAt,
		); err != nil {
			return fmt.Errorf("scan volume: %w", err)
		}

		if qosJSON != nil {
			var qos QoSPolicy
			if err := json.Unmarshal(qosJSON, &qos); err == nil {
				v.QoSPolicy = &qos
			}
		}
		if metaJSON != nil {
			json.Unmarshal(metaJSON, &v.Metadata) //nolint:errcheck
		}
		if exportHostID != nil && exportProtocol != nil {
			v.ExportInfo = &ExportInfo{HostID: *exportHostID, Protocol: *exportProtocol}
		}
		if snapsJSON != nil {
			json.Unmarshal(snapsJSON, &v.Snapshots) //nolint:errcheck
		}
		if v.Snapshots == nil {
			v.Snapshots = []string{}
		}

		stored := v
		s.volumes[v.VolumeID] = &stored

		// Recompute backend capacity counters
		if b, ok := s.backends[v.BackendID]; ok {
			b.AllocatedCapacityGB += v.SizeGB
			if !v.ThinProvisioned {
				b.UsedCapacityGB += v.SizeGB
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load volumes rows: %w", err)
	}
	rows.Close()

	// ── Load snapshots ──────────────────────────────────────────────────────
	rows, err = s.db.Query(ctx,
		`SELECT snapshot_id, volume_id, size_gb, consumed_gb, state, child_clones, metadata, created_at
		 FROM storage_sim_snapshots`)
	if err != nil {
		return fmt.Errorf("load snapshots: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var snap Snapshot
		var clonesJSON, metaJSON []byte
		if err := rows.Scan(
			&snap.SnapshotID, &snap.VolumeID, &snap.SizeGB, &snap.ConsumedGB,
			&snap.State, &clonesJSON, &metaJSON, &snap.CreatedAt,
		); err != nil {
			return fmt.Errorf("scan snapshot: %w", err)
		}
		if clonesJSON != nil {
			json.Unmarshal(clonesJSON, &snap.ChildClones) //nolint:errcheck
		}
		if snap.ChildClones == nil {
			snap.ChildClones = []string{}
		}
		if metaJSON != nil {
			json.Unmarshal(metaJSON, &snap.Metadata) //nolint:errcheck
		}
		stored := snap
		s.snapshots[snap.SnapshotID] = &stored
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load snapshots rows: %w", err)
	}

	s.logger.Info("state loaded from DB",
		"backends", len(s.backends),
		"volumes", len(s.volumes),
		"snapshots", len(s.snapshots),
	)
	return nil
}

// dbSaveBackend upserts a backend (call with lock already held or after releasing it).
func (s *Store) dbSaveBackend(b *Backend) {
	if s.db == nil {
		return
	}
	caps, _ := json.Marshal(b.Capabilities)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `
		INSERT INTO storage_sim_backends (backend_id, total_capacity_gb, total_iops, capabilities, overprovision_ratio, state)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (backend_id) DO UPDATE SET
			total_capacity_gb   = EXCLUDED.total_capacity_gb,
			total_iops          = EXCLUDED.total_iops,
			capabilities        = EXCLUDED.capabilities,
			overprovision_ratio = EXCLUDED.overprovision_ratio,
			state               = EXCLUDED.state`,
		b.BackendID, b.TotalCapacityGB, b.TotalIOPS, caps, b.OverprovisionRatio, b.State,
	)
	if err != nil {
		s.logger.Error("persist backend", "backend_id", b.BackendID, "error", err)
	}
}

// dbSaveVolume upserts a volume.
func (s *Store) dbSaveVolume(v *Volume) {
	if s.db == nil {
		return
	}
	var qosJSON, metaJSON []byte
	qosJSON, _ = json.Marshal(v.QoSPolicy)
	metaJSON, _ = json.Marshal(v.Metadata)
	snapsJSON, _ := json.Marshal(v.Snapshots)

	var exportHostID, exportProtocol *string
	if v.ExportInfo != nil {
		exportHostID = &v.ExportInfo.HostID
		exportProtocol = &v.ExportInfo.Protocol
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `
		INSERT INTO storage_sim_volumes
			(volume_id, backend_id, size_gb, consumed_gb, thin_provisioned, state,
			 qos_policy, metadata, export_host_id, export_protocol,
			 parent_snapshot_id, snapshots, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (volume_id) DO UPDATE SET
			backend_id         = EXCLUDED.backend_id,
			size_gb            = EXCLUDED.size_gb,
			consumed_gb        = EXCLUDED.consumed_gb,
			thin_provisioned   = EXCLUDED.thin_provisioned,
			state              = EXCLUDED.state,
			qos_policy         = EXCLUDED.qos_policy,
			metadata           = EXCLUDED.metadata,
			export_host_id     = EXCLUDED.export_host_id,
			export_protocol    = EXCLUDED.export_protocol,
			parent_snapshot_id = EXCLUDED.parent_snapshot_id,
			snapshots          = EXCLUDED.snapshots,
			created_at         = EXCLUDED.created_at`,
		v.VolumeID, v.BackendID, v.SizeGB, v.ConsumedGB, v.ThinProvisioned, v.State,
		qosJSON, metaJSON, exportHostID, exportProtocol,
		v.ParentSnapshotID, snapsJSON, v.CreatedAt,
	)
	if err != nil {
		s.logger.Error("persist volume", "volume_id", v.VolumeID, "error", err)
	}
}

// dbDeleteVolume removes a volume from the DB.
func (s *Store) dbDeleteVolume(id string) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `DELETE FROM storage_sim_volumes WHERE volume_id = $1`, id); err != nil {
		s.logger.Error("delete volume from DB", "volume_id", id, "error", err)
	}
}

// dbSaveSnapshot upserts a snapshot.
func (s *Store) dbSaveSnapshot(snap *Snapshot) {
	if s.db == nil {
		return
	}
	clonesJSON, _ := json.Marshal(snap.ChildClones)
	metaJSON, _ := json.Marshal(snap.Metadata)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `
		INSERT INTO storage_sim_snapshots
			(snapshot_id, volume_id, size_gb, consumed_gb, state, child_clones, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (snapshot_id) DO UPDATE SET
			volume_id    = EXCLUDED.volume_id,
			size_gb      = EXCLUDED.size_gb,
			consumed_gb  = EXCLUDED.consumed_gb,
			state        = EXCLUDED.state,
			child_clones = EXCLUDED.child_clones,
			metadata     = EXCLUDED.metadata,
			created_at   = EXCLUDED.created_at`,
		snap.SnapshotID, snap.VolumeID, snap.SizeGB, snap.ConsumedGB,
		snap.State, clonesJSON, metaJSON, snap.CreatedAt,
	)
	if err != nil {
		s.logger.Error("persist snapshot", "snapshot_id", snap.SnapshotID, "error", err)
	}
}

// dbDeleteSnapshot removes a snapshot from the DB.
func (s *Store) dbDeleteSnapshot(id string) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `DELETE FROM storage_sim_snapshots WHERE snapshot_id = $1`, id); err != nil {
		s.logger.Error("delete snapshot from DB", "snapshot_id", id, "error", err)
	}
}

// dbClearAll truncates all storage sim tables.
func (s *Store) dbClearAll() {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx,
		`TRUNCATE storage_sim_backends, storage_sim_volumes, storage_sim_snapshots`); err != nil {
		s.logger.Error("clear storage sim tables", "error", err)
	}
}
