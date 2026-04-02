-- Sprint 6: Storage基盤 — storage_backends, volume_types, volumes + hosts.storage_properties

-- ホストのストレージ接続属性（iSCSIイニシエータIQN、Cephキーリング等）
-- プロトコル固有カラムを避け、JSOBで拡張可能にする
-- 例: {"iscsi_iqn": "iqn.2024.com.example:host1", "ceph_client": "client.host1"}
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS storage_properties JSONB NOT NULL DEFAULT '{}';

-- ストレージバックエンド
-- 1バックエンド = ストレージ装置上の1論理プール
CREATE TABLE IF NOT EXISTS storage_backends (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_domain_id UUID NOT NULL REFERENCES storage_domains(id),
    name              VARCHAR(63) UNIQUE NOT NULL,
    driver            VARCHAR(63) NOT NULL,   -- "sim", "iscsi", "rbd", ...
    endpoint          TEXT NOT NULL,           -- バックエンド管理APIのURL
    total_capacity_gb BIGINT NOT NULL,
    total_iops        BIGINT NOT NULL DEFAULT 0,
    capabilities      JSONB NOT NULL DEFAULT '[]',  -- ["ssd", "encryption", ...]
    driver_config     JSONB NOT NULL DEFAULT '{}',  -- ドライバ固有設定（認証情報等）
    state             VARCHAR(32) NOT NULL DEFAULT 'active', -- active, draining, retired
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_storage_backends_domain ON storage_backends(storage_domain_id);
CREATE INDEX IF NOT EXISTS idx_storage_backends_state ON storage_backends(state);

-- ボリュームタイプ
-- バックエンド特性のユーザ向け抽象化（Flavorのストレージ版）
CREATE TABLE IF NOT EXISTS volume_types (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  VARCHAR(63) UNIQUE NOT NULL,
    description           TEXT,
    required_capabilities JSONB NOT NULL DEFAULT '[]',  -- バックエンド選定条件
    qos_policy            JSONB,  -- {"iops_limit": 1000, "bw_limit_mb": 100}
    is_public             BOOLEAN NOT NULL DEFAULT true,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ボリューム
CREATE TABLE IF NOT EXISTS volumes (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id),
    name             VARCHAR(63) NOT NULL,
    volume_type_id   UUID REFERENCES volume_types(id),
    backend_id       UUID REFERENCES storage_backends(id),
    size_gb          BIGINT NOT NULL,
    state            VARCHAR(32) NOT NULL DEFAULT 'creating',
    -- エクスポート状態（Driver.ExportVolumeが返したExportInfoを保持）
    exported_host_id UUID REFERENCES hosts(id),
    export_info      JSONB,
    az_id            UUID REFERENCES availability_zones(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_volumes_tenant_name ON volumes(tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_volumes_backend ON volumes(backend_id);
CREATE INDEX IF NOT EXISTS idx_volumes_tenant ON volumes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_volumes_state ON volumes(state);
