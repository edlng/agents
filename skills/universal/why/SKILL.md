---
name: why
description: "Use for 'why does X work this way', 'why we picked Y', design rationale, regressions, postmortems, or data-backed thresholds. Queries git and gh, Pippin, and the Amazon internal wiki in parallel, then returns a cited read on decisions and tradeoffs. Use how for runtime behavior."
---

# Why

Investigate the motivation and intent behind code.

Companion to the `how` skill. `how` answers what the code does and how it works. `why` answers what forces led to its shape.

Investigators run as the read-only `explore` agent at its catalog model and effort. When the investigator prompt sets an output format, it replaces the agent's default survey report. You synthesize in the main session.

## Operating Posture

Operate as a **careful, cautious, and precise investigator**. Be honest about what you know vs what you're inferring. Read `references/epistemics.md` for the full confidence framework and phrasing guide. The synthesizer must follow it.

## Step 1. Understand the Target and the Question

Parse what the user is asking. The **target** is usually a chunk of code, a pattern, a feature, or a named design decision. The **question** is usually a design rationale, a tradeoff, a motivating edge case, an external constraint, dead code, or a broad history sweep.

If the target is vague ("why do we do it this way?" with no clear referent), make your best guess from conversation context (open files, recent edits, what was just discussed). State your interpretation briefly so the user can redirect if you're off, then proceed.

## Step 2. Establish the Code Anchor

Before spawning investigators, anchor the investigation in concrete code. You need:

- The relevant file path(s) and line range(s)
- The key symbols (function names, class names, constants)
- An initial commit list. The last few commits touching the target.
- PR numbers from merge commits (pattern `(#1234)` in the subject line)

Build this inline.

```bash
# Blame target lines for last-touch commits
git blame -L <start>,<end> <file>

# Full file history, with patches, through renames
git log --follow -p -- <file>

# Last N commits touching the file, PR numbers visible
git log --oneline -20 -- <file>

# Extract PR numbers from a commit message
git log -1 --format=%B <commit>
```

Pull PR bodies and discussion via `gh` for any substantive commits:

```bash
gh pr view <number> --json title,body,author,createdAt,mergedAt,labels,closingIssuesReferences,comments,reviews
```

Capture this as seed context (file paths, symbols, commits, PR numbers, linked ticket IDs). Pass it to the investigators.

## Step 3. Spawn Parallel Investigators (default posture)

**Default to the full parallel investigation.**

### Investigator roster. Three fixed sources

Spawn these three investigators in a single message so they run concurrently. Each owns exactly one source. Don't ask one agent to cover several.

1. **Source control investigator.** Git history, `gh` for PRs, code comments, tests. Playbook `references/sources/code-archaeology.md`. Always spawn. The only guaranteed source. Best at surfacing *implementation-time rationale captured during review*.
2. **Pippin investigator.** Design docs, specs, and review comments in Pippin, through the Pippin MCP read tools. Playbook `references/sources/pippin.md`. Best at surfacing *long-form design rationale written before the code*.
3. **Wiki investigator.** Amazon internal wiki pages, through `InternalSearch` and `ReadInternalWebsites`. Playbook `references/sources/wiki.md`. Best at surfacing *team runbooks, onboarding pages, and decision records that never reached a design doc*.

Each investigator gets:
1. The base prompt from `references/investigator-prompt.md`
2. Its playbook from the roster above
3. The cross-cutting `references/sources/incident-postmortem.md` **if the target code looks defensive** (null checks, retry logic, timeout handling, rate limiting, feature flags, egress guards, OOM handlers)
4. The code anchor from Step 2 (file paths, symbols, commit hashes, PR numbers, ticket IDs)
5. The user's original question

### When to skip an investigator

Only skip with an **explicit, written justification** that goes in the final "Sources Consulted" section. Two valid reasons:

- **The source is unreachable** in this session: the Pippin MCP or the internal search tools are not connected, or authentication failed. Flag this as a gap, not a choice.
- **The source is provably irrelevant**, not just "probably irrelevant." A high bar. Example: "Wiki skipped. Target is a third-party vendored file with no team ownership."

If the target is a single-commit change whose PR description already contains the complete answer, you may answer inline **only after** confirming the Pippin and wiki searches would be redundant. Say so explicitly. This should be rare.

## Step 4. Synthesize

Write the synthesis yourself in the main session. You already hold the investigator findings, so do not spawn another agent. Follow `references/synthesizer-prompt.md` as your instructions and `references/epistemics.md` for confidence language. Spot-check key citations with the same tools the investigators used.

Your inputs:
1. The investigator findings, including any null results and any sources skipped with justification
2. The code anchor from Step 2 (file paths, symbols, commit hashes, PR numbers, ticket IDs)
3. The user's original question

## Step 5. Present

Present the synthesis to the user. Keep the confidence language from `references/epistemics.md` intact.

## Output Format

The output structure is the one in `references/synthesizer-prompt.md`: The Question, The Code in Question, What We Found, What We Can Reasonably Infer, Competing Hypotheses, What We Don't Know, Sources Consulted, Confidence Summary. Adapt as needed, but keep the confidence separation intact, and keep Sources Consulted as one line per investigator, including the ones that returned nothing or were skipped, with the reason.

After the Sources Consulted block, if the user's `why` question is a precursor to actually changing this code, convert the lineage findings into a Preserve / Change / Avoid / Risk constraint set suitable for planning the change.

## Common Failure Modes to Avoid

- **Recency bias**. Assuming the most recent commit is authoritative. The current shape is often the accretion of many earlier decisions. Trace back.

## Reference Files

- `references/epistemics.md`. Confidence tiers and phrasing guide. The synthesizer must follow it.
- `references/investigator-prompt.md`. Base prompt template for investigator subagents.
- `references/source-playbook.md`. Index pointing at the category playbooks below.
- `references/sources/*.md`. One playbook per source (`code-archaeology.md`, `pippin.md`, `wiki.md`), plus cross-cutting `incident-postmortem.md`.
- `references/synthesizer-prompt.md`. Instructions for the synthesis step, including the output format.
