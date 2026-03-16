SHELL := /bin/bash

ENV_FILE := $(if $(wildcard $(CURDIR)/.env.local),.env.local,.env.example)
COMPOSE        := docker compose -f $(CURDIR)/docker-compose.yml --env-file $(CURDIR)/$(ENV_FILE)
COMPOSE_LOCAL  := $(COMPOSE) -f $(CURDIR)/docker-compose.local.yml
CENTIAN_SERVICE := centian-demo

CENTIAN_HOST_PORT ?= 8576
JAEGER_UI_HOST_PORT ?= 16686

.PHONY: setup clean demo-up demo-up-local demo-test demo-down

setup:
	@bash "$(CURDIR)/scripts/setup.sh"

clean:
	@find "$(CURDIR)" -type d -name "__pycache__" -prune -exec rm -rf {} +
	@echo "Removed demo local artifacts."

# Pull the published Docker Hub image and start all services (no local build needed)
demo-up:
	@$(COMPOSE) pull $(CENTIAN_SERVICE)
	@$(COMPOSE) up -d
	@echo "Demo gateways are up:"
	@echo "  - http://localhost:$(CENTIAN_HOST_PORT)/mcp/logging-demo"
	@echo "  - http://localhost:$(CENTIAN_HOST_PORT)/mcp/modification-demo"
	@echo "Auth is disabled for demo mode (no X-Centian-Auth header required)."
	@echo "Warning: this no-auth setup is only for local demo/testing."
	@echo "Jaeger UI: http://localhost:$(JAEGER_UI_HOST_PORT)"
	@echo "No Jaeger login or DSN setup required for this demo path."
	@echo "Aggregated gateway endpoints in use; tools are namespaced (e.g. logging-demo-db___query)."

# Build Centian from source and start all services (for contributors / local development)
demo-up-local:
	@$(COMPOSE_LOCAL) up -d --build
	@echo "Demo gateways are up (local build):"
	@echo "  - http://localhost:$(CENTIAN_HOST_PORT)/mcp/logging-demo"
	@echo "  - http://localhost:$(CENTIAN_HOST_PORT)/mcp/modification-demo"
	@echo "Auth is disabled for demo mode (no X-Centian-Auth header required)."
	@echo "Warning: this no-auth setup is only for local demo/testing."
	@echo "Jaeger UI: http://localhost:$(JAEGER_UI_HOST_PORT)"
	@echo "No Jaeger login or DSN setup required for this demo path."
	@echo "Aggregated gateway endpoints in use; tools are namespaced (e.g. logging-demo-db___query)."

demo-test:
	@bash "$(CURDIR)/scripts/smoke_demo.sh" all "$(CENTIAN_HOST_PORT)" "$(JAEGER_UI_HOST_PORT)"

demo-down:
	@$(COMPOSE) down --remove-orphans
	@echo "Demo services are down."
