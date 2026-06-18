---
name: implement-cookbook
description: Given a PR link containing a Valkey integration in a project, create a cookbook matching the style of valkey-io/Valkey-Samples (you can find it in ~/valkey/Valkey-Samples). Builds the cookbook locally in the project directory, verifies it runs, then copies to ~/valkey/valkey-samples/ and removes the local copy. Use when asked to create a cookbook, write a cookbook, or implement a cookbook from a PR. Also works from local git commits on the current branch.
---

# Implement Cookbook

Produce a working cookbook that demonstrates the Valkey integration in the current project.

---

## Phase 1: Analyze the Integration

Determine the input source:
- If `$ARGUMENTS` is a PR link: fetch the PR diff and description using GitHub tools.
- If `$ARGUMENTS` is empty or not a URL: use the local git history. Run `git log --oneline -20` and identify the commits related to the Valkey integration (typically the most recent commits from HEAD that touch Valkey-related files). Then run `git diff <base-commit>..HEAD` to get the full diff. Use commit messages as context for what was implemented.

From the diff (whether from PR or local git), identify:
   - The project/framework being integrated with Valkey
   - Which Valkey features are used (vector search, caching, checkpointing, pub/sub, etc.)
   - The language and package ecosystem (Python/pip, Node/npm, Java/Maven, Go/mod, etc.)
   - Key API calls and patterns introduced
3. Determine the cookbook category: `framework-integrations` or `use-cases`.
4. Decide on a kebab-case directory name matching the project name (e.g., `strands`, `langchain`, `crewai`).

---

## Phase 2: Write the Cookbook Locally

Create the cookbook directory **in the current project directory** (not in `~/valkey/valkey-samples/` yet):

```
cookbook/
  README.md
  meta.json
  01-getting-started.md
  02-<next-topic>.md
  ...
```

Aim for 2-4 cookbook pages depending on integration complexity.

### Cookbook File Patterns

All files below must follow these exact structures.

#### `README.md`

```markdown
# <Framework> + Valkey

> <N> cookbooks for <one-sentence description of what the integration does>.

## Cookbooks

| # | Cookbook | Description | Tags |
| --- | --- | --- | --- |
| 01 | <nobr>[Getting Started](01-getting-started.md)</nobr> | <One sentence>. | Beginner, ~15 min, <Language> |
| 02 | <nobr>[<Title>](02-<slug>.md)</nobr> | <One sentence>. | Intermediate, ~20 min, <Language> |
```

#### `meta.json`

```json
{
  "trackName": "<Framework>",
  "cookbooks": [
    {
      "num": "01",
      "source": "01-getting-started.md",
      "output": "01-getting-started.html",
      "title": "Getting Started with <Framework> + Valkey",
      "h1": "Getting Started with <Framework> + Valkey",
      "breadcrumb": "Getting Started",
      "lead": "<One paragraph describing what this cookbook teaches>.",
      "difficulty": "Beginner",
      "time": "15 min",
      "next": {
        "file": "02-<slug>.html",
        "title": "02 - <Title>"
      }
    },
    {
      "num": "02",
      "source": "02-<slug>.md",
      "output": "02-<slug>.html",
      "title": "<Title>",
      "h1": "<Title>",
      "breadcrumb": "<Short Title>",
      "lead": "<One paragraph>.",
      "difficulty": "Intermediate",
      "time": "20 min",
      "prev": {
        "file": "01-getting-started.html",
        "title": "01 - Getting Started"
      }
    }
  ]
}
```

Note: Last cookbook has no `next`. First cookbook has no `prev`. Middle cookbooks have both.

#### Numbered Markdown Pages (`01-getting-started.md`, etc.)

```markdown
# <H1 matching meta.json h1 field>

> <Lead paragraph matching meta.json lead field>.

**<Difficulty>** · <Language> · ~<Time>

<2-3 sentence intro explaining why this matters. What problem does it solve?>

## Prerequisites

- Docker installed
- <Language runtime version>
- <Any other prereqs like API keys>

## Step 1: Start Valkey

docker run -d --name valkey -p 6379:6379 valkey/valkey-bundle:latest

docker exec valkey valkey-cli PING
# PONG

## Step 2: Install Dependencies

<package manager install command>

## Step 3: <First concept>

<code calling project API directly - NOT raw Valkey commands>

## Step 4: <Next concept>

...

---

[<Next page number> - <Next page title> →](<next-file>.md)
```

### Content Rules

- **Think like a user of the project.** Code snippets MUST show how a developer would USE the project's own API with Valkey configured as the backend. The reader should see the project's constructor/init, then call the project's methods normally.
- Code snippets MUST call the project's functions/methods directly (e.g., `stagehand.act(...)`, `agent.store(...)`, `session_manager.save(...)`)
- Code snippets must NEVER simulate Valkey usage in place of project calls (e.g., NEVER `valkeyClient.get(key) # this simulates agent.store()`)
- Code snippets must NOT narrate Valkey internals inline (e.g., no `# this calls FT.SEARCH` inside code blocks)
- The verification script (`verify.sh`) MAY use raw Valkey commands (via GLIDE or valkey-cli) to assert that the project wrote expected keys, but the cookbook prose snippets should not
- It IS fine to show a "What Happens Under the Hood" or "Valkey Commands Fired" informational section OUTSIDE code blocks to explain what Valkey operations the project performs
- Show realistic, runnable examples derived from the PR's actual implementation
- Each page targets one concept
- Last page has no "Next" link; first page has no "Prev" link
- Navigation format: `[<NN> - <Title> →](<file>.md)`

---

## Phase 3: Create a Runnable Verification Script

Create `cookbook/verify.sh` (or `cookbook/verify.py` for Python-only cookbooks) that:
1. Starts Valkey via Docker (if not already running)
2. Installs dependencies
3. Runs the code snippets from the cookbook pages end-to-end
4. Asserts expected outputs (non-zero exit on failure)
5. Cleans up (stops Docker container)

---

## Phase 4: Run and Verify

Execute the verification script. If it fails:
- Analyze the error
- Fix the cookbook code or the verification script
- Re-run until all examples pass

Do NOT proceed to Phase 5 until verification passes.

---

## Phase 5: Move to Valkey-Samples

1. Target path: `~/valkey/valkey-samples/cookbooks/<category>/<directory-name>/`
2. Copy cookbook files (README.md, meta.json, numbered markdowns) to the target. Do NOT copy `verify.sh`/`verify.py`.
3. Remove the local `cookbook/` directory from the project.

---

## Phase 6: Summary

Report:
- Cookbook location in `~/valkey/valkey-samples/`
- Number of pages created
- What was verified (which examples ran successfully)
- Manual steps needed:
  - Update `~/valkey/valkey-samples/cookbooks/<category>/README.md` to add a row: `| <nobr>[<Framework>](<dir-name>/)</nobr> | <One-sentence description>. |`
  - Update `~/valkey/valkey-samples/cookbooks/README.md` parent table if the framework is not already listed
