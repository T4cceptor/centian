# Centian - Task Tempaltes

Task templates are simple yaml files that outline the process for a specific task - e.g. test-driven development, technical research spike, report generation, etc.

They are used in the taskverification functionality of centian to define the overall process, boundaries for each step in the process (e.g. limiting MCP capabilities of an agent), checks and verifications.

**Important**
File included in the `integrated/` directory are included in the final binary build of centian. Do not move any templates there for testing, development, or prototyping. Only refined, and properly tested templates should be included in `integrated/`.