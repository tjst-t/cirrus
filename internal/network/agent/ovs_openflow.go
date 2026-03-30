package agent

// TODO(Sprint 5N-b): Real OVS OpenFlow client using antrea-io/libOpenflow + antrea-io/ofnet.
// This will be implemented when docker-compose integration tests are set up.
// For now, all tests use the mock OVS client.
//
// The real implementation will:
// - Connect to OVS via OpenFlow 1.3 using ofctrl
// - Use OpenFlow Bundle messages for atomic flow updates
// - Manage ports via ovs-vsctl exec (port management is outside OpenFlow scope)
// - Query ofport numbers via ovs-vsctl get Interface <name> ofport
