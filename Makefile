.PHONY: all build test lint serve stop serve-storage stop-storage logs logs-worker logs-sim \
        clean-dev reset-db fresh proto test-unit test-mock test-integration test-smoke build-sim \
        build-storage-servers web-install web-build test-e2e

# ── Configuration ──

CIRRUS_SIM_ENV     ?= small
AUTH_TOKENS        ?= dev-token=dev-admin
REGISTRATION_TOKEN ?= dev-registration-token

# Docker (auto-detect sudo need)
DOCKER_SUDO        := $(shell docker info >/dev/null 2>&1 && echo "" || echo "sudo -E")
COMPOSE            := $(DOCKER_SUDO) docker-compose -f docker-compose.dev.yml
COMPOSE_STORAGE    := $(DOCKER_SUDO) docker-compose -f docker-compose.dev.yml -f docker-compose.storage.yml

# Docker host IP (for worker containers to reach host services)
CONTROLLER_HOST    := $(shell sudo docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}' 2>/dev/null || echo "172.17.0.1")

# Temp files
TMP_DIR            := /tmp/cirrus-dev
PID_SIM            := $(TMP_DIR)/sim.pid
PID_CONTROLLER     := $(TMP_DIR)/controller.pid
LOG_SIM            := $(TMP_DIR)/sim.log
LOG_CONTROLLER     := $(TMP_DIR)/controller.log
PORTMAN_ENV        := $(TMP_DIR)/portman.env

# ── Build ──

all: lint test build

web-install:
	@if [ -d web ]; then cd web && npm install; else echo "web/ not found, skipping npm install"; fi

web-build:
	@if [ -d web ] && [ -d web/node_modules ]; then \
		echo "==> Building web UI..."; \
		cd web && npm run build; \
	elif [ -d web ]; then \
		echo "==> web/node_modules not found, running npm install first..."; \
		cd web && npm install && npm run build; \
	else \
		echo "web/ not found, skipping web build"; \
	fi

build: web-install web-build
	go build -o bin/cirrus ./cmd/cirrus/
	go build -o bin/cirrusctl ./cmd/cirrusctl/

build-sim:
	go build -o bin/cirrus-sim ./cmd/cirrus-sim/
	go build -o bin/libvirtd-sim ./cmd/libvirtd-sim/
	go build -o bin/cirrus-sim-ctl ./cmd/cirrus-sim-ctl/

build-storage-servers:
	go build -o bin/cirrus-iscsi-server ./cmd/cirrus-iscsi-server/
	go build -o bin/cirrus-rbd-server ./cmd/cirrus-rbd-server/

# ── Proto ──

proto:
	protoc -I proto \
	  --go_out=proto/agentpb --go_opt=module=github.com/tjst-t/cirrus/proto/agentpb \
	  --go-grpc_out=proto/agentpb --go-grpc_opt=module=github.com/tjst-t/cirrus/proto/agentpb \
	  proto/agent.proto
	protoc -I proto \
	  --go_out=proto/networkpb --go_opt=module=github.com/tjst-t/cirrus/proto/networkpb \
	  --go-grpc_out=proto/networkpb --go-grpc_opt=module=github.com/tjst-t/cirrus/proto/networkpb \
	  proto/network.proto

# ── Test / Lint ──

test: test-unit

test-unit:
	go test ./...

test-mock:
	go test ./test/mock/...

test-smoke:
	@echo "==> Running network smoke tests (requires 'make serve')..."
	@./test/smoke/network_smoke_test.sh

test-integration:
	@$(COMPOSE) build
	@$(COMPOSE) up -d
	@echo "Integration environment is up. Running tests..."
	go test -tags integration -v -timeout 5m ./test/integration/...

test-e2e: web-install
	cd web && PLAYWRIGHT_CHROMIUM_PATH=$(shell ls $(HOME)/.cache/ms-playwright/chromium-*/chrome-linux*/chrome 2>/dev/null | head -1) npx playwright test --reporter=list

