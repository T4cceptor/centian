Connect to the MCP server named `centian` and solve this Python TDD task entirely through Centian-exposed tools.

Problem:

- There is an existing bug in `/workspace/project/mathlib.py`.
- The behavior should be simple integer addition.
- There is already a pytest file in `/workspace/project/tests/test_mathlib.py`, but for this workflow you should create a new focused pytest file for the bug during scaffolding.
- Register the task before using any project-access MCP tool.
- Your job is to establish the failing baseline through Centian, then implement the fix.

Rules:

- Use `centian.task_*` to drive the task lifecycle.
- Use only Centian-exposed MCP tools for project access.
- Use `filesystem___*` tools to inspect and edit files.
- Use `shell___*` tools only when you need focused local context.
- For compound shell commands or directory changes, use `bash -lc '...'`.
- Do not use local container filesystem access as a substitute for project access.
- Do not call `centian.task_fail` unless recovery is impossible.
- Prefer `centian.task_complete_step` over manually re-running the full validation flow yourself.
- Read and follow the task instructions and structured workflow state returned by Centian after each lifecycle call.
- Use Centian's `phase`, `currentNodeKind`, `nextNodePath`, `allowedTools`, and failure diagnostics as the source of truth.

Bootstrap the task like this:

1. Call `centian.task_list_templates`.
2. Choose the generic Python TDD workflow template.
3. Call `centian.task_register` immediately with:
   - `templateId`: `python_tdd_workflow`
4. Call `centian.task_complete_onboarding` with a concise project summary, relevant artifact map, and the planned commands.
5. Call `centian.task_complete_planning` with the required planning artifact, including:
   - `selectedFiles`: `/workspace/project/mathlib.py` and `/workspace/project/tests/test_mathlib_addition.py`
   - `parameters.testCommand`: `python -m pytest -q`
   - `parameters.testFile`: `tests/test_mathlib_addition.py`
   - `parameters.testName`: `test_add_two_numbers`
   - `parameters.testTarget`: `tests/test_mathlib_addition.py::test_add_two_numbers`
   - `parameters.lintCommand`: `python -m ruff check .`
   - `parameters.expectedError`: `AssertionError: assert -1 == 3`
   - `parameters.implementationTarget`: `/workspace/project/mathlib.py`
6. After planning enters `scaffolding`, run all four steps in order.
7. In scaffolding, create the new focused test file and leave the existing implementation file in place.
8. In execution step 3, verify the new targeted test fails for the expected reason without extra edits.
9. In execution step 4, fix `/workspace/project/mathlib.py`, keep the new test file stable, and complete the task.
10. Stop after the task is completed and provide a short summary of the change and which Centian lifecycle steps passed.
