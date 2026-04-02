# Centian Task Templates

Task templates are YAML files used by Centian taskverification to define a task workflow, its parameters, tool boundaries, checks, and invariants.

Canonical authoring guide:

- [Task Template Authoring](../docs/task-template-authoring.md)

Packaging rule:

- files in `task-templates/integrated/` are embedded into the Centian binary
- files in `task-templates/` outside `integrated/` are runtime disk templates only

Only refined and tested templates should be moved into `integrated/`.