lint:
	golangci-lint run ./...

# ── Serve (sim + controller on host via portman, workers in docker) ──

serve: build build-sim
	@mkdir -p $(TMP_DIR)
	@# ── 1. Stop everything ──
	@$(MAKE) --no-print-directory stop
	@# ── 2. Allocate ports ──
	@$(MAKE) --no-print-directory _alloc-ports
	@# ── 3. Start sim (host) ──
	@$(MAKE) --no-print-directory _start-sim
	@# ── 4. Start controller (host) ──
	@$(MAKE) --no-print-directory _start-controller
	@# ── 5. Start worker containers ──
	@$(MAKE) --no-print-directory _start-workers
	@# ── 6. Seed topology ──
	@$(MAKE) --no-print-directory _seed-topology
	@# ── 7. Activate hosts ──
	@$(MAKE) --no-print-directory _activate-hosts
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo ""; \
	  echo "  ─────────────────────────────────────────"; \
	  echo "  All services running."; \
	  echo "  Dashboard        http://localhost:$$SIM_AGGREGATOR_PORT"; \
	  echo "  Controller API   http://localhost:$$API_PORT"; \
	  echo "  ─────────────────────────────────────────"; \
	  echo "  make logs        controller logs"; \
	  echo "  make logs-sim    simulator logs"; \
	  echo "  make logs-worker worker container logs"; \
	  echo "  make stop        stop all"; \
	  echo ""'

# ── Stop ──

stop:
	@# Stop worker containers (source env for port vars needed by compose)
	@bash -c 'set -a; [ -f $(PORTMAN_ENV) ] && source $(PORTMAN_ENV); set +a; \
	  CONTROLLER_HOST=$(CONTROLLER_HOST) $(COMPOSE) down --remove-orphans 2>/dev/null || true'
	@# Stop controller
	@if [ -f $(PID_CONTROLLER) ]; then \
	  PID=$$(cat $(PID_CONTROLLER)); \
	  if kill -0 $$PID 2>/dev/null; then \
	    echo "==> Stopping controller (PID: $$PID)..."; \
	    kill $$PID 2>/dev/null; \
	    for i in $$(seq 1 30); do kill -0 $$PID 2>/dev/null || break; sleep 0.1; done; \
	    kill -0 $$PID 2>/dev/null && kill -9 $$PID 2>/dev/null || true; \
	  fi; \
	  rm -f $(PID_CONTROLLER); \
	fi
	@# Stop cirrus-sim
	@if [ -f $(PID_SIM) ]; then \
	  PID=$$(cat $(PID_SIM)); \
	  if kill -0 $$PID 2>/dev/null; then \
	    echo "==> Stopping cirrus-sim (PID: $$PID)..."; \
	    kill $$PID 2>/dev/null; \
	    for i in $$(seq 1 50); do kill -0 $$PID 2>/dev/null || break; sleep 0.1; done; \
	    kill -0 $$PID 2>/dev/null && kill -9 $$PID 2>/dev/null || true; \
	  fi; \
	  rm -f $(PID_SIM); \
	fi

# ── Internal: allocate ports ──

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
	  --name worker-1 \
	  --name worker-2 \
	  --name worker-3 \
	  --name api:expose \
	  --name grpc \
	  --output $(PORTMAN_ENV)

# ── Internal: start sim (host) ──

_start-sim:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Starting cirrus-sim (env: $(CIRRUS_SIM_ENV))..."; \
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
	@echo "==> Waiting for cirrus-sim..."
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  for i in $$(seq 1 60); do \
	    curl -sf http://localhost:$$SIM_COMMON_PORT/api/v1/events >/dev/null 2>&1 && break; \
	    sleep 0.5; \
	  done; \
	  curl -sf http://localhost:$$SIM_COMMON_PORT/api/v1/events >/dev/null 2>&1 \
	    && echo "    cirrus-sim is ready." \
	    || { echo "ERROR: cirrus-sim failed. Check $(LOG_SIM)"; exit 1; }'

