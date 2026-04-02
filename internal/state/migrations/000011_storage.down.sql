-- Sprint 6: Storage基盤 rollback

DROP TABLE IF EXISTS volumes;
DROP TABLE IF EXISTS volume_types;
DROP TABLE IF EXISTS storage_backends;
ALTER TABLE hosts DROP COLUMN IF EXISTS storage_properties;
