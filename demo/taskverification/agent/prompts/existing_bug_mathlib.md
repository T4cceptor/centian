Connect to the MCP server named `centian` and solve this Python TDD task entirely through Centian-exposed tools.

Problem:

- There is an existing bug in `/workspace/project/mathlib.py`.
- The behavior should be simple integer addition.
- There is already a pytest file in `/workspace/project/tests/test_mathlib.py`.
- Your job is to derive the right task parameters from the project, establish the failing baseline through Centian, then implement the fix.

Rules:

- Use `centian.task_*` to drive the task lifecycle.
- Use only Centian-exposed MCP tools for project access.
- Use `filesystem___*` tools to inspect and edit files.
- Use `shell___*` tools only when you need focused local context.
- Do not use local container filesystem access as a substitute for project access.
- Do not call `centian.task_fail` unless recovery is impossible.
- Prefer `centian.task_complete_step` over manually re-running the full validation flow yourself.
- Read and follow the task instructions returned by Centian after listing templates or registering the task.

Bootstrap the task like this:

1. Call `centian.task_list_templates`.
2. Choose the generic Python TDD workflow template.
3. Derive the required parameter values from the project files and the failing behavior.
4. Call `centian.task_register` with the correct template id and parameter values you derived.
5. Use the instructions returned by Centian to drive the remaining step lifecycle.
6. Stop after the task is completed and provide a short summary of the change and which Centian steps passed.