# ── Internal: start controller (host) ──

_start-controller:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  DB_DSN="postgresql://cirrus:cirrus@localhost:$$SIM_POSTGRES_PORT/cirrus?sslmode=disable"; \
	  echo "==> Starting controller (API: $$API_PORT, gRPC: $$GRPC_PORT)..."; \
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
	@echo "==> Waiting for controller..."
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  for i in $$(seq 1 60); do \
	    curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 && break; \
	    sleep 0.5; \
	  done; \
	  curl -sf http://localhost:$$API_PORT/healthz >/dev/null 2>&1 \
	    && echo "    Controller is ready." \
	    || { echo "ERROR: Controller not ready. Check $(LOG_CONTROLLER)"; exit 1; }'

# ── Internal: start workers (docker) ──

_start-workers:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Starting worker containers (gRPC: $$GRPC_PORT)..."; \
	  env \
	    CONTROLLER_HOST=$(CONTROLLER_HOST) \
	    GRPC_PORT=$$GRPC_PORT \
	    REGISTRATION_TOKEN=$(REGISTRATION_TOKEN) \
	    WORKER_1_PORT=$$WORKER_1_PORT \
	    WORKER_2_PORT=$$WORKER_2_PORT \
	    WORKER_3_PORT=$$WORKER_3_PORT \
	    $(COMPOSE) up -d --build 2>&1 | tail -5; \
	  echo "    Workers started."'

# ── Internal: seed topology ──

