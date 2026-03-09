.PHONY: help dev build test clean

help:
	@echo "Mobile Check Deposit — available targets:"
	@echo "  make        — show this help"
	@echo "  make dev    — create dirs, copy .env if needed, build and run server"
	@echo "  make build  — compile ./cmd/server to ./bin/server"
	@echo "  make test   — run tests"
	@echo "  make clean  — remove bin and generated artifacts"

dev: data reports
	@[ -f .env ] || (cp .env.example .env && echo "Created .env from .env.example")
	@$(MAKE) build
	@echo "Starting server — http://localhost:$${PORT:-8080}"
	@./bin/server

data:
	@mkdir -p data
	@mkdir -p data/images

reports:
	@mkdir -p reports

build:
	@mkdir -p bin
	go build -o bin/server ./cmd/server

test:
	go test ./...

clean:
	rm -rf bin
	rm -f reports/*.out reports/*.txt
	@echo "Cleaned."
