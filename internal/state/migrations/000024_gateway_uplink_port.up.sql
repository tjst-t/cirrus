-- Add uplink_port to gateway_nodes for Direct Connect VLAN trunk support.
ALTER TABLE gateway_nodes ADD COLUMN IF NOT EXISTS uplink_port TEXT NOT NULL DEFAULT '';
