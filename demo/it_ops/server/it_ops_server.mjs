import { stdin, stdout } from "node:process";

const getDocsDescription = "Return simple documentation placeholder content."
const getDocsResponse = "Hello from IT ops docs. Here will be the docs."
const getTicketDescription = "Return the next IT ticket."
const getTicketResponse = [
  `Ticket ${args.id ?? "IT-1042"}`,
  "Title: Checkout service returns intermittent 503 responses",
  "Priority: P2",
  "Status: investigating",
  "Owner: platform-oncall",
].join("\n")
const getLogsDescription = "Return Kubernetes service logs."
let getLogsResponseFn = (args) => {
  const service = args.service ?? "checkout-api";
      const namespace = args.namespace ?? "prod";
      return {
        content: [
          {
            type: "text",
            text: [
              `2026-05-20T10:14:03Z namespace=${namespace} service=${service} level=info msg="handled request" status=200 latency_ms=42`,
              `2026-05-20T10:14:09Z namespace=${namespace} service=${service} level=warn msg="upstream retry" upstream=inventory-api attempt=2`,
              `2026-05-20T10:14:15Z namespace=${namespace} service=${service} level=error msg="request failed" status=503 reason="upstream timeout"`,
            ].join("\n"),
          },
        ],
      };
}
const kubectlDescription = "Accept a fake kubectl command."
const kubectlResponse = {
  content: [
    {
      type: "text",
      text: `Command accepted: kubectl ${args.command ?? ""}`.trim(),
    },
  ],
}

const TOOLS = {
  get_docs: {
    description: getDocsDescription,
    inputSchema: {
      type: "object",
      properties: {},
      additionalProperties: false,
    },
    handler: () => ({
      content: [
        {
          type: "text",
          text: getDocsResponse,
        },
      ],
    }),
  },
  get_ticket: {
    description: getTicketDescription,
    inputSchema: {
      type: "object",
      properties: {
        id: {
          type: "string",
          description: "Optional ticket id.",
        },
      },
      additionalProperties: false,
    },
    handler: (args = {}) => ({
      content: [
        {
          type: "text",
          text: getTicketResponse,
        },
      ],
    }),
  },
  get_logs: {
    description: getLogsDescription,
    inputSchema: {
      type: "object",
      properties: {
        service: {
          type: "string",
          description: "Optional Kubernetes service name.",
        },
        namespace: {
          type: "string",
          description: "Optional Kubernetes namespace.",
        },
      },
      additionalProperties: false,
    },
    handler: (args = {}) => {
      return getLogsResponseFn(args)
    },
  },
  kubectl: {
    description: kubectlDescription,
    inputSchema: {
      type: "object",
      properties: {
        command: {
          type: "string",
          description: "kubectl command text.",
        },
      },
      required: ["command"],
      additionalProperties: false,
    },
    handler: (args = {}) => (kubectlResponse),
  },
};

function write(message) {
  stdout.write(`${JSON.stringify(message)}\n`);
}

function respond(id, result) {
  write({ jsonrpc: "2.0", id, result });
}

function respondError(id, code, message) {
  write({ jsonrpc: "2.0", id, error: { code, message } });
}

function listTools() {
  return Object.entries(TOOLS).map(([name, tool]) => ({
    name,
    description: tool.description,
    inputSchema: tool.inputSchema,
  }));
}

function handleRequest(message) {
  const { id, method, params } = message;

  if (method === "initialize") {
    respond(id, {
      protocolVersion: params?.protocolVersion ?? "2024-11-05",
      capabilities: { tools: {} },
      serverInfo: {
        name: "it-ops-mock",
        version: "0.1.0",
      },
    });
    return;
  }

  if (method === "notifications/initialized") {
    return;
  }

  if (method === "tools/list") {
    respond(id, { tools: listTools() });
    return;
  }

  if (method === "tools/call") {
    const tool = TOOLS[params?.name];
    if (!tool) {
      respondError(id, -32601, `Unknown tool: ${params?.name ?? "<missing>"}`);
      return;
    }

    respond(id, tool.handler(params?.arguments ?? {}));
    return;
  }

  if (id !== undefined) {
    respondError(id, -32601, `Method not found: ${method}`);
  }
}

let buffer = "";
stdin.setEncoding("utf8");
stdin.on("data", (chunk) => {
  buffer += chunk;

  let newlineIndex = buffer.indexOf("\n");
  while (newlineIndex >= 0) {
    const line = buffer.slice(0, newlineIndex).trim();
    buffer = buffer.slice(newlineIndex + 1);

    if (line) {
      try {
        handleRequest(JSON.parse(line));
      } catch {
        respondError(null, -32700, "Parse error");
      }
    }

    newlineIndex = buffer.indexOf("\n");
  }
});
