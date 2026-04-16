#!/bin/bash
set -e

echo "=== Worker starting: HOST_ID=${HOST_ID} ==="

# Start OVS
mkdir -p /var/run/openvswitch /var/log/openvswitch /etc/openvswitch
# Recreate the DB on each start to get a clean OVS state
rm -f /etc/openvswitch/conf.db
ovsdb-tool create /etc/openvswitch/conf.db /usr/share/openvswitch/vswitch.ovsschema
# Kill any stale OVS processes from a previous run
kill $(cat /var/run/openvswitch/ovsdb-server.pid 2>/dev/null) 2>/dev/null || true
kill $(cat /var/run/openvswitch/ovs-vswitchd.pid 2>/dev/null) 2>/dev/null || true
sleep 0.5
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
  ${DB_DSN:+--db-dsn="${DB_DSN}"} \
  &

echo "libvirtd-sim started for host ${HOST_ID}"

# Wait for libvirtd-sim to be ready (DB load may take a moment)
for i in $(seq 1 60); do
  curl -sf http://localhost:${LIBVIRTD_SIM_MGMT_PORT:-8100}/healthz >/dev/null 2>&1 && break
  sleep 0.5
done
echo "libvirtd-sim is ready"

# Wait for controller to be ready
echo "Waiting for controller at ${CONTROLLER_ADDR}..."
for i in $(seq 1 60); do
  # Try gRPC health or just TCP connect
  if bash -c "echo >/dev/tcp/${CONTROLLER_ADDR%%:*}/${CONTROLLER_ADDR##*:}" 2>/dev/null; then
    echo "Controller is reachable"
    break
  fi
  sleep 1
done

# Detect container IP for Geneve tunnel fabric
FABRIC_IP=$(hostname -i | awk '{print $1}')
echo "Detected fabric IP: ${FABRIC_IP}"

# Start cirrus worker (self-registration)
WORKER_GRPC_PORT="${WORKER_GRPC_PORT:-9191}"
cirrus worker \
  --controller="${CONTROLLER_ADDR}" \
  --registration-token="${REGISTRATION_TOKEN:-dev-registration-token}" \
  --libvirt-uri="tcp://localhost:${LIBVIRT_PORT:-16509}" \
  --libvirt-sim-mgmt-addr="http://localhost:${LIBVIRTD_SIM_MGMT_PORT:-8100}" \
  --fabric-ip="${FABRIC_IP}" \
  --storage-domains="default-sd" \
  --location="default-site" \
  --grpc-port="${WORKER_GRPC_PORT}" \
  --worker-grpc-addr="${FABRIC_IP}:${WORKER_GRPC_PORT}" \
  &

echo "cirrus worker started"

# Wait for any process to exit
wait -n

echo "=== Worker exiting ==="
exit 1
