SHELL := /bin/bash

ENV_FILE := $(if $(wildcard $(CURDIR)/.env.local),.env.local,.env.example)
COMPOSE  := docker compose -f $(CURDIR)/docker-compose.yml --env-file $(CURDIR)/$(ENV_FILE)

CENTIAN_HOST_PORT ?= 8576
JAEGER_UI_HOST_PORT ?= 16686

.PHONY: setup clean demo-up demo-up-local demo-test demo-down

setup:
	@bash "$(CURDIR)/scripts/setup.sh"

clean:
	@find "$(CURDIR)" -type d -name "__pycache__" -prune -exec rm -rf {} +
	@echo "Removed demo local artifacts."

demo-up:
	go build -o build/prompt-injection-guard ../it_ops/prompt_injection_guard
	@$(MAKE) -C "$(CURDIR)/../.." build
	@$(COMPOSE) up -d --build
	@echo "Demo gateways are up:"
	@echo "  - http://localhost:$(CENTIAN_HOST_PORT)/mcp/logging-demo"
	@echo "  - http://localhost:$(CENTIAN_HOST_PORT)/mcp/modification-demo"
	@echo "Auth is disabled for demo mode (no X-Centian-Auth header required)."
	@echo "Warning: this no-auth setup is only for local demo/testing."
	@echo "Jaeger UI: http://localhost:$(JAEGER_UI_HOST_PORT)"
	@echo "No Jaeger login or DSN setup required for this demo path."
	@echo "Aggregated gateway endpoints in use; tools are namespaced (e.g. logging-demo-db___query)."

demo-up-local:
	@$(MAKE) demo-up

demo-test:
	@bash "$(CURDIR)/scripts/smoke_demo.sh" all "$(CENTIAN_HOST_PORT)" "$(JAEGER_UI_HOST_PORT)"

demo-down:
	@$(COMPOSE) down --remove-orphans
	@echo "Demo services are down."
