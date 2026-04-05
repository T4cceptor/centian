# Centian

**Project Name:** Centian

**Repository:** https://github.com/T4cceptor/centian
**Repo Owner:** T4cceptor (User)
**Repo Name:** centian

**Description:** A CLI tool to proxy MCP servers, collect and configure their configurations at a single place, and enable lifecycle hooks for tool requests and their responses.

## Purpose

Centian serves as a centralized proxy for Model Context Protocol (MCP) servers, providing:

- **MCP Server Proxy**: Acts as a proxy layer for multiple MCP servers
- **Centralized Configuration**: Collect and manage configurations for all MCP servers in one place
- **Lifecycle Hooks**: Enable custom hooks for tool requests and their responses
- **Request/Response Management**: Handle and transform MCP tool requests and responses

## Architecture

The CLI tool is built in Go and provides a unified interface to interact with multiple MCP servers while offering configuration management and extensibility through lifecycle hooks.


## Global Configuration System

  Key Features:
  - Config Location: ~/.centian/config.json
  - Auto-initialization: Creates default config if none exists
  - Server Management: Add, remove, enable/disable MCP servers
  - Lifecycle Hooks: Pre/post request hooks and connection events
  - Validation: Built-in config validation
  - Project-based isolation: Optional multi-project layout for tenant separation

  Main Components:

  1. GlobalConfig - Root configuration structure with proxy settings and either flat gateways (legacy) or named projects
  2. ProjectConfig - Per-project configuration holding gateways, processors, capabilities, auth, and metadata. Each project gets its own SQLite database, route prefix, and feature flags
  3. MCPServer - Individual server configurations with command, args, environment variables
  4. ProxySettings - Bind address, port, logging, timeouts (truly global settings)
  5. HookSettings - Lifecycle hooks for request/response interception

  Config Layouts:
  - Legacy flat: gateways, processors, auth at the top level of GlobalConfig (auto-migrated to a "default" project at runtime via ResolveProjects())
  - Project-based: named ProjectConfig entries under "projects", each with its own gateways, processors, capabilities, and auth

  Route Structure:
  - Default project (or legacy layout): /mcp/<gateway>/<server>, /ui, /api/task-runs
  - Named projects: /<project_slug>/mcp/<gateway>/<server>, /<project_slug>/ui

  CLI Commands:
  centian config init             # Initialize default config
  centian config show             # Display current config
  centian config validate         # Validate config file
  centian config server list      # List all servers
  centian config server add       # Add new server
  centian config server remove    # Remove server
  centian config server enable    # Enable server
  centian config server disable   # Disable server

## Debugging

General rules:
- if a bug persists more than 2 edits, write a test case first, run the test case and make sure it FAILS (after all at this point the bug is not yet fixed), THEN start debugging
- Start with an architecture analysis, instead of fixing symptoms check what the root cause might be and give an overview/approach first instead of making edits directly

## Testing

- Use "Given-when-then" structure
- Example:
```
// Given: a VSCode Discoverer
discoverer := VSCodeDiscoverer(config)

// When: running the discovery process using the discoverer
result, err := discoverer.discover()

// Then: 2 config files are found in the given location, with 3 servers each, and 1 duplicate each
<assert statements>
```

## General development

- commit after finalizing a significant portion of the task (e.g. 20, 30, 50, 70, 90%)
- call out edge cases in the code, but do not handle them immediately if they are unlikely or unexpected
