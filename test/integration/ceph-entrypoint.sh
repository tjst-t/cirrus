#!/bin/bash
# Start the Ceph demo cluster, then launch cirrus-rbd-server once Ceph is ready.
set -e

# Launch the Ceph demo bootstrap in the background.
/entrypoint.sh &
CEPH_PID=$!

echo "==> Waiting for Ceph cluster to be ready..."
for i in $(seq 1 120); do
    if ceph status --connect-timeout 2 >/dev/null 2>&1; then
        echo "==> Ceph is ready."
        break
    fi
    sleep 2
done

# Verify Ceph is actually up.
if ! ceph status --connect-timeout 2 >/dev/null 2>&1; then
    echo "ERROR: Ceph failed to start. Exiting."
    exit 1
fi

# Start cirrus-rbd-server in the foreground alongside Ceph.
exec /usr/local/bin/cirrus-rbd-server &
RBD_PID=$!

# Wait for both processes. If either exits, the container will stop.
wait $CEPH_PID $RBD_PID
