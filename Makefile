.PHONY: all build test lint serve stop logs logs-worker logs-sim clean-dev proto

# ── Configuration ──

CIRRUS_SIM_DIR   ?= $(shell cd .. && pwd)/cirrus-sim
CIRRUS_SIM_ENV   ?= small
DB_DSN           ?= postgres://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable
AUTH_TOKENS      ?= dev-token=dev-admin

# Temp files
TMP_DIR          := /tmp/cirrus-dev
PID_CONTROLLER   := $(TMP_DIR)/controller.pid
PID_SIM          := $(TMP_DIR)/sim.pid
PID_WORKER_DIR   := $(TMP_DIR)/workers
LOG_CONTROLLER   := $(TMP_DIR)/controller.log
LOG_SIM          := $(TMP_DIR)/sim.log
LOG_WORKER_DIR   := $(TMP_DIR)/worker-logs
PORTMAN_ENV      := $(TMP_DIR)/portman.env

# ── Build ──

all: lint test build

build:
	go build -o bin/cirrus ./cmd/cirrus/
	go build -o bin/cirrusctl ./cmd/cirrusctl/

# ── Proto ──

proto:
	protoc -I proto \
	  --go_out=proto/agentpb --go_opt=module=github.com/tjst-t/cirrus/proto/agentpb \
	  --go-grpc_out=proto/agentpb --go-grpc_opt=module=github.com/tjst-t/cirrus/proto/agentpb \
	  proto/agent.proto

# ── Test / Lint ──

test:
	go test ./...

lint:
	golangci-lint run ./...

# ── Serve (all-in-one: sim + controller + workers) ──

serve: build
	@mkdir -p $(TMP_DIR) $(PID_WORKER_DIR) $(LOG_WORKER_DIR)
	@# ── 1. Stop existing processes ──
	@$(MAKE) --no-print-directory _stop-all
	@# ── 2. Allocate all ports ──
	@$(MAKE) --no-print-directory _alloc-ports
	@# ── 3. Start cirrus-sim ──
	@$(MAKE) --no-print-directory _start-sim
	@# ── 4. Start PostgreSQL ──
	@$(MAKE) --no-print-directory _start-db
	@# ── 5. Start controller ──
	@$(MAKE) --no-print-directory _start-controller
	@# ── 6. Register hosts ──
	@$(MAKE) --no-print-directory _register-hosts
	@# ── 7. Start workers ──
	@$(MAKE) --no-print-directory _start-workers
	@echo ""
	@echo "  All services running. Use 'make logs' to view controller logs."
	@echo "  Stop: make stop"

# ── Stop all ──

stop:
	@$(MAKE) --no-print-directory _stop-all

