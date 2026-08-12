---
name: daily-log
description: Reads today's AI journal entry from Obsidian, appends a concise daily log entry to the current month's Notes file, and updates evidence and statuses in goals/roles. Use when asked to log today, write a daily log, update the monthly notes, or record career-goal evidence.
---

# Daily Log

Read today's journal entry and create a concise daily log entry in the current month's Notes file, following the format established in prior months.

Follow each phase in order. Do not skip phases.

---

## Phase 1: Determine Date Context

Identify today's date from system context. Extract:
- **Day of week** (e.g. "Thursday")
- **Date string** for the journal entry filename: `YYYY-MM-DD`
- **Month name and year** for the Notes file: e.g. "July 2026"

---

## Phase 2: Read Format Reference

Read `Notes/June 2026.md` and `Notes/May 2026.md` from the Obsidian vault to understand the established format:

- Weeks are separated by `---` (horizontal rule)
- Each day starts with `**DayName**` (bold, no date number)
- Below the day name, categorized sections appear as top-level bullets: `- Technical`, `- Misc`
- Individual items are tab-indented bullets under the category
- Items are concise, 1 line each, written in past tense or as short noun phrases
- Project names use backticks: \`ProjectName\`
- PHD or vacation days are noted as `**DayName** - PHD` or similar on a single line

---

## Phase 3: Read Today's Journal Entry

Read `journals/entries/{YYYY}/{MM}/{YYYY-MM-DD}.md` from the Obsidian vault (e.g. `journals/entries/2026/07/2026-07-02.md`).

If the file does not exist, inform the user that no journal entry was found for today and stop.

Extract the substantive work items from the timestamped entries. Group them into:
- **Technical**: coding, implementation, debugging, reviews, research, architecture, testing, PR work, tooling
- **Misc**: meetings (non-standup), social events, training sessions, presentations, company events

Standup is always included under Technical as the first item. Co-op check-ins, 1:1s, and named meetings go under Misc unless they are deeply technical.

---

## Phase 4: Synthesize the Log Entry

Condense the journal entries into concise bullet points following these rules:

1. Each bullet should be one line, capturing the essence of what was accomplished
2. Combine related entries into a single bullet (e.g. multiple debugging entries about the same issue become one item)
3. Use action-oriented phrasing: "Addressed comments on...", "Worked on...", "Helped review..."
4. Reference project names in backticks
5. Do not include timestamps or AI-specific details (which model was used, what skill was invoked)
6. Focus on outcomes and deliverables, not process

---

## Phase 5: Read Current Month's Notes File

Read `Notes/{Month Year}.md` (e.g. `Notes/July 2026.md`) from the Obsidian vault.

Determine where to append:
- If the file is empty, start fresh with the day entry
- If the file has content, check whether a `---` separator is needed before this entry (a new week starts on Monday; if the last entry in the file was from a previous week, add `---` before this entry)
- If today's day already exists in the file, inform the user and ask whether to overwrite or skip

---

## Phase 6: Write the Entry

Append (or write) the formatted entry to `Notes/{Month Year}.md` using the Obsidian `write_note` tool with mode `append` (or `overwrite` if the file is empty).

Format:
```
**DayName**
- Technical
	- Standup
	- [item 1]
	- [item 2]
- Misc
	- [item if applicable]
```

If there are no Misc items, omit that category entirely.

After writing, output the entry in chat so the user can review it.

---

## Phase 7: Update Role Evidence

Review every Markdown file in `goals/roles/`, including the overview file. Before making or delegating any evidence or status decision, the coordinator must list and read every file and create one immutable evaluation snapshot containing:

- The exact contents of every role file, including existing evidence
- Today's journal date, path, and full entry
- The exact daily entry written in Phase 6 and its Notes path
- The evidence and status rules below

Every evaluator must receive and inspect this same complete snapshot, either inline or through the same shared read-only reference. Do not provide different or partial summaries of the role ladder.

Parallelize only when the runtime can start subagents, provide the complete snapshot to each one, and collect every result before any role-file write. Partition status-bearing criteria into disjoint assignments, preferably by shared section across role levels so related cumulative criteria have one owner. Assign each `(file path, exact criterion bullet)` to exactly one evaluator. Exclude overview vocabulary or ladder-description lines unless they are actual status-bearing criteria.

Subagents are read-only. They may propose changes only for their assigned criteria and must not edit the journal, current-month Notes file, or role files. The coordinator is the sole writer. If these parallelization conditions are unavailable, perform the same evaluation sequentially.

For each role criterion bullet that has a status in brackets:

1. Compare the criterion with the substantive work in today's journal entry and the daily entry just added to the current month's Notes file.
2. When today's work directly supports the criterion, add a concise evidence bullet immediately beneath that criterion. Use the exact journal date (`YYYY-MM-DD`) for journal evidence and a month-level date for Notes evidence. Link to the source note, for example `[[journals/entries/2026/07/2026-07-15]]` or `[[Notes/July 2026]]`.
3. Preserve existing evidence. Extend an existing dated evidence bullet only when today's work is materially part of the same evidence period; otherwise add a new dated bullet. Do not add duplicate claims or links.
4. Update the criterion status only when the evidence warrants it:
   - `[demonstrated]`: direct evidence substantially satisfies the full criterion
   - `[partial]`: relevant evidence exists, but the criterion is broader, conditional, or not fully proven
   - `[not yet]`: no direct supporting evidence is present
5. Do not infer qualifications, years of experience, recruitment, travel, customer relationships, formal people leadership, company-wide adoption, or external-community leadership from unrelated technical work. Keep `[partial]` when the note itself says the broader requirement is not established.

Each evaluator must return proposals only, using one record per proposed criterion change:

- `file`: exact role-file path
- `criterion`: exact current criterion bullet
- `evidence`: exact evidence bullet to add or extend, or `none`
- `status`: current status and proposed status, or `unchanged`
- `reason`: one concise sentence tied to the source evidence

The evaluator must also identify every assigned criterion or file for which no change is proposed.

Wait for all assignments before aggregation. If a result is missing, malformed, or outside its assignment, the coordinator evaluates that assignment directly from the shared snapshot. Merge proposals by `(file, criterion)`. Deduplicate identical proposals. Treat non-identical proposals for the same key or contradictory factual claims as conflicts; resolve them against the complete snapshot and the rules above, never by voting or averaging. Reusing the same evidence for separate cumulative criteria is not a conflict by itself. If a conflict remains unresolved, preserve the current status, omit the disputed edit, and report it.

Before writing, re-read every target file. If its targeted criterion, status, or evidence changed after the snapshot was created, re-evaluate the affected proposal instead of overwriting newer content. Apply accepted changes one role file at a time; never perform concurrent role-file writes.

Make the smallest possible edits, preserve existing evidence and Markdown structure, and leave files unchanged when today's work provides no new evidence. The overview file normally only documents the status vocabulary and role ladder; do not add evidence to it unless it contains an actual status-bearing criterion.

Report changed role files, status changes, skipped proposals, and unresolved conflicts.

## Phase 8: Confirm

End with a one-line confirmation: "Added {DayName}'s entry to Notes/{Month Year}.md"
