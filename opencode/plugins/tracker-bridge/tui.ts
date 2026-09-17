import { Plugin } from "@opencode/plugin/tui"
import { appendFileSync, mkdirSync, renameSync, writeFileSync } from "fs"

const STATE_ROOT = process.env.XDG_STATE_HOME || `${process.env.HOME || ""}/.local/state`
const OP_STATE_DIR = `${STATE_ROOT}/op`
const LOG_FILE = "/tmp/tracker-bridge-tui-debug.log"
const POLL_MS = 400
const HEARTBEAT_MS = 8_000

const log = (msg: string, data?: any) => {
  try {
    appendFileSync(LOG_FILE, `[${new Date().toISOString()}] ${msg}${data ? " " + JSON.stringify(data) : ""}\n`)
  } catch {}
}

const sanitizeKey = (v = "") => v.replace(/[^A-Za-z0-9_]/g, "_")
const sesFile = (sid: string) => `${OP_STATE_DIR}/ses_${sanitizeKey(sid)}`

// Runs inside the TUI process, which lives in the tmux pane: it registers the
// pane for the TUI's current session so the shared server's tracker plugin can
// attribute events to this pane. Pane ids never change, so only the pane id is
// stored; readers resolve all other tmux fields live.
export default Plugin.define({
  id: "tracker.bridge.tui",
  setup(ctx) {
    const paneId = process.env.TMUX_PANE
    if (!paneId || !process.env.TMUX) {
      log("not in tmux, disabled")
      return
    }

    const pane = { paneId }
    const directory = (ctx as any).location?.directory || process.cwd()

    let currentSession = ""
    let lastWrite = 0
    const written = new Set<string>()

    const writeRegistration = (sessionID: string, closing = false) => {
      if (!sessionID) return
      try {
        mkdirSync(OP_STATE_DIR, { recursive: true })
        const reg: any = {
          v: 1,
          sessionID,
          directory,
          pane,
          pid: process.pid,
          heartbeat: Date.now(),
        }
        if (closing) reg.closing = true
        const file = sesFile(sessionID)
        const tmp = `${file}.tmp`
        writeFileSync(tmp, JSON.stringify(reg), "utf8")
        renameSync(tmp, file)
        if (!closing) {
          written.add(sessionID)
          lastWrite = Date.now()
        }
      } catch (e) {
        log("write failed", { error: String(e) })
      }
    }

    const tick = () => {
      let route: any = null
      try {
        route = ctx.ui.router.current()
      } catch {
        return
      }
      const sid = route?.type === "session" ? (route.sessionID as string) : ""
      if (!sid) return
      if (sid !== currentSession) {
        currentSession = sid
        log("session changed", { sid })
        writeRegistration(sid)
      } else if (Date.now() - lastWrite > HEARTBEAT_MS) {
        writeRegistration(sid)
      }
    }
    const timer = setInterval(tick, POLL_MS)
    tick()

    // Close the race for freshly created sessions: the server re-checks
    // registration shortly after execution starts, so re-register now.
    try {
      ctx.data.on("session.execution.started" as any, (ev: any) => {
        const sid = (ev as any)?.data?.sessionID ?? ""
        if (sid && sid === currentSession) writeRegistration(sid)
      })
    } catch {}

    return () => {
      clearInterval(timer)
      for (const sid of written) writeRegistration(sid, true)
      log("cleanup", { written: [...written] })
    }
  },
})
