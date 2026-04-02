import { tool } from "@opencode-ai/plugin"

const MAX_THEME_CHARS = 48
const MAX_NOW_CHARS = 48
const TMUX_THEME_OPTION = "@op_work_theme"
const TMUX_NOW_OPTION = "@op_work_now"
const TMUX_LEGACY_OPTION = "@op_work_summary"
const TRACKER_BIN = `${process.env.HOME || ""}/.config/agent-tracker/bin/agent`
const GENERIC_LABELS = new Set([
	"working",
	"coding",
	"debugging",
	"researching",
	"thinking",
	"fixing",
	"checking",
	"investigating",
	"task",
	"stuff",
	"project",
	"feature",
	"issue",
	"bug",
])

function capitalizeFirstLetter(value: string): string {
	return value.replace(/[A-Za-z]/, (letter) => letter.toUpperCase())
}

function normalizeLabel(value: string, maxChars: number): string {
	const collapsed = value
		.replace(/\s+/g, " ")
		.replace(/[.!,;:]+$/g, "")
		.trim()
	if (collapsed.length <= maxChars) {
		return capitalizeFirstLetter(collapsed)
	}

	const words = collapsed.split(" ")
	let clipped = ""
	for (const word of words) {
		const next = clipped ? `${clipped} ${word}` : word
		if (next.length > maxChars) {
			break
		}
		clipped = next
	}
	if (clipped) {
		return capitalizeFirstLetter(clipped)
	}

	return capitalizeFirstLetter(collapsed.slice(0, maxChars).trim())
}

function ensureSpecific(label: string, field: string): string {
	if (!label) {
		return ""
	}

	if (GENERIC_LABELS.has(label.toLowerCase())) {
		throw new Error(
			`Use a more specific ${field}. Good examples: \"Tmux status summaries\", \"Patch work-context layout\", \"Wait for user reply\".`,
		)
	}

	return label
}

async function runTmux(tmuxBin: string, args: string[]) {
	const proc = Bun.spawn([tmuxBin, ...args], {
		stdin: "ignore",
		stdout: "pipe",
		stderr: "pipe",
	})
	const [stdout, stderr, exitCode] = await Promise.all([
		new Response(proc.stdout).text(),
		new Response(proc.stderr).text(),
		proc.exited,
	])
	if (exitCode !== 0) {
		const message = stderr.trim() || stdout.trim() || `tmux exited ${exitCode}`
		throw new Error(message)
	}
	return stdout.trim()
}

async function readTmuxOption(tmuxBin: string, tmuxPane: string, option: string) {
	return runTmux(tmuxBin, ["display-message", "-p", "-t", tmuxPane, `#{${option}}`]).catch(() => "")
}

async function writeTmuxOption(tmuxBin: string, tmuxPane: string, option: string, value: string) {
	if (!value) {
		await runTmux(tmuxBin, ["set-option", "-p", "-u", "-t", tmuxPane, option])
		return
	}

	await runTmux(tmuxBin, ["set-option", "-p", "-t", tmuxPane, option, value])
}

async function runCommand(args: string[]) {
	const proc = Bun.spawn(args, {
		stdin: "ignore",
		stdout: "ignore",
		stderr: "pipe",
	})
	const [stderr, exitCode] = await Promise.all([
		new Response(proc.stderr).text(),
		proc.exited,
	])
	if (exitCode !== 0) {
		throw new Error(stderr.trim() || `${args[0]} exited ${exitCode}`)
	}
}

function trackerSummary(theme: string, now: string) {
	if (theme && now) {
		return `${theme} -> ${now}`
	}
	return theme || now
}

export default tool({
	description:
		"Set the tmux pane's stable theme and immediate current-step labels for the current OpenCode session.",
	args: {
		theme: tool.schema
			.string()
			.optional()
			.describe(
				"Stable grand objective. Answer: what is this pane about overall? Keep it specific and under 48 characters. Prefer richer phrases like 'Tmux status summaries' or 'Agent tracker integration'.",
			),
		now: tool.schema
			.string()
			.optional()
			.describe(
				"Immediate next step. Answer: what are you about to do next? Keep it specific and under 48 characters. Use next-action phrasing like 'Read restore code', 'Patch status layout', or 'Wait for user reply'.",
			),
		summary: tool.schema
			.string()
			.optional()
			.describe("Legacy alias for theme. Prefer using theme plus now."),
	},
	async execute(args) {
		const tmuxPane = (process.env.TMUX_PANE || "").trim()
		const tmuxBin = Bun.which("tmux") || "/opt/homebrew/bin/tmux"
		const hasTheme = typeof args.theme === "string"
		const hasNow = typeof args.now === "string"
		const hasSummary = typeof args.summary === "string"

		if (!hasTheme && !hasNow && !hasSummary) {
			throw new Error("Provide at least one of: theme, now, or summary.")
		}

		let theme = hasTheme
			? ensureSpecific(normalizeLabel(args.theme || "", MAX_THEME_CHARS), "theme")
			: ""
		const now = hasNow
			? ensureSpecific(normalizeLabel(args.now || "", MAX_NOW_CHARS), "current-step label")
			: ""

		if (!hasTheme && hasSummary) {
			theme = ensureSpecific(normalizeLabel(args.summary || "", MAX_THEME_CHARS), "theme")
		}

		if (!tmuxPane) {
			return JSON.stringify({ theme, now })
		}

		if (!hasTheme && !hasSummary) {
			theme =
				(await readTmuxOption(tmuxBin, tmuxPane, TMUX_THEME_OPTION)) ||
				(await readTmuxOption(tmuxBin, tmuxPane, TMUX_LEGACY_OPTION))
		}

		const finalNow = hasNow ? now : await readTmuxOption(tmuxBin, tmuxPane, TMUX_NOW_OPTION)
		const finalTheme = theme

		if (hasTheme || hasSummary) {
			await writeTmuxOption(tmuxBin, tmuxPane, TMUX_THEME_OPTION, theme)
			await writeTmuxOption(tmuxBin, tmuxPane, TMUX_LEGACY_OPTION, theme)
		}

		if (hasNow) {
			await writeTmuxOption(tmuxBin, tmuxPane, TMUX_NOW_OPTION, now)
		}

		const trackerText = trackerSummary(finalTheme, finalNow)
		if (trackerText && (await Bun.file(TRACKER_BIN).exists())) {
			await runCommand([TRACKER_BIN, "tracker", "command", "-pane", tmuxPane, "-summary", trackerText, "update_task"]).catch(() => {})
		}

		await runTmux(tmuxBin, ["refresh-client", "-S"])
		return JSON.stringify({ theme: finalTheme, now: finalNow })
	},
})
