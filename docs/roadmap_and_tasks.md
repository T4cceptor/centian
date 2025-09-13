# Roadmap & Tasks

The purpose of this document is to give a rough guideline to both contributors and AI assistants as well.


## High Level Roadmap
1. Initial PoC - done, implemented very basic stdio proxy


## Tasks
- Required improvements:
    - replacement of existing MCP configs:
        - move all configs into the centian/config.json
            - central place for configs
        - replace existing configs with centian config
        - centian config applies filter when in a project folder
            - e.g.:
            {
                "command": "centian",
                "args": ["start", "--dir", "path/to/directory/which/can/be/used/for/filtering"]
            }


## Features:
- enable config persistence and loading
    - config should include (for now):
        - MCP servers + their configurations - DONE
        - Available tools + config - TODO: requires us to load the MCP server
        - hash of metadata of all available tools - TODO: requires us to persist MCP tools
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

## Nice-to-have
- automated deduplication, based on config used
- flagging of duplicates with slightly different config
- 
