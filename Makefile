.PHONY: all build test lint serve stop logs logs-worker logs-sim clean-dev reset-db fresh proto \
       test-unit test-mock test-integration build-sim

# ── Configuration ──

CIRRUS_SIM_ENV   ?= small
AUTH_TOKENS      ?= dev-token=dev-admin
REGISTRATION_TOKEN ?= dev-registration-token

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

build-sim:
	go build -o bin/cirrus-sim ./cmd/cirrus-sim/
	go build -o bin/libvirtd-sim ./cmd/libvirtd-sim/
	go build -o bin/cirrus-sim-ctl ./cmd/cirrus-sim-ctl/

# ── Proto ──

proto:
	protoc -I proto \
	  --go_out=proto/agentpb --go_opt=module=github.com/tjst-t/cirrus/proto/agentpb \
	  --go-grpc_out=proto/agentpb --go-grpc_opt=module=github.com/tjst-t/cirrus/proto/agentpb \
	  proto/agent.proto

# ── Test / Lint ──

test: test-unit

test-unit:
	go test ./...

test-mock:
	go test ./test/mock/...

test-integration:
	@echo "==> Building test images..."
	@cd test/integration && docker compose build
	@echo "==> Starting integration environment..."
	@cd test/integration && docker compose up -d
	@echo "==> Waiting for services..."
	@sleep 10
	@echo "==> Running integration checks..."
	@cd test/integration && docker compose ps
	@echo "==> Integration environment is up. Run 'cd test/integration && docker compose down' to stop."

lint:
	golangci-lint run ./...

# ── Serve (all-in-one: sim + controller + workers) ──

serve: build build-sim
	@mkdir -p $(TMP_DIR) $(PID_WORKER_DIR) $(LOG_WORKER_DIR)
	@# ── 1. Stop existing processes ──
	@$(MAKE) --no-print-directory _stop-all
	@# ── 2. Allocate all ports ──
	@$(MAKE) --no-print-directory _alloc-ports
	@# ── 3. Start cirrus-sim (includes embedded PostgreSQL) ──
	@$(MAKE) --no-print-directory _start-sim
	@# ── 4. Start controller ──
	@$(MAKE) --no-print-directory _start-controller
	@# ── 5. Seed topology (domains + locations for dev) ──
	@$(MAKE) --no-print-directory _seed-topology
	@# ── 6. Start workers (self-register with topology declaration) ──
	@$(MAKE) --no-print-directory _start-workers
	@# ── 7. Activate all registered hosts (dev convenience) ──
	@$(MAKE) --no-print-directory _activate-hosts
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
	@# Stop cirrus-sim (includes embedded PostgreSQL)
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
	  --name sim-aggregator:expose \
	  --name sim-libvirt \
	  --name sim-awx \
	  --name sim-storage \
	  --name sim-postgres \
	  --name sim-postgres-mgmt \
	  --range sim-libvirt-hosts=20 \
	  --name api:expose \
	  --name grpc \
	  --output $(PORTMAN_ENV)

# ── Internal: start cirrus-sim ──

_start-sim:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Starting cirrus-sim (env: $(CIRRUS_SIM_ENV), log: $(LOG_SIM))"; \
	  nohup ./bin/cirrus-sim \
	    -common=$$SIM_COMMON_PORT \
	    -aggregator=$$SIM_AGGREGATOR_PORT \
	    -libvirt=$$SIM_LIBVIRT_PORT \
	    -awx=$$SIM_AWX_PORT \
	    -storage=$$SIM_STORAGE_PORT \
	    -postgres=$$SIM_POSTGRES_PORT \
	    -postgres-mgmt=$$SIM_POSTGRES_MGMT_PORT \
	    -env=test/sim/environments/$(CIRRUS_SIM_ENV).yaml \
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

# ── Internal: start controller ──

_start-controller:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  DB_DSN="postgresql://cirrus:cirrus@localhost:$$SIM_POSTGRES_PORT/cirrus?sslmode=disable"; \
	  echo "==> Starting controller (API: $$API_PORT, gRPC: $$GRPC_PORT, DB: $$DB_DSN, log: $(LOG_CONTROLLER))"; \
	  nohup ./bin/cirrus controller \
	    --api-port=$$API_PORT \
	    --grpc-port=$$GRPC_PORT \
	    --db-dsn="$$DB_DSN" \
	    --storage-endpoint="http://localhost:$$SIM_STORAGE_PORT" \
	    --awx-endpoint="http://localhost:$$SIM_AWX_PORT" \
	    --auth-tokens="$(AUTH_TOKENS)" \
	    --registration-token="$(REGISTRATION_TOKEN)" \
	    --log-level=debug \
	    > $(LOG_CONTROLLER) 2>&1 & \
	  echo $$! > $(PID_CONTROLLER); \
	  echo "    PID: $$(cat $(PID_CONTROLLER))"'