_stop-all:
	@# Stop workers
	@if [ -d $(PID_WORKER_DIR) ]; then \
	  for pidfile in $(PID_WORKER_DIR)/*.pid; do \
	    [ -f "$$pidfile" ] || continue; \
	    PID=$$(cat "$$pidfile"); \
	    if kill -0 $$PID 2>/dev/null; then \
	      echo "==> Stopping worker (PID: $$PID)..."; \
	      kill $$PID 2>/dev/null; \
	      for i in $$(seq 1 30); do kill -0 $$PID 2>/dev/null || break; sleep 0.1; done; \
	      kill -0 $$PID 2>/dev/null && kill -9 $$PID 2>/dev/null || true; \
	    fi; \
	    rm -f "$$pidfile"; \
	  done; \
	fi
	@# Stop controller
	@if [ -f $(PID_CONTROLLER) ]; then \
	  PID=$$(cat $(PID_CONTROLLER)); \
	  if kill -0 $$PID 2>/dev/null; then \
	    echo "==> Stopping controller (PID: $$PID)..."; \
	    kill $$PID; \
	    for i in $$(seq 1 50); do kill -0 $$PID 2>/dev/null || break; sleep 0.1; done; \
	    kill -0 $$PID 2>/dev/null && kill -9 $$PID 2>/dev/null || true; \
	  fi; \
	  rm -f $(PID_CONTROLLER); \
	fi
	@# Stop cirrus-sim
	@if [ -f $(PID_SIM) ]; then \
	  PID=$$(cat $(PID_SIM)); \
	  if kill -0 $$PID 2>/dev/null; then \
	    echo "==> Stopping cirrus-sim (PID: $$PID)..."; \
	    kill $$PID; \
	    for i in $$(seq 1 50); do kill -0 $$PID 2>/dev/null || break; sleep 0.1; done; \
	    kill -0 $$PID 2>/dev/null && kill -9 $$PID 2>/dev/null || true; \
	  fi; \
	  rm -f $(PID_SIM); \
	fi

# ── Internal: allocate all ports ──

_alloc-ports:
	@echo "==> Allocating ports..."
	@portman env \
	  --name sim-common \
	  --name sim-dashboard:expose \
	  --name sim-libvirt \
	  --name sim-ovn \
	  --name sim-awx \
	  --name sim-netbox \
	  --name sim-storage \
	  --range sim-libvirt-hosts=20 \
	  --range sim-ovn-clusters=5 \
	  --name api:expose \
	  --name grpc \
	  --output $(PORTMAN_ENV)

# ── Internal: start cirrus-sim ──

_start-sim:
	@if [ ! -x $(CIRRUS_SIM_DIR)/bin/cirrus-sim ]; then \
	  echo "ERROR: cirrus-sim binary not found at $(CIRRUS_SIM_DIR)/bin/cirrus-sim"; \
	  echo "       Run 'make build-unified' in $(CIRRUS_SIM_DIR) first."; \
	  exit 1; \
	fi
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Starting cirrus-sim (env: $(CIRRUS_SIM_ENV), log: $(LOG_SIM))"; \
	  nohup $(CIRRUS_SIM_DIR)/bin/cirrus-sim \
	    -common=$$SIM_COMMON_PORT \
	    -dashboard=$$SIM_DASHBOARD_PORT \
	    -libvirt=$$SIM_LIBVIRT_PORT \
	    -ovn=$$SIM_OVN_PORT \
	    -awx=$$SIM_AWX_PORT \
	    -netbox=$$SIM_NETBOX_PORT \
	    -storage=$$SIM_STORAGE_PORT \
	    -env=$(CIRRUS_SIM_DIR)/environments/$(CIRRUS_SIM_ENV).yaml \
	    > $(LOG_SIM) 2>&1 & \
	  echo $$! > $(PID_SIM); \
	  echo "    PID: $$(cat $(PID_SIM))"'
	@echo "==> Waiting for cirrus-sim to be ready..."
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  for i in $$(seq 1 30); do \
	    curl -sf http://localhost:$$SIM_COMMON_PORT/api/v1/events >/dev/null 2>&1 && break; \
	    sleep 0.5; \
	  done; \
	  curl -sf http://localhost:$$SIM_COMMON_PORT/api/v1/events >/dev/null 2>&1 \
	    && echo "    cirrus-sim is ready." \
	    || { echo "ERROR: cirrus-sim failed to start. Check $(LOG_SIM)"; exit 1; }'

# ── Internal: check PostgreSQL ──

_start-db:
	@echo "==> PostgreSQL: $(DB_DSN)"

# ── Internal: start controller ──

_start-controller:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Starting controller (API: $$API_PORT, gRPC: $$GRPC_PORT, log: $(LOG_CONTROLLER))"; \
	  nohup ./bin/cirrus controller \
	    --api-port=$$API_PORT \
	    --grpc-port=$$GRPC_PORT \
	    --db-dsn="$(DB_DSN)" \
	    --ovn-nb="tcp:localhost:$$SIM_OVN_PORT" \
	    --storage-endpoint="http://localhost:$$SIM_STORAGE_PORT" \
	    --awx-endpoint="http://localhost:$$SIM_AWX_PORT" \
	    --netbox-endpoint="http://localhost:$$SIM_NETBOX_PORT" \
	    --auth-tokens="$(AUTH_TOKENS)" \
	    > $(LOG_CONTROLLER) 2>&1 & \
	  echo $$! > $(PID_CONTROLLER); \
	  echo "    PID: $$(cat $(PID_CONTROLLER))"'

# ── Internal: register hosts from cirrus-sim into controller ──

_register-hosts:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Waiting for controller API..."; \
	  for i in $$(seq 1 30); do \
	    curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 && break; \
	    sleep 0.5; \
	  done; \
	  curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 \
	    || { echo "ERROR: Controller API not ready"; exit 1; }; \
	  echo "==> Registering hosts from cirrus-sim..."; \
	  HOSTS=$$(curl -sf http://localhost:$$SIM_LIBVIRT_PORT/sim/hosts); \
	  HOST_COUNT=$$(echo "$$HOSTS" | jq length); \
	  TOKEN="$(firstword $(subst =, ,$(AUTH_TOKENS)))"; \
	  for i in $$(seq 0 $$((HOST_COUNT - 1))); do \
	    HOST_ID=$$(echo "$$HOSTS" | jq -r ".[$${i}].host_id"); \
	    LIBVIRT_PORT=$$(echo "$$HOSTS" | jq -r ".[$${i}].libvirt_port"); \
	    curl -sf -X POST \
	      -H "Authorization: Bearer $$TOKEN" \
	      -H "Content-Type: application/json" \
	      -d "{\"name\":\"$$HOST_ID\",\"address\":\"localhost:$$LIBVIRT_PORT\"}" \
	      http://localhost:$$API_PORT/api/v1/hosts >/dev/null 2>&1 || true; \
	  done; \
	  echo "    Registered $$HOST_COUNT hosts"'

# ── Internal: start workers (one per simulated host) ──

_start-workers:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Fetching host list from cirrus-sim..."; \
	  HOSTS=$$(curl -sf http://localhost:$$SIM_LIBVIRT_PORT/sim/hosts); \
	  if [ -z "$$HOSTS" ] || [ "$$HOSTS" = "null" ]; then \
	    echo "ERROR: Failed to get host list from cirrus-sim"; exit 1; \
	  fi; \
	  HOST_COUNT=$$(echo "$$HOSTS" | jq length); \
	  echo "    Found $$HOST_COUNT hosts"; \
	  echo "==> Starting workers..."; \
	  for i in $$(seq 0 $$((HOST_COUNT - 1))); do \
	    HOST_ID=$$(echo "$$HOSTS" | jq -r ".[$${i}].host_id"); \
	    LIBVIRT_PORT=$$(echo "$$HOSTS" | jq -r ".[$${i}].libvirt_port"); \
	    nohup ./bin/cirrus worker \
	      --controller="localhost:$$GRPC_PORT" \
	      --host-id="$$HOST_ID" \
	      --libvirt-uri="tcp://localhost:$$LIBVIRT_PORT" \
	      > $(LOG_WORKER_DIR)/$$HOST_ID.log 2>&1 & \
	    echo $$! > $(PID_WORKER_DIR)/$$HOST_ID.pid; \
	  done; \
	  echo "    Started $$HOST_COUNT workers"'

# ── Logs ──

logs:
	@if [ -f $(LOG_CONTROLLER) ]; then tail -f $(LOG_CONTROLLER); \
	else echo "No controller log found."; fi

logs-worker:
	@if [ -d $(LOG_WORKER_DIR) ]; then tail -f $(LOG_WORKER_DIR)/*.log; \
	else echo "No worker logs found."; fi

logs-sim:
	@if [ -f $(LOG_SIM) ]; then tail -f $(LOG_SIM); \
	else echo "No cirrus-sim log found."; fi

# ── Clean ──

clean-dev:
	@$(MAKE) --no-print-directory _stop-all
	rm -rf $(TMP_DIR)
	@echo "Cleaned dev state."
