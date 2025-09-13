# Centian CLI

**Project Name:** Centian CLI

**Description:** A CLI tool to proxy MCP servers, collect and configure their configurations at a single place, and enable lifecycle hooks for tool requests and their responses.

## Purpose

Centian CLI serves as a centralized proxy for Model Context Protocol (MCP) servers, providing:

- **MCP Server Proxy**: Acts as a proxy layer for multiple MCP servers
- **Centralized Configuration**: Collect and manage configurations for all MCP servers in one place
- **Lifecycle Hooks**: Enable custom hooks for tool requests and their responses
- **Request/Response Management**: Handle and transform MCP tool requests and responses

## Architecture

The CLI tool is built in Go and provides a unified interface to interact with multiple MCP servers while offering configuration management and extensibility through lifecycle hooks.


## Global Configuration System

  Key Features:
  - Config Location: ~/.centian/config.jsonc
  - Auto-initialization: Creates default config if none exists
  - Server Management: Add, remove, enable/disable MCP servers
  - Lifecycle Hooks: Pre/post request hooks and connection events
  - Validation: Built-in config validation

  Main Components:

  1. GlobalConfig - Root configuration structure with servers, proxy settings, hooks, and metadata
  2. MCPServer - Individual server configurations with command, args, environment variables
  3. ProxySettings - Transport method, logging, timeouts
  4. HookSettings - Lifecycle hooks for request/response interception

  CLI Commands:
  centian config init             # Initialize default config
  centian config show             # Display current config
  centian config validate         # Validate config file
  centian config server list      # List all servers
  centian config server add       # Add new server
  centian config server remove    # Remove server
  centian config server enable    # Enable server
  centian config server disable   # Disable server

