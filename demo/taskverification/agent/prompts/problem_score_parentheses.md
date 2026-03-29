Connect to the MCP server named `centian` and solve this Python TDD task entirely through Centian-exposed tools.

Problem:

Implement a Python function named `score_parentheses(text: str) -> int`.

The input string is guaranteed to be balanced and consists only of `(` and `)`.

Scoring rules:

- `()` has score `1`
- `AB` has score `A + B`, where `A` and `B` are balanced parentheses strings
- `(A)` has score `2 * A`

Requirements:

- Start from the problem statement only. There is no provided implementation or test for this exercise.
- Register the task before using any project-access MCP tool.
- Use scaffolding to create a production module under `/workspace/project` and a pytest file under `/workspace/project/tests`.
- Establish a failing test first, then implement the solution through the Centian workflow.

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
3. Call `centian.task_register` immediately with these parameters:
   - `templateId`: `python_tdd_workflow`
   - `testCommand`: `python -m pytest -q`
   - `testFile`: `tests/test_score_parentheses.py`
   - `testName`: `test_score_parentheses_examples`
   - `testTarget`: `tests/test_score_parentheses.py::test_score_parentheses_examples`
   - `lintCommand`: `python -m ruff check .`
   - `expectedError`: `AssertionError: assert 0 == 1`
   - `implementationTarget`: `/workspace/project/score_parentheses.py`
4. Call `centian.task_complete_onboarding` with a concise summary of the planned new module and test, plus useful commands and constraints.
5. Call `centian.task_complete_planning` with the required planning artifact, including:
   - `selectedFiles`: `/workspace/project/score_parentheses.py` and `/workspace/project/tests/test_score_parentheses.py`
   - `testTarget`: `tests/test_score_parentheses.py::test_score_parentheses_examples`
   - `lintCommand`: `python -m ruff check .`
   - `expectedFailure`: `AssertionError: assert 0 == 1`
   - `implementationTarget`: `/workspace/project/score_parentheses.py`
6. After planning enters `scaffolding`, run all four steps in order with `centian.task_start_step` and `centian.task_complete_step`:
   - `setup_test_file`
   - `setup_test_scaffolding`
   - `establish_failing_baseline`
   - `implement_solution`
7. In scaffolding, create the new test file and the minimal production stub needed for the planned failing baseline.
8. In execution step 3, verify the failing baseline without additional edits.
9. In execution step 4, implement the solution in the production module only and keep the test file stable.
10. Stop after the task is completed and provide a short summary of the change and which Centian lifecycle steps passed.
