import { tool } from "@opencode-ai/plugin"
import { readFileSync } from "fs"
import { homedir } from "os"
import { join } from "path"

export default tool({
  description:
    "Get a second opinion from another model. Provide the full context and your question. The other model will analyze and respond with a slightly critical perspective.",
  args: {
    context: tool.schema
      .string()
      .describe(
        "Full context: conversation summary, code snippets, options being considered, tradeoffs, etc."
      ),
    question: tool.schema
      .string()
      .describe("What you want the other model to weigh in on"),
  },
  async execute(args) {
    let model = "google/gemini-3-pro-preview"
    try {
      const configPath = join(homedir(), ".config/opencode/consult.json")
      const config = JSON.parse(readFileSync(configPath, "utf-8"))
      if (config.model) {
        model = config.model
      }
    } catch {}

    const prompt = `You are providing a second opinion with a slightly critical eye. Review this context and help with the question. Don't just agree - look for potential issues, edge cases, or alternative approaches that may have been missed.

## Context
${args.context}

## Question
${args.question}

Provide your analysis and recommendation.`

    const result =
      await Bun.$`echo ${prompt} | opencode run -m ${model}`.text()
    return result
  },
})
