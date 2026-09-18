import { Plugin } from "@opencode/plugin/tui"
import { TextRenderable } from "@opentui/core"
import { appendFileSync } from "node:fs"
import { renderMermaidASCII } from "/Users/david/.config/opencode-v2/diagram-vendor/node_modules/beautiful-mermaid"

const DEBUG = process.env.DIAGRAM_TUI_DEBUG === "1"

function debug(...parts: unknown[]) {
  if (!DEBUG) return
  try {
    const line = `[diagram ${new Date().toISOString()}] ` + parts.map((p) => (typeof p === "string" ? p : JSON.stringify(p))).join(" ") + "\n"
    appendFileSync("/tmp/diagram-tui-debug.log", line)
  } catch {}
}

export default Plugin.define({
  id: "david.diagram",
  setup(context: any) {
    const renderer = context?.renderer
    debug("setup renderer:", !!renderer, "markdown:", typeof context?.markdown?.registerCodeBlockRenderer)
    if (!renderer || typeof context?.markdown?.registerCodeBlockRenderer !== "function") return
    return context.markdown.registerCodeBlockRenderer("mermaid", (token: any, render: any) => {
      try {
        const ascii = renderMermaidASCII(token.text)
        debug("rendered", String(token.text).split("\n")[0], "->", `${ascii.length} chars`)
        return new TextRenderable(renderer, { content: ascii })
      } catch (error) {
        debug("failed:", (error as Error)?.message)
        return render?.defaultRender?.()
      }
    })
  },
})
