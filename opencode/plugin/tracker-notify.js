const NOTIFY_BIN = "/usr/bin/python3";
const NOTIFY_SCRIPT = "/Users/david/.config/codex/notify.py";
const MAX_SUMMARY_CHARS = 600;
const TRACKER_BIN = "/Users/david/.config/agent-tracker/bin/tracker-client";
const finishedSessions = new Set(); // track by sessionID

export const TrackerNotifyPlugin = async ({ client, directory, $ }) => {
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

	const startTask = async (summary) => {
		if (!summary) return;
		if (!(await trackerReady())) return;
		await $`${TRACKER_BIN} command -summary ${summary} start_task`.nothrow();
	};

	const finishTask = async (summary) => {
		if (!(await trackerReady())) return;
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
				// ignore notification failures but still finish
			}
		} catch {
			// Ignore notification failures
		}
	};

	const finishOnce = async (sessionID, summary) => {
		if (!sessionID || finishedSessions.has(sessionID)) return;
		finishedSessions.add(sessionID);
		await finishTask(summary?.slice(0, 200) || "");
	};

	return {
		event: async ({ event }) => {
			// On idle: ensure finish, even if message hook missed
			if (event?.type === "session.idle" && event?.properties?.sessionID) {
				const sessionID = event.properties.sessionID;
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
				await finishOnce(sessionID, text || "done");
				await notify(sessionID);
			}
		},
		"chat.message": async (_input, output) => {
			if (output?.message?.role !== "user") return;
			const summary = summarizeText(output.parts).slice(0, 200);
			await startTask(summary);
		},
		"message.updated": async ({ event }) => {
			if (event?.properties?.info?.role !== "assistant") return;
			const sessionID = event?.properties?.info?.sessionID;
			if (!sessionID) return;

			const parts = event?.properties?.parts || [];
			const text = summarizeText(parts) || "done";

			// Prefer explicit finish flag; otherwise if we see any assistant message update after start, finish once.
			const isFinished =
				event?.properties?.info?.finish ||
				event?.properties?.info?.summary ||
				false;

			if (isFinished || !finishedSessions.has(sessionID)) {
				await finishOnce(sessionID, text);
			}
		},
	};
};
