const NOTIFY_BIN = "/usr/bin/python3";
const NOTIFY_SCRIPT = "/Users/david/.config/codex/notify.py";
const MAX_SUMMARY_CHARS = 600;
const TRACKER_BIN = "/Users/david/.config/agent-tracker/bin/tracker-client";

export const TrackerNotifyPlugin = async ({ client, directory, $ }) => {
	let taskActive = false;
	let currentSessionID = null;

	const trackerReady = async () => {
		const check = await $`test -x ${TRACKER_BIN}`.nothrow();
		return check?.exitCode === 0;
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
		await $`${TRACKER_BIN} command -summary ${summary} start_task`.nothrow();
	};

	const finishTask = async (summary) => {
		if (!taskActive) return;
		if (!(await trackerReady())) return;
		taskActive = false;
		currentSessionID = null;
		await $`${TRACKER_BIN} command -summary ${summary || "done"} finish_task`.nothrow();
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

	const isSessionTrulyIdle = async (sessionID) => {
		try {
			const messages =
				(await client.session.messages({
					path: { id: sessionID },
					query: { directory },
				})) || [];
			if (!messages.length) return true;

			const last = messages[messages.length - 1];
			if (!last?.info) return true;

			// If last message is from user, assistant hasn't responded yet
			if (last.info.role === "user") return false;

			// If last message is assistant, check if it has completed
			return !!last.info.time?.completed;
		} catch {
			return true;
		}
	};

	const tryFinishTask = async (sessionID) => {
		if (!taskActive) return;
		// Only finish for the session that started the task
		if (currentSessionID && sessionID !== currentSessionID) return;

		// Verify session is truly idle (assistant message completed)
		if (!(await isSessionTrulyIdle(sessionID))) return;

		let text = "";
		try {
			const messages =
				(await client.session.messages({
					path: { id: sessionID },
					query: { directory },
				})) || [];
			const assistant = [...messages]
				.reverse()
				.find((m) => m?.info?.role === "assistant");
			if (assistant) text = summarizeText(assistant.parts);
		} catch {
			// ignore fetch errors
		}
		await finishTask(text || "done");
		await notify(sessionID);
	};

	return {
		event: async ({ event }) => {
			// session.idle event - verify with message state
			if (event?.type === "session.idle" && event?.properties?.sessionID) {
				await tryFinishTask(event.properties.sessionID);
				return;
			}

			// session.status event - "idle" status is more reliable
			if (event?.type === "session.status") {
				const sessionID = event?.properties?.sessionID;
				const status = event?.properties?.status;
				if (sessionID && status === "idle") {
					await tryFinishTask(sessionID);
				}
				return;
			}
		},
		"chat.message": async (_input, output) => {
			if (output?.message?.role !== "user") return;
			const sessionID = output?.message?.sessionID;
			const summary = summarizeText(output.parts).slice(0, 200);
			await startTask(summary, sessionID);
		},
		"message.updated": async ({ event }) => {
			// When an assistant message is updated with time.completed, the turn is done
			if (event?.properties?.info?.role !== "assistant") return;
			if (!event?.properties?.info?.time?.completed) return;

			const sessionID = event?.properties?.info?.sessionID;
			if (!sessionID) return;

			await tryFinishTask(sessionID);
		},
	};
};
