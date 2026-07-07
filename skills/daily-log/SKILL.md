---
name: daily-log
description: Reads today's AI journal entry from Obsidian, then appends a concise daily log entry to the current month's Notes file matching the established format (bold day name, categorized bullet points). Use when asked to log today, write a daily log, update the monthly notes, or add today's entry.
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

## Phase 7: Confirm

End with a one-line confirmation: "Added {DayName}'s entry to Notes/{Month Year}.md"