_seed-topology:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Seeding topology..."; \
	  TOKEN="$(firstword $(subst =, ,$(AUTH_TOKENS)))"; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default-sd\"}" \
	    http://localhost:$$API_PORT/api/v1/storage-domains >/dev/null 2>&1 || true; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default-site\",\"type\":\"site\"}" \
	    http://localhost:$$API_PORT/api/v1/locations >/dev/null 2>&1 || true; \
	  LOC_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/locations | jq -r ".[0].id"); \
	  SD_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/storage-domains | jq -r ".[0].id"); \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default-az\",\"location_id\":\"$$LOC_ID\"}" \
	    http://localhost:$$API_PORT/api/v1/admin/availability-zones >/dev/null 2>&1 || true; \
	  AZ_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/admin/availability-zones | jq -r ".[0].id"); \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"storage_domain_id\":\"$$SD_ID\"}" \
	    http://localhost:$$API_PORT/api/v1/admin/availability-zones/$$AZ_ID/storage-domains >/dev/null 2>&1 || true; \
	  echo "    Topology seeded."; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"m1.small\",\"vcpus\":1,\"ram_mb\":1024,\"disk_gb\":10}" \
	    http://localhost:$$API_PORT/api/v1/admin/flavors >/dev/null 2>&1 || true; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"m1.medium\",\"vcpus\":2,\"ram_mb\":4096,\"disk_gb\":20}" \
	    http://localhost:$$API_PORT/api/v1/admin/flavors >/dev/null 2>&1 || true; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"m1.large\",\"vcpus\":4,\"ram_mb\":8192,\"disk_gb\":40}" \
	    http://localhost:$$API_PORT/api/v1/admin/flavors >/dev/null 2>&1 || true; \
	  echo "    Default flavors seeded."; \
	  SD_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" http://localhost:$$API_PORT/api/v1/storage-domains | jq -r ".[0].id"); \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"sim-backend\",\"driver\":\"sim\",\"endpoint\":\"sim://local\",\"storage_domain_id\":\"$$SD_ID\",\"capacity_gb\":1000,\"driver_config\":{\"sim_backend_id\":\"ceph-pool-ssd\"}}" \
	    http://localhost:$$API_PORT/api/v1/admin/storage-backends >/dev/null 2>&1 || true; \
	  curl -sf -X POST \
	    -H "Authorization: Bearer $$TOKEN" \
	    -H "Content-Type: application/json" \
	    -d "{\"name\":\"default\",\"description\":\"Default sim volume type\",\"required_capabilities\":[],\"is_public\":true}" \
	    http://localhost:$$API_PORT/api/v1/admin/volume-types >/dev/null 2>&1 || true; \
	  echo "    Default storage backend and volume type seeded."; \
  ORG_ID=$$(curl -sf -X POST \
    -H "Authorization: Bearer $$TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"test-org\"}" \
    http://localhost:$$API_PORT/api/v1/organizations | jq -r ".id" 2>/dev/null); \
  if [ -n "$$ORG_ID" ] && [ "$$ORG_ID" != "null" ]; then \
    PSQL_URL="postgresql://cirrus:cirrus@localhost:$$SIM_POSTGRES_PORT/cirrus?sslmode=disable"; \
    psql "$$PSQL_URL" -c "INSERT INTO tenants (id, organization_id, name) VALUES ('\''4af01cf9-7325-4742-bf30-f1852368c1e8'\'', '\''$$ORG_ID'\'', '\''test-tenant'\'') ON CONFLICT (id) DO NOTHING" >/dev/null 2>&1 || true; \
    echo "    Test org and tenant seeded (fixed UUID)."; \
  fi'

# ── Internal: activate hosts ──

_activate-hosts:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Waiting for workers to register (up to 60s)..."; \
	  TOKEN="$(firstword $(subst =, ,$(AUTH_TOKENS)))"; \
	  for i in $$(seq 1 30); do \
	    REG=$$(curl -sf -H "Authorization: Bearer $$TOKEN" \
	      "http://localhost:$$API_PORT/api/v1/hosts?state=registering" 2>/dev/null | jq length 2>/dev/null || echo 0); \
	    ACT=$$(curl -sf -H "Authorization: Bearer $$TOKEN" \
	      "http://localhost:$$API_PORT/api/v1/hosts?state=active" 2>/dev/null | jq length 2>/dev/null || echo 0); \
	    [ "$$REG" -ge 1 ] && break; \
	    [ "$$ACT" -ge 1 ] && { echo "    Hosts already active, skipping wait."; break; }; \
	    sleep 2; \
	  done; \
	  echo "==> Activating all registered hosts..."; \
	  HOSTS=$$(curl -sf -H "Authorization: Bearer $$TOKEN" \
	    "http://localhost:$$API_PORT/api/v1/hosts?state=registering"); \
	  HOST_COUNT=$$(echo "$$HOSTS" | jq length 2>/dev/null || echo 0); \
	  SD_ID=$$(curl -sf -H "Authorization: Bearer $$TOKEN" \
	    "http://localhost:$$API_PORT/api/v1/storage-domains" | jq -r ".items[0].id // .[0].id // empty"); \
	  for i in $$(seq 0 $$((HOST_COUNT - 1))); do \
	    HOST_UUID=$$(echo "$$HOSTS" | jq -r ".[$${i}].id"); \
	    curl -sf -X POST \
	      -H "Authorization: Bearer $$TOKEN" \
	      -H "Content-Type: application/json" \
	      -d "{\"action\":\"activate\"}" \
	      http://localhost:$$API_PORT/api/v1/hosts/$$HOST_UUID/actions >/dev/null 2>&1 || true; \
	    if [ -n "$$SD_ID" ] && [ "$$SD_ID" != "null" ]; then \
	      curl -sf -X POST \
	        -H "Authorization: Bearer $$TOKEN" \
	        -H "Content-Type: application/json" \
	        -d "{\"storage_domain_id\":\"$$SD_ID\"}" \
	        http://localhost:$$API_PORT/api/v1/hosts/$$HOST_UUID/storage-domains >/dev/null 2>&1 || true; \
	    fi; \
	  done; \
	  echo "    Activated $$HOST_COUNT hosts and associated with default-sd"'

# ── Logs ──

logs:
	@tail -f $(LOG_CONTROLLER) 2>/dev/null || echo "No controller log. Run 'make serve' first."

logs-sim:
	@tail -f $(LOG_SIM) 2>/dev/null || echo "No sim log. Run 'make serve' first."

logs-worker:
	@bash -c 'set -a; [ -f $(PORTMAN_ENV) ] && source $(PORTMAN_ENV); set +a; \
	  CONTROLLER_HOST=$(CONTROLLER_HOST) $(COMPOSE) logs -f 2>/dev/null || echo "No worker containers running."'

# ── Clean ──

clean-dev:
	@$(MAKE) --no-print-directory stop
	@rm -rf $(TMP_DIR)
	@portman release --all 2>/dev/null || true
	@echo "Cleaned dev state."

# ── Database ──

reset-db:
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo "==> Resetting database..."; \
	  go run ./cmd/internal/resetdb "postgresql://cirrus:cirrus@localhost:$$SIM_POSTGRES_PORT/cirrus?sslmode=disable"; \
	  echo "    Database reset OK."'

fresh: build build-sim
	@mkdir -p $(TMP_DIR)
	@# ── 1. Stop everything ──
	@$(MAKE) --no-print-directory stop
	@# ── 2. Allocate ports ──
	@$(MAKE) --no-print-directory _alloc-ports
	@# ── 3. Start sim (postgres が起動するまで待つ) ──
	@$(MAKE) --no-print-directory _start-sim
	@# ── 4. Reset DB ──
	@$(MAKE) --no-print-directory reset-db
	@# ── 5. Start controller (migrations を実行) ──
	@$(MAKE) --no-print-directory _start-controller
	@# ── 6. Start worker containers ──
	@$(MAKE) --no-print-directory _start-workers
	@# ── 7. Seed topology ──
	@$(MAKE) --no-print-directory _seed-topology
	@# ── 8. Activate hosts ──
	@$(MAKE) --no-print-directory _activate-hosts
	@bash -c '\
	  set -a; source $(PORTMAN_ENV); set +a; \
	  echo ""; \
	  echo "  ─────────────────────────────────────────"; \
	  echo "  All services running (fresh DB)."; \
	  echo "  Dashboard        http://localhost:$$SIM_AGGREGATOR_PORT"; \
	  echo "  Controller API   http://localhost:$$API_PORT"; \
	  echo "  ─────────────────────────────────────────"; \
	  echo "  make logs        controller logs"; \
	  echo "  make logs-sim    simulator logs"; \
	  echo "  make logs-worker worker container logs"; \
	  echo "  make stop        stop all"; \
	  echo ""'

# ── Storage Layer (local dev only: iSCSI + Ceph) ──

serve-storage:
	@bash -c 'set -a; [ -f $(PORTMAN_ENV) ] && source $(PORTMAN_ENV); set +a; \
	  echo "==> Starting storage layer (iSCSI + Ceph)..."; \
	  env CONTROLLER_HOST=$(CONTROLLER_HOST) \
	    $(COMPOSE_STORAGE) up -d --build iscsi-target ceph 2>&1 | tail -10; \
	  echo "  iSCSI target  http://localhost:18080 (mgmt)  tcp://10.100.0.100:3260 (iSCSI)"; \
	  echo "  Ceph (RBD)    http://localhost:18090 (mgmt)  tcp://10.100.0.101:6789 (mon)"'

stop-storage:
	@bash -c 'set -a; [ -f $(PORTMAN_ENV) ] && source $(PORTMAN_ENV); set +a; \
	  CONTROLLER_HOST=$(CONTROLLER_HOST) $(COMPOSE_STORAGE) stop iscsi-target ceph 2>/dev/null || true'
