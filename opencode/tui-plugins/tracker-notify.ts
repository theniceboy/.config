import { appendFileSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "fs";

const NOTIFY_BIN = "/usr/bin/python3";
const NOTIFY_SCRIPT = "/Users/david/.config/codex/notify.py";
const MAX_SUMMARY_CHARS = 600;
const TRACKER_BIN = "/Users/david/.config/agent-tracker/bin/tracker-client";
const LOG_FILE = "/tmp/tracker-notify-debug.log";
const STATE_ROOT = process.env.XDG_STATE_HOME || `${process.env.HOME || ""}/.local/state`;
const OP_STATE_DIR = `${STATE_ROOT}/op`;
const TMUX_BINS = [process.env.TMUX_BIN, "/opt/homebrew/bin/tmux", "tmux"].filter(Boolean);
const TMUX_QUESTION_OPTION = "@op_question_pending";

const log = (msg: string, data?: any) => {
	const timestamp = new Date().toISOString();
	const logMsg = `[${timestamp}] ${msg}${data ? " " + JSON.stringify(data) : ""}\n`;
	try {
		appendFileSync(LOG_FILE, logMsg);
	} catch (e) {
		// ignore
	}
};

export const TrackerNotifyPlugin = async ({ client, directory, $ }) => {
	const trackerNotifyEnabled = process.env.OP_TRACKER_NOTIFY === "1";
	if (!trackerNotifyEnabled) {
		return {};
	}

	// Only run within tmux (TMUX_PANE must be set)
	const TMUX_PANE = process.env.TMUX_PANE;
	log("Plugin loading, TMUX_PANE:", TMUX_PANE);
	if (!TMUX_PANE) {
		log("Not in tmux, plugin disabled");
		return {};
	}

	// Resolve tmux context once at startup to avoid race conditions
	let tmuxContext = null;
	const resolveTmuxContext = async () => {
		if (tmuxContext) return tmuxContext;
		for (const tmuxBin of TMUX_BINS) {
			try {
				const output = await $`${tmuxBin} display-message -p -t ${TMUX_PANE} "#{session_id}:::#{window_id}:::#{pane_id}:::#{session_name}:::#{window_index}:::#{pane_index}"`.text();
				const parts = output.trim().split(":::");
				if (parts.length === 6) {
					tmuxContext = {
						sessionId: parts[0],
						windowId: parts[1],
						paneId: parts[2],
						sessionName: parts[3],
						windowIndex: parts[4],
						paneIndex: parts[5],
					};
					break;
				}
			} catch {
				// continue
			}
		}
		if (!tmuxContext) {
			tmuxContext = { paneId: TMUX_PANE };
		}
		return tmuxContext;
	};
	await resolveTmuxContext();

	const sanitizeKey = (value = "") => value.replace(/[^A-Za-z0-9_]/g, "_");
	const paneLocator = () => {
		if (!tmuxContext?.sessionName || !tmuxContext?.windowIndex || !tmuxContext?.paneIndex) {
			return "";
		}
		return `${tmuxContext.sessionName}:${tmuxContext.windowIndex}.${tmuxContext.paneIndex}`;
	};
	const paneSessionStateFile = () => {
		const locator = paneLocator();
		if (!locator) return "";
		return `${OP_STATE_DIR}/loc_${sanitizeKey(locator)}`;
	};
	const persistPaneSessionMap = async (sessionID) => {
		const stateFile = paneSessionStateFile();
		if (!sessionID || !stateFile) return;
		try {
			mkdirSync(OP_STATE_DIR, { recursive: true });
			const tmpPath = `${stateFile}.tmp`;
			writeFileSync(tmpPath, `${sessionID}\n`, "utf8");
			renameSync(tmpPath, stateFile);
		} catch (error) {
			log("failed to persist pane session", { stateFile, error: String(error) });
		}
	};
	const loadPersistedPaneSessionID = () => {
		const stateFile = paneSessionStateFile();
		if (!stateFile) return "";
		try {
			return readFileSync(stateFile, "utf8").trim();
		} catch {
			return "";
		}
	};
	const eventSessionID = (event) => {
		return (
			event?.properties?.sessionID ||
			event?.properties?.session?.id ||
			event?.properties?.info?.id ||
			""
		);
	};

	let taskActive = false;
	let currentSessionID = null;
	let lastUserMessage = "";
	let rootSessionID = loadPersistedPaneSessionID();
	let questionPending: boolean | null = null;

	const setTmuxPaneOption = async (option, value: string | null) => {
		if (!tmuxContext?.paneId) return;
		for (const tmuxBin of TMUX_BINS) {
			const cmd =
				value === null
					? await $`${tmuxBin} set-option -p -u -t ${tmuxContext.paneId} ${option}`.nothrow()
					: await $`${tmuxBin} set-option -p -t ${tmuxContext.paneId} ${option} ${value}`.nothrow();
			if (cmd?.exitCode === 0) {
				return;
			}
		}
	};

	const applyQuestionPending = async (pending: boolean) => {
		if (questionPending === pending) return;
		questionPending = pending;
		await setTmuxPaneOption(TMUX_QUESTION_OPTION, pending ? "1" : null);
	};

	const listPendingQuestions = async () => {
		try {
			const response = await client.question.list({ directory });
			if (Array.isArray(response?.data)) {
				return response.data;
			}
			if (Array.isArray(response)) {
				return response;
			}
		} catch {
			// ignore
		}
		return [];
	};

	const syncPendingQuestionState = async (sessionID = rootSessionID) => {
		const effectiveSessionID = sessionID || loadPersistedPaneSessionID();
		if (!effectiveSessionID) {
			return;
		}
		rootSessionID = effectiveSessionID;
		const pending = (await listPendingQuestions()).some(
			(question) => question?.sessionID === effectiveSessionID,
		);
		await applyQuestionPending(pending);
	};

	if (rootSessionID) {
		await syncPendingQuestionState(rootSessionID);
	} else {
		await applyQuestionPending(false);
	}

	const trackerReady = async () => {
		const check = await $`test -x ${TRACKER_BIN}`.nothrow();
		return check?.exitCode === 0;
	};

	const buildTrackerArgs = () => {
		const args = [];
		if (tmuxContext?.sessionId) args.push("-session-id", tmuxContext.sessionId);
		if (tmuxContext?.windowId) args.push("-window-id", tmuxContext.windowId);
		if (tmuxContext?.paneId) args.push("-pane", tmuxContext.paneId);
		return args;
	};

	const summarizeText = (parts = []) => {
		const text = parts
			.filter((p) => p?.type === "text" && !p.ignored)
			.map((p) => p.text || "")
			.join("\n")
			.trim();
		return text.slice(0, MAX_SUMMARY_CHARS);
	};

	const collectUserInputs = (messages) => {
		return messages
			.filter((m) => m?.info?.role === "user")
			.slice(-3)
			.map((m) => summarizeText(m.parts))
			.filter((text) => text);
	};

	const startTask = async (summary, sessionID) => {
		if (!summary) return;
		if (!(await trackerReady())) return;
		taskActive = true;
		currentSessionID = sessionID;
		const args = buildTrackerArgs();
		await $`${TRACKER_BIN} command ${args} -summary ${summary} start_task`.nothrow();
	};

	const finishTask = async (summary) => {
		if (!taskActive) return;
		if (!(await trackerReady())) return;
		taskActive = false;
		currentSessionID = null;
		const args = buildTrackerArgs();
		await $`${TRACKER_BIN} command ${args} -summary ${summary || "done"} finish_task`.nothrow();
	};

	const notify = async (sessionID) => {
		try {
			const messages =
				(await client.session.messages({
					path: { id: sessionID },
					query: { directory },
				})) || [];

			const assistant = [...messages]
				.reverse()
				.find((m) => m?.info?.role === "assistant");
			if (!assistant) return;

			const assistantText = summarizeText(assistant.parts);
			if (!assistantText) return;

			const payload = {
				type: "agent-turn-complete",
				"last-assistant-message": assistantText,
				input_messages: collectUserInputs(messages),
			};

			const serialized = JSON.stringify(payload);
			try {
				await $`${NOTIFY_BIN} ${NOTIFY_SCRIPT} ${serialized}`;
			} catch {
				// ignore notification failures
			}
		} catch {
			// Ignore notification failures
		}
	};

	const getLastMessageText = async (sessionID, role, retries = 3) => {
		for (let attempt = 0; attempt < retries; attempt++) {
			try {
				const messages =
					(await client.session.messages({
						path: { id: sessionID },
						query: { directory },
					})) || [];
				const msg = [...messages]
					.reverse()
					.find((m) => m?.info?.role === role);
				if (msg) {
					const text = summarizeText(msg.parts);
					if (text) return text;
				}
			} catch {
				// ignore fetch errors
			}
			if (attempt < retries - 1) {
				await new Promise((r) => setTimeout(r, 100));
			}
		}
		return "";
	};

	// Track message IDs to their roles
	const messageRoles = new Map();

	return {
		"tool.execute.before": async (input, output) => {
			if (input.tool === "question") {
				if (!rootSessionID && input?.sessionID) {
					rootSessionID = input.sessionID;
				}
				await applyQuestionPending(true);
				log("Question tool called:", {
					questions: output.args?.questions || "no questions",
					timestamp: new Date().toISOString()
				});
			}
		},
		
		event: async ({ event }) => {
			if (event?.type === "question.asked") {
				await applyQuestionPending(true);
				return;
			}

			if (event?.type === "question.replied" || event?.type === "question.rejected") {
				const sessionID = event?.properties?.sessionID || rootSessionID;
				await syncPendingQuestionState(sessionID);
				return;
			}
			
			// Track message roles from message.updated events
			if (event?.type === "message.updated") {
				const info = event?.properties?.info;
				if (info?.id && info?.role) {
					messageRoles.set(info.id, info.role);
				}
			}

			// Capture user message text from message.part.updated
			if (event?.type === "message.part.updated") {
				const part = event?.properties?.part;
				if (part?.type === "text" && part?.text && part?.messageID) {
					const role = messageRoles.get(part.messageID);
					// Capture if it's a user message, or if we're not yet in a task (user input comes first)
					if (role === "user" || (!role && !taskActive)) {
						const text = part.text?.trim();
						if (text && text.length > 0) {
							lastUserMessage = text.slice(0, MAX_SUMMARY_CHARS);
						}
					}
				}
			}

			if (event?.type !== "session.updated" && event?.type !== "session.status") return;

			const sessionID = eventSessionID(event);
			if (!sessionID) return;

			// Check if this is a subagent session
			const session = await client.session.get({ path: { id: sessionID } }).catch(() => null);
			const parentID = session?.data?.parentID;

			// Skip subagent sessions (they have a parentID)
			if (parentID) {
				return;
			}

			const sessionChanged = sessionID !== rootSessionID;
			rootSessionID = sessionID;
			await persistPaneSessionMap(sessionID);
			if (sessionChanged) {
				await syncPendingQuestionState(sessionID);
			}
			if (event?.type !== "session.status") return;

			const status = event?.properties?.status;
			if (!status) return;

			if (status.type === "busy" && !taskActive) {
				// Use captured message first, then fall back to API
				let text = lastUserMessage;
				if (!text) {
					text = await getLastMessageText(sessionID, "user");
				}
				await startTask(text || "working...", sessionID);
				lastUserMessage = "";
			} else if (status.type === "idle" && taskActive) {
				if (currentSessionID && sessionID !== currentSessionID) return;
				const text = await getLastMessageText(sessionID, "assistant");
				await finishTask(text || "done");
				await notify(sessionID);
			}
		},
	};
};
