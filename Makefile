.PHONY: help dev build test demo demo-full report clean

PORT ?= 8080

help:
	@echo "Mobile Check Deposit System — available targets:"
	@echo ""
	@echo "  make           Show this help"
	@echo "  make dev       Build, seed, and start server (http://localhost:$(PORT))"
	@echo "  make build     Compile ./cmd/server to ./bin/server"
	@echo "  make test      Run all tests (go test ./...)"
	@echo "  make demo      Run 6-act demo script against running server"
	@echo "  make demo-full Start server, run demo, stop server (self-contained)"
	@echo "  make report    Generate test coverage + demo reports in /reports"
	@echo "  make clean     Remove bin, data, and generated artifacts"

dev: data reports
	@[ -f .env ] || (cp .env.example .env && echo "Created .env from .env.example")
	@$(MAKE) build
	@echo ""
	@echo "Starting server..."
	@echo "  Dashboard:  http://localhost:$(PORT)"
	@echo "  Submit:     http://localhost:$(PORT)/submit"
	@echo "  Operator:   http://localhost:$(PORT)/operator"
	@echo "  API:        http://localhost:$(PORT)/api/v1/deposits"
	@echo "  Health:     http://localhost:$(PORT)/health"
	@echo ""
	PORT=$(PORT) ./bin/server

data:
	@mkdir -p data data/images

reports:
	@mkdir -p reports

build:
	@mkdir -p bin
	CGO_ENABLED=1 go build -o bin/server ./cmd/server

test:
	go test -count=1 ./...

demo:
	@./scripts/demo.sh http://localhost:$(PORT)

demo-full: build data reports
	@echo "Starting server in background on port $(PORT)..."
	@rm -f data/apex.db data/apex.db-shm data/apex.db-wal
	@PORT=$(PORT) ./bin/server & SERVER_PID=$$!; \
	echo "Server PID: $$SERVER_PID"; \
	for i in 1 2 3 4 5 6 7 8 9 10; do \
		if curl -s http://localhost:$(PORT)/health >/dev/null 2>&1; then \
			break; \
		fi; \
		sleep 0.5; \
	done; \
	if ! curl -s http://localhost:$(PORT)/health >/dev/null 2>&1; then \
		echo "ERROR: Server failed to start"; \
		kill $$SERVER_PID 2>/dev/null; \
		exit 1; \
	fi; \
	echo "Server ready."; \
	echo ""; \
	./scripts/demo.sh http://localhost:$(PORT) 2>&1 | tee reports/demo_results.txt; \
	DEMO_EXIT=$$?; \
	kill $$SERVER_PID 2>/dev/null; \
	wait $$SERVER_PID 2>/dev/null; \
	echo ""; \
	echo "Server stopped. Demo results saved to reports/demo_results.txt"; \
	exit $$DEMO_EXIT

report: reports
	@echo "Generating test coverage report..."
	go test -count=1 -coverprofile=reports/coverage.out ./...
	@go tool cover -func=reports/coverage.out | tail -1
	@echo ""
	@echo "Coverage report: reports/coverage.out"
	@echo "View HTML: go tool cover -html=reports/coverage.out"

clean:
	rm -rf bin
	rm -f data/apex.db data/apex.db-shm data/apex.db-wal
	rm -f reports/*.out reports/*.txt
	@echo "Cleaned."
