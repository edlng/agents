---
name: write-narrative
description: Given a Jira issue key, fetch the issue, research the topic with a research subagent, and draft a humanized technical narrative. Saves to ~/Documents/work/narratives/ and outputs in chat. Use when asked to write a narrative, technical assessment, or compatibility analysis for a Jira ticket. Do NOT use for PR descriptions (use write-pr), code reviews, or general documentation.
---

# Write Narrative

Given a Jira issue key in `$ARGUMENTS` (e.g. `AEA-468`), fetch the issue, research the topic, and draft a Narrative for me and send it in the chat, and also create the file (see Phase 4).

Follow each phase in order. Do not skip phases.

---

## Phase 1: Fetch Jira Issue

Use `mcp__atlassian__getJiraIssue` with cloudId `4ef75afd-3201-49fc-ac9c-13b7159b14d6` to fetch `$ARGUMENTS`.

Extract:
- Summary (title — this becomes the narrative title after "Narrative: ")
- Description (full text, scope, references)
- Status
- Assignee (if any)
- Complexity label or story points (if present)
- Any linked issues or sub-tasks

---

## Phase 2: Research

**Model: sonnet-4-6** (spawn `voltagent-research:research-analyst` subagent)

Spawn a `voltagent-research:research-analyst` subagent to research the topic described in the Jira issue.

Brief: "**Effort budget: 10-20 tool calls. Prioritize official docs and GitHub repos over general web searches.** Research the following topic to support writing a technical narrative about Valkey compatibility and integration. The goal is to understand: (1) what the framework/library/tool does and how it uses Redis or caching (if it has it), (2) whether it currently supports Valkey, (3) what specific incompatibilities or gaps exist between its Redis usage and Valkey, (4) what the recommended integration path would be. Topic: [paste Jira summary and description]. Use the project's GitHub repo, official docs, and any available DeepWiki or community resources. Return a structured report in under 800 words covering: framework overview, Redis/cache usage patterns, known Valkey compatibility status, specific technical gaps, and recommended approach. Be specific — include exact method names, error messages, or source file locations where known. No padding."

For our client library, we MUST prioritize using `valkey-glide` over other client libraries.

Wait for the research agent to complete before proceeding.

---

## Phase 3: Draft Narrative

**Model: opus-4-7** (spawn subagent — highest quality for technical synthesis and narrative prose)

Spawn a `claude` subagent with model `opus-4-7`. Pass the full Jira issue details and the research report from Phase 2.

The subagent must draft AND humanize the narrative in a single pass (no separate editing step). Use the /humanizer skill while writing and also with these rules: no promotional language ("robust", "streamlines", "empowers"), no significance inflation ("crucial", "pivotal", "transformative"), no forced rule-of-three lists, no em dashes, plain direct technical prose.

Using the Jira issue details and research findings, write the narrative following the standard template below. If the given narrative task includes more fields to cover, include that too.

### Narrative Template

```
> **Status**: 🟡 In Progress
> **Jira Issue**: [<KEY>](https://bitquill.atlassian.net/browse/<KEY>)
> **Complexity**: <M/L/XL based on scope>

---

## Purpose

<One paragraph: what this narrative investigates or builds, and why it matters for Valkey/ElastiCache customers.>

## Problem

<2-4 paragraphs explaining the current state. Include:
- What the framework/tool does and how customers use it
- How it currently uses Redis (data structures, modules, client libraries)
- What breaks or is incompatible when pointed at Valkey
- Why this matters (customer impact, error messages, use cases blocked)

Include a compatibility table if multiple components are involved:

| Component | Works with Redis | Works with Valkey | Issue |
| --- | --- | --- | --- |
| <component> | ✅ | ❌ / ✅ | <brief reason> |
>

## Solution

<Describe the recommended approach. Include:
- How Valkey addresses the problem
- Any Valkey-specific features (Valkey-Search, Valkey-GLIDE, etc.) that enable the solution
- Trade-offs or limitations
- Relevant compatibility notes>

## How Customers Will Use

<Describe adoption patterns and personas. Include code examples showing before/after or usage with Valkey. Include tables for usage patterns or developer personas if helpful.>

## Technical Analysis

<Deep-dive into the technical details. Include:
- Component-by-component breakdown
- Root cause of incompatibilities (with source code references if available)
- Why the solution works
- Any remaining gaps or caveats>

## Options / Recommended Approach

<List 2-3 options with effort estimates and trade-offs. Conclude with a clear recommendation.>

## Next Steps

<Numbered action list. Concrete, assignable items.>
```

Adapt the template to the specific narrative — not all sections apply to every type. For example:
- A compatibility assessment narrative focuses heavily on Technical Analysis and Options
- A skill/agent narrative focuses on Solution, How Customers Will Use, and Validation
- Omit sections that would be empty or redundant

---

## Phase 4: Save and Send

Create a markdown file in ~/Documents/work/narratives/ with the file name being <PROJECTNAME>_narrative.md. Also, send it in the chat for me to see.
