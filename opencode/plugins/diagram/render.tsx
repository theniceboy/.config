import { Plugin } from "@opencode/plugin/tui"
import { StyledText, TextRenderable, RGBA } from "@opentui/core"
import { appendFileSync, readFileSync } from "node:fs"
import { renderMermaidASCII } from "/Users/david/.config/opencode-v2/diagram-vendor/node_modules/beautiful-mermaid"

const DEBUG = process.env.DIAGRAM_TUI_DEBUG === "1"

const SETTINGS_PATH = "/Users/david/.config/opencode/plugins/diagram/settings.json"
const DEFAULT_SETTINGS = { paddingX: 4, paddingY: 2, boxBorderPadding: 0 }

function loadSettings() {
  try {
    return { ...DEFAULT_SETTINGS, ...JSON.parse(readFileSync(SETTINGS_PATH, "utf8")) }
  } catch {
    return { ...DEFAULT_SETTINGS }
  }
}

function debug(...parts: unknown[]) {
  if (!DEBUG) return
  try {
    const line = `[diagram ${new Date().toISOString()}] ` + parts.map((p) => (typeof p === "string" ? p : JSON.stringify(p))).join(" ") + "\n"
    appendFileSync("/tmp/diagram-tui-debug.log", line)
  } catch {}
}

function chunk(text: string, fg?: any) {
  return { __isChunk: true as const, text, ...(fg ? { fg } : {}) }
}

function ansiToStyledText(ansi: string): StyledText {
  const chunks: any[] = []
  let fg: any
  let pos = 0
  const re = /\x1b\[([0-9;]*)m/g
  let match: RegExpExecArray | null
  while ((match = re.exec(ansi))) {
    if (match.index > pos) chunks.push(chunk(ansi.slice(pos, match.index), fg))
    const params = match[1]
    if (params === "0" || params === "") fg = undefined
    else {
      const parts = params.split(";").map(Number)
      if (parts[0] === 38 && parts[1] === 2 && parts.length >= 5) fg = RGBA.fromInts(parts[2], parts[3], parts[4])
      else if (parts[0] === 39) fg = undefined
    }
    pos = re.lastIndex
  }
  if (pos < ansi.length) chunks.push(chunk(ansi.slice(pos), fg))
  return new StyledText(chunks)
}

function cssColor(c: any): string | undefined {
  if (!c) return undefined
  if (typeof c === "string") return c
  try {
    const [r, g, b] = c.toInts()
    return "#" + [r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")
  } catch {
    return undefined
  }
}

export default Plugin.define({
  id: "david.diagram",
  setup(context: any) {
    const renderer = context?.renderer
    debug("setup renderer:", !!renderer, "markdown:", typeof context?.markdown?.registerCodeBlockRenderer)
    if (!renderer || typeof context?.markdown?.registerCodeBlockRenderer !== "function") return
    return context.markdown.registerCodeBlockRenderer("mermaid", (token: any, render: any) => {
      try {
        const t = context.theme
        const theme = {
          fg: cssColor(t?.text?.default),
          border: cssColor(t?.text?.subdued),
          line: cssColor(t?.text?.subdued),
          arrow: cssColor(t?.text?.default),
        }
        const ascii = renderMermaidASCII(token.text, { colorMode: "truecolor", theme, ...loadSettings() })
        debug("rendered", String(token.text).split("\n")[0], "->", `${ascii.length} chars`)
        return new TextRenderable(renderer, { content: ansiToStyledText(ascii) })
      } catch (error) {
        debug("failed:", (error as Error)?.message)
        return render?.defaultRender?.()
      }
    })
  },
})
