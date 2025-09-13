# Roadmap & Tasks

The purpose of this document is to give a rough guideline to both contributors and AI assistants as well.


## High Level Roadmap
1. Initial PoC - done, implemented very basic stdio proxy


## Tasks


## Features:
- enable config persistence and loading
    - config should include (for now):
        - MCP servers + their configurations
        - Available tools + config
        - hash of metadata of all available tools
            - if this changes we want to flag it

- enable http
- enable on request + on response hooks
    - this requires configuration of the hooks, and the MCP tools where they should be applied
    - Idea: we allow configuration of appliance on the hook level, meaning we will have something like:
        {
            "name": "Log everything",
            "type": "log",
            "on": "request | response | both (default)"
            "config: {
                "target": "https://myawesomeotelserver.com"
            },
            "applied_at": {
                "include": <regex for which servers/tools this is to be included>,
                "exclude": <regex for which servers/tools this is to be excluded>
            }
            // This might be too simple, because this can be quite complex
            // Is the full request covered in the MCP specs? then I can actually provide exactly this as a JSON
            // For the response its the same
        }