---
name: crash-course
description: Research any topic and produce a structured crash course saved to Obsidian. Takes a topic as input, dispatches a researcher subagent with a bounded effort budget, optionally validates findings, formats output as a consistent crash course document, saves to the Obsidian vault, and outputs in chat. Use when asked to learn about, research, get a crash course on, or explore any technology, concept, framework, or project.
---

# Crash Course

Research `$ARGUMENTS` and produce a structured crash course. Save to Obsidian and output in chat.

Follow each phase in order. Do not skip phases.

---

## Phase 1: Scope the Topic

Parse `$ARGUMENTS` to determine:
- **Topic**: the subject to research (e.g. "DynamoDB DAX", "Kubernetes HPA", "React Server Components")
- **Depth flag** (optional): if the user says "deep" or "comprehensive", set depth to DEEP. Otherwise default to STANDARD.
- **Vault folder** (optional): if user specifies a folder, use it. Otherwise derive a folder name from the topic in PascalCase or kebab-case matching the existing vault structure.

If `$ARGUMENTS` is empty, ask the user what topic they want to learn about and stop.

---

## Phase 2: Research

Spawn a `researcher` subagent with the following brief:

> **Effort budget: {BUDGET} tool calls.** Research the following topic to produce a crash course for an engineer being onboarded. Cover: (1) what it is and why it exists, (2) how it works architecturally, (3) key APIs/interfaces/commands, (4) when to use it and when NOT to use it, (5) limitations and gotchas, (6) pricing or cost model if applicable, (7) how it compares to alternatives, (8) best practices. Topic: {TOPIC}. Prioritize official documentation and authoritative sources over blog posts. Return a structured report with source URLs for every claim. No padding, no filler.

Set BUDGET based on depth:
- STANDARD: 10-15 tool calls
- DEEP: 20-30 tool calls

Wait for the researcher to return before proceeding.

---

## Phase 3: Validate (conditional)

If depth is DEEP, spawn a `research-validator` subagent to cross-check the researcher's findings against the cited sources. Pass the full research output.

Wait for the validator to return. Drop any findings marked CONTRADICTED. Flag UNVERIFIED findings with a note in the final output.

If depth is STANDARD, skip this phase.

---

## Phase 4: Format as Crash Course

Structure the research into the following template. Adapt sections to fit the topic - omit sections that would be empty or forced, add topic-specific sections if the research warrants them.

### Template

```markdown
# {Topic} - Crash Course

## What It Is

<1-2 paragraphs: what this is, why it exists, the problem it solves.>

## Architecture / How It Works

<Core mechanics, data flow, component relationships. Include ASCII diagrams or tables where they aid understanding.>

## Key Concepts

<The vocabulary and mental models needed to work with this effectively. Bullet points or short definitions.>

## API / Interface / Usage

<Primary APIs, commands, or interfaces. Include code examples showing the most common operations. Keep examples minimal and practical.>

## When to Use

<Bullet list of ideal use cases and conditions.>

## When NOT to Use

<Bullet list of anti-patterns, poor fits, and scenarios where alternatives are better.>

## Limitations & Gotchas

<Table or bullet list of things that will bite you. Include workarounds where known.>

## Pricing / Cost Model

<If applicable. Approximate figures, pricing dimensions, cost optimization tips.>

## Comparison to Alternatives

<Table comparing this to 2-3 alternatives on key dimensions.>

## Best Practices

<Actionable recommendations for production use.>

## Sources

<Bulleted list of URLs cited in the research.>
```

Rules for formatting:
- No em dashes. Use commas, hyphens, or colons instead.
- Direct, technical prose. No filler, no enthusiasm.
- Tables for structured comparisons. Bullet lists for enumerations.
- Code examples must be concrete and runnable (not pseudocode).
- Keep total length reasonable: STANDARD ~1500-2500 words, DEEP ~3000-5000 words.

---

## Phase 5: Save and Output

1. Save to Obsidian vault at `{folder}/{Topic} - Crash Course.md` with frontmatter:
   ```yaml
   ---
   created: {YYYY-MM-DD}
   tags: [crash-course, {topic-tag}, {domain-tag}]
   title: "{Topic} - Crash Course"
   ---
   ```
   Use the Obsidian MCP tools (`write_note`) to save. Derive `{topic-tag}` from the topic name (lowercase, hyphenated). Derive `{domain-tag}` from the broader domain (e.g. "aws", "kubernetes", "frontend", "databases").

2. Output the full crash course in chat.

3. End with a one-line summary: "Saved to {path} in your vault."
