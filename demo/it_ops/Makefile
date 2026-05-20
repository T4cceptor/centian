SHELL := /bin/bash

COMPOSE := docker compose -f $(CURDIR)/docker-compose.yml
CENTIAN_HOST_PORT ?= 8577
POSTGRES_HOST_PORT ?= 55432

.PHONY: build-centian build-processor postgres-up postgres-down run-centian clean

build-centian:
	@$(MAKE) -C "$(CURDIR)/../.." build-go

build-processor:
	@$(MAKE) -C "$(CURDIR)/../.." processor

postgres-up:
	@POSTGRES_HOST_PORT="$(POSTGRES_HOST_PORT)" $(COMPOSE) up -d
	@echo "Postgres is starting on 127.0.0.1:$(POSTGRES_HOST_PORT)."

postgres-down:
	@$(COMPOSE) down --remove-orphans
	@echo "IT ops Postgres is down."

run-centian: build-centian build-processor
	@mkdir -p "$(CURDIR)/.centian"
	@echo "Starting Centian IT ops demo on http://127.0.0.1:$(CENTIAN_HOST_PORT)/mcp/it-ops"
	@cd "$(CURDIR)/../.." && ./build/centian start --config-path "$(CURDIR)/configs/it_ops_config.json"

clean:
	@rm -rf "$(CURDIR)/.centian"
	@echo "Removed IT ops local artifacts."
