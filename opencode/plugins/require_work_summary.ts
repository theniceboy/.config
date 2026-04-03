import type { Plugin } from "@opencode-ai/plugin"

type TurnState = {
	summaryCalled: boolean
}

const states = new Map<string, TurnState>()
const exploratoryTools = new Set([
	"bash",
	"read",
	"glob",
	"grep",
	"task",
	"question",
	"webfetch",
	"web_google_search",
	"web_website_fetch",
	"web_website_search",
	"web_website_outline",
	"web_website_extract_section",
	"web_website_extract_pricing",
])

function sessionState(sessionID: string) {
	let state = states.get(sessionID)
	if (!state) {
		state = { summaryCalled: false }
		states.set(sessionID, state)
	}
	return state
}

function eventSessionID(event: any) {
	return String(
		event?.properties?.sessionID ||
			event?.properties?.session?.id ||
			event?.properties?.info?.sessionID ||
			"",
	).trim()
}

function isExploratoryTool(tool: string) {
	return exploratoryTools.has(tool)
}

export const RequireWorkSummaryPlugin: Plugin = async () => {
	return {
		"chat.message": async (input: any) => {
			const sessionID = String(input?.sessionID || "").trim()
			if (!sessionID) {
				return
			}
			sessionState(sessionID).summaryCalled = false
		},

		"tool.execute.before": async (input: any) => {
			const sessionID = String(input?.sessionID || "").trim()
			if (!sessionID) {
				return
			}

			const tool = String(input?.tool || "").trim()
			if (!tool) {
				return
			}

			const state = sessionState(sessionID)
			if (tool === "set_work_summary") {
				return
			}

			if (isExploratoryTool(tool)) {
				return
			}

			if (!state.summaryCalled) {
				throw new Error(
					"Exploration tools can run first, but call set_work_summary before edits or other committed actions.",
				)
			}
		},

		"tool.execute.after": async (input: any) => {
			const sessionID = String(input?.sessionID || "").trim()
			if (!sessionID) {
				return
			}

			const tool = String(input?.tool || "").trim()
			if (tool === "set_work_summary") {
				sessionState(sessionID).summaryCalled = true
			}
		},

		event: async ({ event }) => {
			if (event?.type === "session.deleted") {
				const sessionID = eventSessionID(event)
				if (sessionID) {
					states.delete(sessionID)
				}
				return
			}

		},
	}
}

export default RequireWorkSummaryPlugin