# ── Internal: seed topology (dev convenience) ──

_seed-topology:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Waiting for controller API..."; \
	  for i in $$(seq 1 30); do \
	    curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 && break; \
	    sleep 0.5; \
	  done; \
	  curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 \
	    || { echo "ERROR: Controller API not ready"; exit 1; }; \
	  TOKEN="$(firstword $(subst =, ,$(AUTH_TOKENS)))"; \
	  echo "==> Seeding topology (storage-domain, network-domain, location, AZ)..."; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default-sd\"}" \
	    http://localhost:$$API_PORT/api/v1/storage-domains >/dev/null 2>&1 || true; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default-nd\"}" \
	    http://localhost:$$API_PORT/api/v1/network-domains >/dev/null 2>&1 || true; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default-site\",\"type\":\"site\"}" \
	    http://localhost:$$API_PORT/api/v1/locations >/dev/null 2>&1 || true; \
	  LOC_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/locations | jq -r '.[0].id'); \
	  ND_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/network-domains | jq -r '.[0].id'); \
	  SD_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/storage-domains | jq -r '.[0].id'); \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default-az\",\"location_id\":\"$$LOC_ID\",\"network_domain_id\":\"$$ND_ID\"}" \
	    http://localhost:$$API_PORT/api/v1/admin/availability-zones >/dev/null 2>&1 || true; \
	  AZ_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/admin/availability-zones | jq -r '.[0].id'); \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"storage_domain_id\":\"$$SD_ID\"}" \
	    http://localhost:$$API_PORT/api/v1/admin/availability-zones/$$AZ_ID/storage-domains >/dev/null 2>&1 || true; \
	  echo "    Topology seeded."'

# ── Internal: activate all registered hosts (dev convenience) ──

_activate-hosts:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Waiting for workers to register..."; \
	  sleep 3; \
	  echo "==> Activating all registered hosts (dev convenience)..."; \
	  TOKEN="$(firstword $(subst =, ,$(AUTH_TOKENS)))"; \
	  HOSTS=$$(curl -sf -H "Authorization: Bearer $$TOKEN" \
	    "http://localhost:$$API_PORT/api/v1/hosts?state=registering"); \
	  HOST_COUNT=$$(echo "$$HOSTS" | jq length); \
	  for i in $$(seq 0 $$((HOST_COUNT - 1))); do \
	    HOST_UUID=$$(echo "$$HOSTS" | jq -r ".[$${i}].id"); \
	    curl -sf -X POST \
	      -H "Authorization: Bearer $$TOKEN" \
	      -H "Content-Type: application/json" \
	      -d "{\"action\":\"activate\"}" \
	      http://localhost:$$API_PORT/api/v1/hosts/$$HOST_UUID/actions >/dev/null 2>&1 || true; \
	  done; \
	  echo "    Activated $$HOST_COUNT hosts"'

# ── Internal: start workers (one per simulated host) ──

_start-workers:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Waiting for controller API..."; \
	  for i in $$(seq 1 30); do \
	    curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 && break; \
	    sleep 0.5; \
	  done; \
	  curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 \
	    || { echo "ERROR: Controller API not ready"; exit 1; }; \
	  echo "==> Fetching host list from cirrus-sim..."; \
	  HOSTS=$$(curl -sf http://localhost:$$SIM_LIBVIRT_PORT/sim/hosts); \
	  if [ -z "$$HOSTS" ] || [ "$$HOSTS" = "null" ]; then \
	    echo "ERROR: Failed to get host list from cirrus-sim"; exit 1; \
	  fi; \
	  HOST_COUNT=$$(echo "$$HOSTS" | jq length); \
	  echo "    Found $$HOST_COUNT hosts"; \
	  echo "==> Starting workers (self-registration)..."; \
	  for i in $$(seq 0 $$((HOST_COUNT - 1))); do \
	    HOST_ID=$$(echo "$$HOSTS" | jq -r ".[$${i}].host_id"); \
	    LIBVIRT_PORT=$$(echo "$$HOSTS" | jq -r ".[$${i}].libvirt_port"); \
	    HOSTNAME_OVERRIDE=$$HOST_ID nohup ./bin/cirrus worker \
	      --controller="localhost:$$GRPC_PORT" \
	      --registration-token="$(REGISTRATION_TOKEN)" \
	      --libvirt-uri="tcp://localhost:$$LIBVIRT_PORT" \
	      --network-domain="default-nd" \
	      --storage-domains="default-sd" \
	      --location="default-site" \
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

# ── Database ──

reset-db:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  DB_DSN="postgresql://cirrus:cirrus@localhost:$$SIM_POSTGRES_PORT/cirrus?sslmode=disable"; \
	  echo "==> Resetting database (DROP + CREATE schema)..."; \
	  go run ./cmd/internal/resetdb "$$DB_DSN"; \
	  echo "    Database reset OK."'

fresh: stop reset-db serve
