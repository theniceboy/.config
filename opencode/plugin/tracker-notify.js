const NOTIFY_BIN = "/usr/bin/python3";
const NOTIFY_SCRIPT = "/Users/david/.config/codex/notify.py";
const MAX_SUMMARY_CHARS = 600;
const TRACKER_BIN = "/Users/david/.config/agent-tracker/bin/tracker-client";

export const TrackerNotifyPlugin = async ({ client, directory, $ }) => {
	// Only run within tmux (TMUX_PANE must be set)
	const TMUX_PANE = process.env.TMUX_PANE;
	if (!TMUX_PANE) {
		return {};
	}

	// Resolve tmux context once at startup to avoid race conditions
	let tmuxContext = null;
	const resolveTmuxContext = async () => {
		if (tmuxContext) return tmuxContext;
		try {
			const result = await $`tmux display-message -p -t ${TMUX_PANE} "#{session_id}:::#{window_id}:::#{pane_id}"`.quiet();
			const parts = result.stdout.trim().split(":::");
			if (parts.length === 3) {
				tmuxContext = {
					sessionId: parts[0],
					windowId: parts[1],
					paneId: parts[2],
				};
			}
		} catch {
			// Fallback: use TMUX_PANE directly
			tmuxContext = { paneId: TMUX_PANE };
		}
		return tmuxContext;
	};
	await resolveTmuxContext();

	let taskActive = false;
	let currentSessionID = null;
	let lastUserMessage = "";

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

	// On init: finish any stale task for this pane
	const finishStaleTask = async () => {
		if (!(await trackerReady())) return;
		const args = buildTrackerArgs();
		await $`${TRACKER_BIN} command ${args} -summary "stale" finish_task`.nothrow();
	};
	finishStaleTask();

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
		event: async ({ event }) => {
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

			if (event?.type !== "session.status") return;

			const sessionID = event?.properties?.sessionID;
			const status = event?.properties?.status;
			if (!sessionID || !status) return;

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
