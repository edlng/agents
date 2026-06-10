---
name: pr-comment-humanizer
version: 1.0.0
description: |
  Humanize PR review comments to match the author's terse, direct code review voice.
  Strips AI-isms and rewrites comments to sound like a real engineer: short imperatives,
  parenthetical precision, "nit" prefix for minor things, "we should" for collaborative
  framing, questions to verify rather than assert, links instead of explanations.
license: MIT
compatibility: claude-code opencode
allowed-tools:
  - Read
  - Write
  - Edit
---

# PR Comment Humanizer

Rewrite PR review comment text to match this specific voice: terse, direct, imperative. No warmup, no hedging, no AI vocabulary.

## Voice Profile

Derived from real PR comments at https://github.com/edlng/ChatDev/pull/1.

**Core traits:**

- **Brutally terse.** If one line does it, use one line. "valkey-glide 2.4" not "Please update the dependency version to valkey-glide 2.4 to ensure compatibility."
- **Imperative by default.** "Add another note about X" not "It would be good to add a note about X" or "Consider adding a note about X."
- **"Might as well"** for low-effort improvements worth doing while you're there. "Might as well also add a case for an upper bound."
- **"nit"** prefix (lowercase) for minor stuff that won't block merge. Never "minor nit" or "small nit" — just "nit".
- **"Same concern as above"** to cross-reference instead of repeating yourself.
- **Parenthetical precision.** Use `(i.e., ...)` and `(e.g., ...)` inline rather than new sentences.
- **"We should"** not "you should" — treats it as a shared problem.
- **Questions to verify, not assert.** "Can you confirm that X?" not "X is incorrect."
- **Links instead of explanations.** Drop the doc URL and stop. Don't summarize what's at the link.
- **Backtick code references inline.** Always wrap identifiers, values, branch names in backticks.
- **No period on single-line comments.** Multi-sentence comments get periods; one-liners often don't.

## What to strip

All 29 patterns from `_shared/humanizer-rules.md` apply (content patterns 1-6, language/grammar 7-13, style 14-19, communication 20-22, filler/hedging 23-29), plus these PR-comment-specific ones:

- **No "consider"** — use imperative or "might as well"
- **No "it would be good to"** — just say what to do
- **No "please"** — it's a code review, not a polite request
- **No "this looks good but"** — lead with the finding
- **No "I noticed that"** or "I see that"** — just state the observation
- **No "to ensure X"** trailing clauses — cut them, the reader knows why
- **No "as per"** — use "per" or rewrite
- **No severity labels in the body** (Blocking/Recommended/Nit) — severity is conveyed by word choice and "nit" prefix

## Process

1. Read the comment body
2. Strip all AI-isms from the general humanizer list
3. Strip PR-comment-specific patterns above
4. Rewrite using the voice profile
5. Self-check: read it aloud — would a real engineer write this in a 2am code review?
6. If no, cut more

## Examples

**Before:**
> I noticed that you might want to consider adding validation for the upper bound as well (i.e., values greater than 65535). This would ensure that the error message is more general and covers all invalid port scenarios. It would be good to change the error to something like "must be a valid port 1-65535" to make it clearer for users.

**After:**
> Might as well also add a case for an upper bound (i.e., `> 65535`) and change the error to be more general (e.g., must be a valid port 1-65535).

---

**Before:**
> This is a great start! However, I noticed that the tests for the two scenarios are in separate test methods. It would be better to consolidate these into a single test to improve readability and reduce redundancy.

**After:**
> Collapse this into one test

---

**Before:**
> Please note that this PR appears to be targeting the `main` branch. We should follow proper git practices and merge into the `add-valkey` branch instead, especially since we plan to open a PR from `edlng:add-valkey` to `upstream:main` later on. You may need to reopen another PR to change the target branch.

**After:**
> We should be merging from your branch into `add-valkey` instead of into `main`. It's proper practice especially once we open a PR on the main repo (`edlng:add-valkey` -> `upstream:main`). You may have to reopen another PR.

---

**Before:**
> I wanted to flag that the same issue exists here as in the previous comment regarding the database index validation. Valkey supports 16 logical databases numbered 0-15. You can find more information about this in the official Valkey documentation.

**After:**
> Same concern as above comment. Valkey has 16 logical databases 0-15 (https://valkey.io/commands/select/).
