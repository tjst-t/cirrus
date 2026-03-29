#!/bin/bash
set -e

echo "=== Worker starting: HOST_ID=${HOST_ID} ==="

# Start OVS
mkdir -p /var/run/openvswitch
ovsdb-server --remote=punix:/var/run/openvswitch/db.sock \
  --remote=db:Open_vSwitch,Open_vSwitch,manager_options \
  --pidfile --detach --log-file=/var/log/openvswitch/ovsdb-server.log

ovs-vswitchd --pidfile --detach --log-file=/var/log/openvswitch/ovs-vswitchd.log

# Create integration bridge
ovs-vsctl add-br br-int

echo "OVS bridge br-int created"

# Start libvirtd-sim (single-host mode)
libvirtd-sim \
  --host-id="${HOST_ID}" \
  --libvirt-port="${LIBVIRT_PORT:-16509}" \
  --mgmt-port="${LIBVIRTD_SIM_MGMT_PORT:-8100}" \
  --cpu-model="${CPU_MODEL:-Intel Xeon Gold 6348}" \
  --cpu-sockets="${CPU_SOCKETS:-2}" \
  --cores-per-socket="${CORES_PER_SOCKET:-28}" \
  --threads-per-core="${THREADS_PER_CORE:-2}" \
  --memory-mb="${MEMORY_MB:-524288}" \
  --enable-netns \
  --ovs-bridge=br-int \
  &

echo "libvirtd-sim started for host ${HOST_ID}"

# Start cirrus worker
cirrus worker \
  --host-id="${HOST_ID}" \
  --controller-addr="${CONTROLLER_ADDR}" \
  &

echo "cirrus worker started"

# Wait for any process to exit
wait -n

echo "=== Worker exiting ==="
exit 1
