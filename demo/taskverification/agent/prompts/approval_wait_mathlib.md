Connect to the MCP server named `centian` and drive a task into an explicit approval-wait node entirely through Centian-exposed tools.

Problem:

- There is an existing bug in `/workspace/project/mathlib.py`.
- The behavior should be simple integer addition.
- There is already a pytest file in `/workspace/project/tests/test_mathlib.py`.
- Your goal is to derive the right task parameters, complete onboarding, complete planning, verify the workflow enters an approval wait, prove proxied tools are blocked there, and then stop.

Rules:

- Use `centian.task_*` to drive the task lifecycle.
- Use only Centian-exposed MCP tools for project access.
- Use `filesystem___*` tools to inspect files when needed before the approval wait.
- Use `shell___*` tools only when you need focused local context.
- For compound shell commands or directory changes, use `bash -lc '...'`.
- Do not use local container filesystem access as a substitute for project access.
- Do not call `centian.task_fail` unless recovery is impossible.
- Read and follow the task instructions and structured workflow state returned by Centian after each lifecycle call.
- Use Centian's `phase`, `currentNodeKind`, `nextNodePath`, `allowedTools`, and failure diagnostics as the source of truth.
- Do not attempt to start execution steps after the approval wait is reached.

Bootstrap the task like this:

1. Call `centian.task_list_templates`.
2. Choose the `python_tdd_approval_wait` workflow template.
3. Derive the required parameter values from the project files and the existing failing behavior.
4. Call `centian.task_register` with the correct template id and parameter values you derived.
5. Call `centian.task_complete_onboarding` with a concise project summary, relevant artifact map, and useful commands.
6. Call `centian.task_complete_planning` with the required planning artifact, including selected files, test target, lint command, expected failure, and implementation target.
7. Verify the resulting task state is in `waiting_for_approval.review_plan`.
8. While still in that approval-wait node, intentionally call one proxied tool such as `shell___exec` with a harmless command like `pwd` and verify Centian blocks it.
9. Stop there and provide a short summary that the task is paused waiting for approval and that the blocked tool behavior was confirmed.
