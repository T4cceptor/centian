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
- Create a production module under `/workspace/project`.
- Create a pytest file under `/workspace/project/tests`.
- Derive the task parameters yourself from the files and failure you introduce.
- Establish a failing test first, then implement the solution through the Centian workflow.

Rules:

- Use `centian.task_*` to drive the task lifecycle.
- Use only Centian-exposed MCP tools for project access.
- Use `filesystem___*` tools to inspect and edit files.
- Use `shell___*` tools only when you need focused local context.
- Do not use local container filesystem access as a substitute for project access.
- Do not call `centian.task_fail` unless recovery is impossible.
- Prefer `centian.task_complete_step` over manually re-running the full validation flow yourself.
- Read and follow the task instructions and structured workflow state returned by Centian after each lifecycle call.
- Use Centian's `phase`, `currentNodeKind`, `nextNodePath`, `allowedTools`, and failure diagnostics as the source of truth.

Bootstrap the task like this:

1. Call `centian.task_list_templates`.
2. Choose the generic Python TDD workflow template.
3. Create the minimal files you need so the task parameters become well-defined.
4. Derive the required parameter values from the project files and the failing behavior.
5. Call `centian.task_register` with the correct template id and parameter values you derived.
6. Call `centian.task_complete_onboarding` with a concise project summary, useful artifact map, common commands, and any constraints or open questions.
7. Call `centian.task_complete_planning` with the required planning artifact, including selected files, test target, lint command, expected failure, and implementation target.
8. Only after planning has moved the workflow into execution, use `centian.task_start_step` and `centian.task_complete_step` in order.
9. Stop after the task is completed and provide a short summary of the change and which Centian lifecycle steps passed.
