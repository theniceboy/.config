import { Plugin } from "@opencode/plugin/tui"
import { createEffect, createMemo, createSignal, For, onCleanup, Show } from "solid-js"
import type { BoxRenderable, MouseEvent } from "@opentui/core"
import { execFile } from "node:child_process"
import { homedir } from "node:os"
import { join } from "node:path"

type MemoryFile = {
  store: string
  path: string
  tokens: number
  baselineTokens: number
  updateTokens: number
}
type Usage = {
  status: "ready" | "unchanged" | "error"
  sessionID: string
  fingerprint?: string
  globalEnabled?: boolean
  allMode?: boolean
  totalTokens?: number
  files?: MemoryFile[]
  error?: string
}
const helper = join(homedir(), "base/scripts/memory-usage.py")
const number = (value = 0) => value.toLocaleString("en-US")
const compact = (value = 0) => value < 1000 ? number(value) : `${(value / 1000).toFixed(1)}k`
const fileKey = (file: MemoryFile) => `${file.store}:${file.path}`
const graphemes = new Intl.Segmenter(undefined, { granularity: "grapheme" })

function directories(files: MemoryFile[]) {
  const groups = new Map<string, MemoryFile[]>()
  for (const file of files) {
    const directory = file.path.slice(0, file.path.lastIndexOf("/") + 1)
    const group = groups.get(directory) ?? []
    group.push(file)
    groups.set(directory, group)
  }
  return [...groups].map(([path, files]) => ({ path, files }))
}

function label(file: MemoryFile, width: number) {
  const name = file.path.slice(file.path.lastIndexOf("/") + 1)
  const stem = name.endsWith(".md") ? name.slice(0, -3) : name
  if (Bun.stringWidth(stem) <= width) return stem
  let shortened = ""
  for (const { segment } of graphemes.segment(stem)) {
    if (Bun.stringWidth(shortened + segment) >= width) break
    shortened += segment
  }
  return shortened + "…"
}

