SHELL := /bin/bash

COMPOSE := docker compose -f $(CURDIR)/docker-compose.yml
CENTIAN_SERVICE := centian
AGENT_SERVICE := agent
PROMPT_EXISTING := /agent/prompts/existing_bug_mathlib.md
PROMPT_PROBLEM := /agent/prompts/problem_score_parentheses.md
PROMPT_APPROVAL := /agent/prompts/approval_wait_mathlib.md
GO_E2E := CENTIAN_RUN_TASKVERIFICATION_INTEGRATION=1 GOCACHE=/tmp/go-build go test -v .

.PHONY: build demo-up run-agent run-agent-existing run-agent-problem run-agent-approval demo-test demo-test-existing demo-test-problem demo-test-approval e2e e2e-existing e2e-problem e2e-approval demo-down clean-artifacts reset

build:
	@mkdir -p "$(CURDIR)/artifacts/logs"
	@$(COMPOSE) build

demo-up:
	@mkdir -p "$(CURDIR)/artifacts/logs"
	@$(COMPOSE) up -d $(CENTIAN_SERVICE)
	@echo "Centian task verification demo is available at http://127.0.0.1:$${CENTIAN_TASKVERIFICATION_PORT:-8678}/mcp/taskverification"

run-agent:
	@mkdir -p "$(CURDIR)/artifacts/logs"
	@PROMPT_PATH="$(PROMPT_PROBLEM)" $(COMPOSE) run --rm $(AGENT_SERVICE)

run-agent-existing:
	@mkdir -p "$(CURDIR)/artifacts/logs"
	@PROMPT_PATH="$(PROMPT_EXISTING)" $(COMPOSE) run --rm $(AGENT_SERVICE)

run-agent-problem:
	@mkdir -p "$(CURDIR)/artifacts/logs"
	@PROMPT_PATH="$(PROMPT_PROBLEM)" $(COMPOSE) run --rm $(AGENT_SERVICE)

run-agent-approval:
	@mkdir -p "$(CURDIR)/artifacts/logs"
	@PROMPT_PATH="$(PROMPT_APPROVAL)" $(COMPOSE) run --rm $(AGENT_SERVICE)

demo-test:
	@$(MAKE) e2e-problem

demo-test-existing:
	@$(MAKE) e2e-existing

demo-test-problem:
	@$(MAKE) e2e-problem

demo-test-approval:
	@$(MAKE) e2e-approval

e2e:
	@cd "$(CURDIR)" && $(GO_E2E) -run 'TestTaskVerificationE2E'

e2e-existing:
	@cd "$(CURDIR)" && CENTIAN_TASKVERIFICATION_SCENARIO=existing_bug $(GO_E2E) -run 'TestTaskVerificationE2E'

e2e-problem:
	@cd "$(CURDIR)" && CENTIAN_TASKVERIFICATION_SCENARIO=problem_only $(GO_E2E) -run 'TestTaskVerificationE2E'

e2e-approval:
	@cd "$(CURDIR)" && CENTIAN_TASKVERIFICATION_SCENARIO=approval_wait $(GO_E2E) -run 'TestTaskVerificationE2E'

demo-down:
	@$(COMPOSE) down --remove-orphans

clean-artifacts:
	@find "$(CURDIR)/artifacts" -mindepth 1 -maxdepth 1 ! -name ".gitkeep" -exec rm -rf {} +

reset:
	@printf 'def add(a: int, b: int) -> int:\n    return a - b\n' > "$(CURDIR)/project/mathlib.py"
	@printf 'from mathlib import add\n\n\ndef test_add_two_numbers() -> None:\n    assert add(1, 2) == 3\n' > "$(CURDIR)/project/tests/test_mathlib.py"
	@rm -f "$(CURDIR)/project/score_parentheses.py"
	@rm -f "$(CURDIR)/project/tests/test_score_parentheses.py"
	@echo "Reset demo project to the initial failing state."
