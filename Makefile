GO := /usr/local/go/bin/go

PID_CONTROLLER := /tmp/cirrus-controller.pid
PID_WORKER := /tmp/cirrus-worker.pid
LOG_CONTROLLER := /tmp/cirrus-controller.log
LOG_WORKER := /tmp/cirrus-worker.log
PORTMAN_ENV := /tmp/cirrus-portman.env
BIN := ./cirrus

.PHONY: build serve serve-worker stop logs db-up db-down db-reset

build:
	$(GO) build -o $(BIN) ./cmd/cirrus/

serve: build db-up
	@# Stop previous controller
	@if [ -f $(PID_CONTROLLER) ]; then \
	  OLD_PID=$$(cat $(PID_CONTROLLER)); \
	  if kill -0 $$OLD_PID 2>/dev/null; then \
	    echo "==> Stopping previous controller (PID: $$OLD_PID)..."; \
	    kill $$OLD_PID; \
	    for i in $$(seq 1 50); do kill -0 $$OLD_PID 2>/dev/null || break; sleep 0.1; done; \
	    kill -0 $$OLD_PID 2>/dev/null && kill -9 $$OLD_PID 2>/dev/null || true; \
	  fi; \
	  rm -f $(PID_CONTROLLER); \
	fi
	@portman env --name cirrus --expose --output $(PORTMAN_ENV)
	@. $(PORTMAN_ENV) && \
	  sed "s/listen: .*/listen: \"0.0.0.0:$$CIRRUS_PORT\"/" dev/controller.yaml > /tmp/cirrus-controller.yaml && \
	  echo "==> Starting controller on port $$CIRRUS_PORT (log: $(LOG_CONTROLLER))" && \
	  nohup $(BIN) controller --config=/tmp/cirrus-controller.yaml > $(LOG_CONTROLLER) 2>&1 & \
	  echo $$! > $(PID_CONTROLLER) && \
	  echo "    PID: $$(cat $(PID_CONTROLLER))" && \
	  sleep 1 && \
	  if ! kill -0 $$(cat $(PID_CONTROLLER)) 2>/dev/null; then \
	    echo "    ERROR: Controller failed to start. Check $(LOG_CONTROLLER)"; \
	    cat $(LOG_CONTROLLER); \
	    exit 1; \
	  fi && \
	  echo "    OK: Controller is running"

serve-worker: build
	@# Stop previous worker
	@if [ -f $(PID_WORKER) ]; then \
	  OLD_PID=$$(cat $(PID_WORKER)); \
	  if kill -0 $$OLD_PID 2>/dev/null; then \
	    echo "==> Stopping previous worker (PID: $$OLD_PID)..."; \
	    kill $$OLD_PID; \
	    for i in $$(seq 1 50); do kill -0 $$OLD_PID 2>/dev/null || break; sleep 0.1; done; \
	    kill -0 $$OLD_PID 2>/dev/null && kill -9 $$OLD_PID 2>/dev/null || true; \
	  fi; \
	  rm -f $(PID_WORKER); \
	fi
	@portman env --name cirrus-worker --output $(PORTMAN_ENV).worker
	@. $(PORTMAN_ENV) && . $(PORTMAN_ENV).worker && \
	  sed -e "s/listen: .*/listen: \"0.0.0.0:$$CIRRUS_WORKER_PORT\"/" \
	      -e "s/advertise: .*/advertise: \"localhost:$$CIRRUS_WORKER_PORT\"/" \
	      -e "s/controller_addr: .*/controller_addr: \"localhost:$$CIRRUS_PORT\"/" \
	      dev/worker.yaml > /tmp/cirrus-worker.yaml && \
	  echo "==> Starting worker on port $$CIRRUS_WORKER_PORT (controller: $$CIRRUS_PORT) (log: $(LOG_WORKER))" && \
	  nohup $(BIN) worker --config=/tmp/cirrus-worker.yaml > $(LOG_WORKER) 2>&1 & \
	  echo $$! > $(PID_WORKER) && \
	  echo "    PID: $$(cat $(PID_WORKER))" && \
	  sleep 1 && \
	  if ! kill -0 $$(cat $(PID_WORKER)) 2>/dev/null; then \
	    echo "    ERROR: Worker failed to start. Check $(LOG_WORKER)"; \
	    cat $(LOG_WORKER); \
	    exit 1; \
	  fi && \
	  echo "    OK: Worker is running"

stop:
	@if [ -f $(PID_WORKER) ]; then kill $$(cat $(PID_WORKER)) 2>/dev/null; rm -f $(PID_WORKER); echo "Worker stopped"; fi
	@if [ -f $(PID_CONTROLLER) ]; then kill $$(cat $(PID_CONTROLLER)) 2>/dev/null; rm -f $(PID_CONTROLLER); echo "Controller stopped"; fi

logs:
	@echo "=== Controller ===" && tail -20 $(LOG_CONTROLLER) 2>/dev/null || true
	@echo "" && echo "=== Worker ===" && tail -20 $(LOG_WORKER) 2>/dev/null || true

db-up:
	@sudo docker-compose up -d 2>/dev/null || true

db-down:
	@sudo docker-compose down 2>/dev/null || true

db-reset:
	@sudo docker exec cirrus_postgres_1 psql -U cirrus -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" 2>/dev/null
	@echo "Database reset"