export default Plugin.define({
  id: "memory-usage-tui",
  setup(context) {
    const [currentUsage, setUsage] = createSignal<Usage>()
    const usageFor = (sessionID?: string) => {
      const value = currentUsage()
      return value?.sessionID === sessionID ? value : undefined
    }
    const inspect = async (sessionID?: string) => {
      const usage = usageFor(sessionID)
      if (usage?.status !== "ready") {
        await context.ui.dialog.alert({ title: "Loaded memory", message: "Memory counts are not available yet. Please try again in a moment." })
        return
      }
      const files = usage.files ?? []
      if (!files.length) {
        await context.ui.dialog.alert({ title: "Loaded memory", message: "No memory files are loaded in this session yet." })
        return
      }
      const key = await context.ui.dialog.select({ title: `Loaded memory · ≈${number(usage.totalTokens)} tokens`, options: files.map(file => ({
        title: file.path, value: fileKey(file), category: file.store === "global" ? "Global" : "Project",
        description: `≈${number(file.tokens)} tokens`,
      })) })
      if (!key) return
      const file = files.find(file => fileKey(file) === key)
      if (!file) return
      await context.ui.dialog.alert({ title: file.path, message:
        `${file.store === "global" ? "Global memory" : "Project memory"}\n${file.path}\n\n≈${number(file.tokens)} tokens total\n${number(file.baselineTokens)} loaded text · ${number(file.updateTokens)} updates\n\nEstimated with count-tokens.` })
    }
    const activate = (event: MouseEvent, action: () => void) => {
      if (event.button !== 0 || event.isDragging) return
      event.stopPropagation()
      action()
    }
    const stopApp = context.ui.slot({
      append: "app",
      render: () => {
        const sessionID = () => {
          const route = context.ui.router.current()
          return route.type === "session" ? route.sessionID : undefined
        }
        context.keymap.layer(() => ({
          mode: "global",
          commands: [{
            id: "base.memory.files", title: "Memory: inspect loaded files", group: "Memory",
            palette: true, slash: { name: "memory-files" }, enabled: () => !!sessionID(),
            run: () => inspect(sessionID()),
          }],
        }))
        createEffect(() => {
          const id = sessionID()
          setUsage(undefined)
          if (!id) return
          let disposed = false
          let timer: ReturnType<typeof setTimeout> | undefined
          let child: ReturnType<typeof execFile> | undefined
          let fingerprint = ""
          const refresh = () => {
            child = execFile("python3", ["-B", helper, id, "--fingerprint", fingerprint],
              { timeout: 10_000, maxBuffer: 2 * 1024 * 1024 }, (error, stdout) => {
                if (disposed) return
                try {
                  if (error) throw error
                  const result: Usage = JSON.parse(stdout)
                  if (result.sessionID !== id) throw new Error("Session changed")
                  if (result.status !== "unchanged") {
                    fingerprint = result.status === "ready" ? result.fingerprint ?? "" : ""
                    setUsage(result)
                  }
                } catch {
                  fingerprint = ""
                  setUsage({ status: "error", sessionID: id, error: "Memory usage unavailable" })
                }
                timer = setTimeout(refresh, 2000)
              })
          }
          refresh()
          onCleanup(() => {
            disposed = true
            clearTimeout(timer)
            child?.kill()
          })
        })
        return null
      },
    })
    const stopFooter = context.ui.slot({
      prepend: "prompt.footer.status",
      render: props => {
        const usage = () => usageFor(props.sessionID)
        const count = () => usage()?.status === "ready" ? `≈${compact(usage()?.totalTokens)}` : usage()?.status === "error" ? "—" : "…"
        const loaded = () => {
          const value = usage()
          return value?.status === "ready" ? ` (${value.files?.length ?? 0} loaded)` : ""
        }
        const status = () => [...(usage()?.allMode ? ["ALL MEMORY"] : []), `memory ${count()}${loaded()}`].join("   ")
        return <Show when={props.sessionID}>
          <text marginLeft={1} width={Bun.stringWidth(status())} flexShrink={0}
            fg={context.theme.text.subdued} wrapMode="none" selectable={false}
            onMouseUp={(event: MouseEvent) => activate(event, () => { void inspect(props.sessionID) })}>
            <Show when={usage()?.allMode}><span style={{ fg: context.theme.markdown.linkText }}><b>ALL MEMORY</b></span></Show>
            {usage()?.allMode ? "   " : null}
            memory <span style={{ fg: usage()?.totalTokens ? context.theme.markdown.heading : context.theme.text.subdued }}><b>{count()}</b></span>{loaded().length ? <span>{loaded()}</span> : null}
          </text>
        </Show>
      },
    })
    const stopSidebar = context.ui.slot({
      append: "sidebar.content",
      render: (props: { sessionID: string }) => {
        const usage = () => usageFor(props.sessionID)
        const [expanded, setExpanded] = createSignal(true)
        const [selected, setSelected] = createSignal<string>()
        const [hovered, setHovered] = createSignal<string>()
        const [width, setWidth] = createSignal(36)
        let container: BoxRenderable | undefined
        const groups = createMemo(() => ["global", "repo"].map(store => ({
          store, title: store === "global" ? "Global" : "Project",
          directories: directories(usage()?.files?.filter(file => file.store === store) ?? []),
        })).filter(group => group.directories.length))
        const choose = (file: MemoryFile) => setSelected(current => current === fileKey(file) ? undefined : fileKey(file))
        createEffect(() => {
          props.sessionID
          setSelected(undefined)
          setHovered(undefined)
        })
        return <box flexDirection="column" marginTop={1}
          ref={(node: BoxRenderable) => { container = node; setWidth(node.width) }}
          onSizeChange={() => { if (container) setWidth(container.width) }}>
          <text fg={context.theme.markdown.heading} selectable={false}
            onMouseUp={(event: MouseEvent) => activate(event, () => setExpanded(value => !value))}>
            <b>{expanded() ? "▼" : "▶"} Loaded memory</b>
            <Show when={usage()?.allMode}><span style={{ fg: context.theme.markdown.linkText }}><b> · ALL MEMORY</b></span></Show>
          </text>
          <Show when={usage()} fallback={<text fg={context.theme.text.subdued}>Counting loaded files…</text>}>
            <Show when={usage()?.status === "ready"} fallback={<text fg={context.theme.text.subdued}>Counts unavailable · retrying…</text>}>
              <box flexDirection="row" justifyContent="space-between" height={1}>
                <box flexDirection="row">
                  <text fg={context.theme.text.default}><b>≈{number(usage()?.totalTokens)}</b></text>
                  <text fg={context.theme.text.subdued}> tokens</text>
                </box>
                <text fg={context.theme.text.subdued}>{usage()?.files?.length ?? 0} {usage()?.files?.length === 1 ? "file" : "files"}</text>
              </box>
              <Show when={expanded()}>
                <Show when={usage()?.files?.length} fallback={<box marginTop={1} flexDirection="column">
                  <text fg={context.theme.text.subdued}>Files appear when the agent loads memory.</text>
                  <Show when={!usage()?.globalEnabled}><text fg={context.theme.text.subdued}>Global memory is off.</text></Show>
                </box>}>
                  <For each={groups()}>{group => <box flexDirection="column" marginTop={1}>
                    <box flexDirection="row" justifyContent="space-between" height={1}>
                      <text fg={context.theme.text.default}><b>{group.title}</b></text>
                      <text fg={context.theme.text.subdued}>≈tokens</text>
                    </box>
                    <For each={group.directories}>{directory => <box flexDirection="column">
                      <Show when={directory.path}><text fg={context.theme.text.subdued} wrapMode="char">{directory.path}</text></Show>
                      <For each={directory.files}>{file => <box flexDirection="column">
                        <box flexDirection="row" height={1} gap={1} paddingLeft={1}
                          onMouseOver={() => setHovered(fileKey(file))} onMouseOut={() => setHovered(undefined)}
                          onMouseUp={(event: MouseEvent) => activate(event, () => choose(file))}>
                          <text fg={hovered() === fileKey(file) || selected() === fileKey(file) ? context.theme.markdown.linkText : context.theme.text.default}
                            flexGrow={1} flexShrink={1} minWidth={0} wrapMode="none" selectable={false}>
                            <Show when={hovered() === fileKey(file) || selected() === fileKey(file)} fallback={label(file, Math.max(8, width() - 9))}>
                              <b>{label(file, Math.max(8, width() - 9))}</b>
                            </Show>
                          </text>
                          <text fg={context.theme.text.subdued} flexShrink={0} selectable={false}>{compact(file.tokens).padStart(6)}</text>
                        </box>
                        <Show when={selected() === fileKey(file)}>
                          <box flexDirection="column" paddingLeft={1} marginBottom={1}>
                            <text fg={context.theme.text.subdued} wrapMode="char">{file.path}</text>
                            <text fg={context.theme.text.action.primary.default}><b>≈{number(file.tokens)}</b> tokens</text>
                            <Show when={file.updateTokens > 0}><text fg={context.theme.text.subdued}>{number(file.updateTokens)} from updates</text></Show>
                          </box>
                        </Show>
                      </box>}</For>
                    </box>}</For>
                  </box>}</For>
                  <text fg={context.theme.text.subdued} marginTop={1}>Click a file · /memory-files</text>
                </Show>
              </Show>
            </Show>
          </Show>
        </box>
      },
    })
    return () => { stopSidebar(); stopFooter(); stopApp() }
  },
})
