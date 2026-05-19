import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js"
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js"
import { z } from "zod"

const baseUrl = (process.env.PINCHTAB_URL || "http://127.0.0.1:9867").replace(/\/+$/, "")
const token = process.env.PINCHTAB_TOKEN || ""

const server = new McpServer({
  name: "pinchtab",
  version: "0.1.0",
})

function toolResult(data) {
  const text = typeof data === "string" ? data : JSON.stringify(data, null, 2)
  return {
    content: [{ type: "text", text }],
  }
}

function toolError(error) {
  const text = error instanceof Error ? error.message : String(error)
  return {
    content: [{ type: "text", text }],
    isError: true,
  }
}

function withDefinedValues(input) {
  return Object.fromEntries(Object.entries(input).filter(([, value]) => value !== undefined))
}

async function requestPinchtab(path, { method = "GET", query, body } = {}) {
  const url = new URL(`${baseUrl}${path}`)
  for (const [key, value] of Object.entries(query || {})) {
    if (value !== undefined) {
      url.searchParams.set(key, String(value))
    }
  }

  const headers = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json"
  }

  const response = await fetch(url, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  const contentType = response.headers.get("content-type") || ""
  const text = await response.text()

  let data = text
  if (contentType.includes("application/json")) {
    try {
      data = text ? JSON.parse(text) : {}
    } catch {
      data = text
    }
  }

  if (!response.ok) {
    const message = typeof data === "string" ? data : JSON.stringify(data)
    throw new Error(`${response.status} ${response.statusText}: ${message}`)
  }

  return data
}

server.registerTool(
  "pinchtab_health",
  {
    description: "Check Pinchtab server status",
  },
  async () => {
    try {
      const result = await requestPinchtab("/health")
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_tabs",
  {
    description: "List open browser tabs",
  },
  async () => {
    try {
      const result = await requestPinchtab("/tabs")
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_navigate",
  {
    description: "Navigate a tab to a URL",
    inputSchema: {
      url: z.string().url(),
      tabId: z.string().optional(),
      newTab: z.boolean().optional(),
    },
  },
  async ({ url, tabId, newTab }) => {
    try {
      const result = await requestPinchtab("/navigate", {
        method: "POST",
        body: withDefinedValues({ url, tabId, newTab }),
      })
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_snapshot",
  {
    description: "Get accessibility snapshot (JSON or text)",
    inputSchema: {
      tabId: z.string().optional(),
      filter: z.enum(["interactive"]).optional(),
      diff: z.boolean().optional(),
      depth: z.number().int().optional(),
      format: z.enum(["json", "text"]).optional(),
    },
  },
  async ({ tabId, filter, diff, depth, format }) => {
    try {
      const result = await requestPinchtab("/snapshot", {
        query: withDefinedValues({
          tabId,
          filter,
          diff: diff ? "true" : undefined,
          depth,
          format: format === "text" ? "text" : undefined,
        }),
      })
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_text",
  {
    description: "Extract readable text from page",
    inputSchema: {
      tabId: z.string().optional(),
      mode: z.enum(["clean", "raw"]).optional(),
    },
  },
  async ({ tabId, mode }) => {
    try {
      const result = await requestPinchtab("/text", {
        query: withDefinedValues({
          tabId,
          mode: mode === "raw" ? "raw" : undefined,
        }),
      })
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_action",
  {
    description: "Run a single browser action",
    inputSchema: {
      tabId: z.string().optional(),
      kind: z.enum(["click", "type", "fill", "press", "focus", "hover", "select", "scroll"]),
      ref: z.string().optional(),
      selector: z.string().optional(),
      text: z.string().optional(),
      key: z.string().optional(),
      value: z.string().optional(),
      nodeId: z.number().int().optional(),
      scrollX: z.number().int().optional(),
      scrollY: z.number().int().optional(),
      waitNav: z.boolean().optional(),
    },
  },
  async (args) => {
    try {
      const result = await requestPinchtab("/action", {
        method: "POST",
        body: withDefinedValues(args),
      })
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_evaluate",
  {
    description: "Evaluate JavaScript expression in page context",
    inputSchema: {
      expression: z.string().min(1),
      tabId: z.string().optional(),
    },
  },
  async ({ expression, tabId }) => {
    try {
      const result = await requestPinchtab("/evaluate", {
        method: "POST",
        body: withDefinedValues({ expression, tabId }),
      })
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_tab",
  {
    description: "Open or close tabs",
    inputSchema: {
      action: z.enum(["new", "close"]),
      tabId: z.string().optional(),
      url: z.string().url().optional(),
    },
  },
  async ({ action, tabId, url }) => {
    try {
      if (action === "close" && !tabId) {
        return toolError("tabId is required when action is close")
      }

      const result = await requestPinchtab("/tab", {
        method: "POST",
        body: withDefinedValues({ action, tabId, url }),
      })
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

server.registerTool(
  "pinchtab_screenshot",
  {
    description: "Capture JPEG screenshot as base64",
    inputSchema: {
      tabId: z.string().optional(),
      quality: z.number().int().min(1).max(100).optional(),
    },
  },
  async ({ tabId, quality }) => {
    try {
      const result = await requestPinchtab("/screenshot", {
        query: withDefinedValues({ tabId, quality }),
      })
      return toolResult(result)
    } catch (error) {
      return toolError(error)
    }
  },
)

const transport = new StdioServerTransport()
await server.connect(transport)
