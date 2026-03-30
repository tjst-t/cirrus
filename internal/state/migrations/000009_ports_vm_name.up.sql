-- Sprint 5N-b: Add vm_name to ports for HostNetworkState (DNS record generation, metadata).
-- Until the Compute module is implemented, vm_name is set when ports are created internally.
ALTER TABLE ports ADD COLUMN vm_name TEXT NOT NULL DEFAULT '';
