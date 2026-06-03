---
name: diagram-valkey-flow
description: Analyze a project and generate a Mermaid sequence diagram showing the full data flow from the application layer through any framework/middleware down to Valkey operations. Works for any language or framework.
---

# Diagram Valkey Flow

Analyze the project at `$ARGUMENTS` (or the current working directory if blank) and produce a Mermaid sequence diagram showing how data flows from the application entry point through to Valkey.

Run all phases in order. Do not skip phases.

---

## Phase 1: Discover Entry Points and Framework

Identify what kind of project this is and where execution begins.

```bash
# Check for common entry points
find . -maxdepth 3 \( \
  -name "main.ts" -o -name "main.js" -o -name "index.ts" -o -name "index.js" \
  -o -name "app.ts" -o -name "app.py" -o -name "main.py" \
  -o -name "main.go" -o -name "*.rb" \
\) -not -path "*/node_modules/*" -not -path "*/.git/*"

# Check package.json for framework hints
cat package.json 2>/dev/null || cat pyproject.toml 2>/dev/null || cat go.mod 2>/dev/null
```

Record: entry point files, framework name (Genkit, LangChain, LlamaIndex, Express, FastAPI, etc.).

---

## Phase 2: Trace the Valkey Integration

### 2.1 — Find Valkey client usage

```bash
grep -rn "GlideClient\|createClient\|valkey\|redis\|ValkeyClient\|RedisClient" \
  --include="*.ts" --include="*.js" --include="*.py" --include="*.go" \
  -l \
  . | grep -v node_modules | grep -v ".git"
```

Read each matched file fully. Identify:
- Where the client is instantiated (constructor args, host, port, config)
- Every Valkey command called: `hset`, `hget`, `FT.CREATE`, `FT.SEARCH`, `get`, `set`, `del`, etc.
- What data is passed in and what is returned

### 2.2 — Find middleware / plugin / adapter layer

```bash
grep -rn "defineIndexer\|defineRetriever\|addDocuments\|similaritySearch\|index\|retrieve\|embed" \
  --include="*.ts" --include="*.js" --include="*.py" --include="*.go" \
  . | grep -v node_modules | grep -v ".git"
```

Read each matched file. Identify:
- The abstraction layer between the app and the raw Valkey client
- What functions/methods the app calls on this layer
- How that layer translates calls into Valkey operations

### 2.3 — Find the caller (application layer)

```bash
grep -rn "index\|retrieve\|search\|query\|generate\|run\|invoke" \
  --include="*.ts" --include="*.js" --include="*.py" --include="*.go" \
  . | grep -v node_modules | grep -v ".git" | grep -v "node_modules"
```

Read the top-level files that call the middleware/plugin. Identify:
- What triggers a flow (user input, HTTP request, function call)
- Which high-level methods are called
- What the final output is

---

## Phase 3: Build a Draft Diagram

Using everything collected, produce a draft Mermaid `sequenceDiagram` with a clear label: **[DRAFT — pending validation]**.

Rules:
- Participants must reflect real layers in THIS project. Use actual class/function/method names — never invent names.
- Show both the **indexing flow** (storing documents + embeddings) and the **retrieval flow** (querying) as separate `alt` blocks if both exist.
- Label each arrow with the actual function or command name found in source (e.g., `hset(key, fields)`, `GlideFt.search(...)`).
- Show async operations with `activate` / `deactivate` if the code uses `await` or promises.
- Add a `Note` where a meaningful transformation happens (e.g., text → embedding vector, Float32Array → Buffer).
- Keep it readable: collapse obvious no-op steps, don't show internal framework bootstrap noise.

---

## Phase 4: Validate the Diagram Against Source

Before finalising, verify every claim in the draft diagram. For each arrow or note in the draft, confirm it is grounded in source you actually read.

Work through this checklist:

### 4.1 — Verify every participant exists
For each participant name in the diagram, confirm it appears in a source file you read. If any participant name was inferred or guessed, re-read the relevant file and correct it.

### 4.2 — Verify every arrow label is a real call
For each arrow label (function/method/command), grep for it:

```bash
grep -rn "<label>" --include="*.ts" --include="*.js" --include="*.py" --include="*.go" \
  . | grep -v node_modules | grep -v ".git"
```

If the grep returns no results, the label is wrong — find the correct name and fix it in the diagram.

### 4.3 — Verify call order matches source
For any file where multiple Valkey calls happen, re-read that file and confirm the sequence of calls in the diagram matches the actual order in the code. Fix any ordering mistakes.

### 4.4 — Verify data transformations
For each `Note` describing a transformation, confirm the transformation code is present in a file you read (e.g., `new Float32Array(embedding).buffer`, `Buffer.from(...)`). Remove any note you cannot verify.

### 4.5 — Flag gaps
If a part of the flow could not be traced to source (e.g., caller code not found), mark it in the diagram with a `Note` like: `Note over X: caller not found in source — inferred`.

---

## Phase 5: Write the Reference Document

Determine the repo name from the project path (e.g., `genkit`, `my-app`). The output file is:

```
~/Documents/work/Projects/<repo-name>-valkey-flow.md
```

Create or overwrite that file with the following structure. Every section is required.

---

### Document structure

```markdown
# <repo-name> — Valkey Flow Reference

> Generated: <date>
> Project path: <absolute path>
> Framework: <detected framework name>

---

## How to Run

Concise step-by-step instructions a developer needs to actually execute the Valkey-integrated flow in this project:
1. Prerequisites (Valkey instance, env vars, dependencies)
2. How to start or invoke the flow (CLI command, function call, HTTP request, test command)
3. Any configuration required (index name, embedder, dimension, host/port)

Include real commands copied from source (package.json scripts, justfile targets, test runner invocations, etc.).

---

## How to Call the Integration

Show the exact API surface a caller uses — real function signatures, types, and a minimal working example derived from actual source or test files:

- Plugin/client instantiation with real parameter names
- Indexer call: function name, input type, what it stores
- Retriever call: function name, query type, return type
- Any ref helpers (e.g., `valkeyIndexerRef`, `valkeyRetrieverRef`) with their signatures

---

## Tests

List every test file found (grep for `*.test.ts`, `*.spec.ts`, `*_test.py`, `*_test.go`, etc.) and for each:
- File path
- What scenarios it covers (one line each)
- How to run it (exact command)

If no tests exist, say so explicitly.

---

## Flow Description

A technical prose explanation of the end-to-end flow. Detailed enough that a developer can understand the mechanics without reading the source. Cover:

**Initialization**
What happens when the plugin/client is set up: connection establishment, index creation logic (including what happens if the index already exists), schema (field names, types, algorithm, distance metric, dimensions).

**Indexing Flow**
Step-by-step: how documents enter the system, how embeddings are generated, how data is serialized (e.g., Float32Array → Buffer), what key structure is used in Valkey, which Valkey command stores the data and what fields it writes.

**Retrieval Flow**
Step-by-step: how a query enters the system, how the query vector is built, the exact Valkey search command and query syntax used (e.g., KNN syntax), which fields are returned, how results are deserialized back into documents.

**Error Handling**
Any error cases handled in the code and how they are treated.

---

## Sequence Diagram

<the validated Mermaid diagram from Phase 4>

---

## Validation Summary

| Claim | Source file:line | Status |
|---|---|---|
| `GlideClient.createClient(config)` | `src/index.ts:61` | Verified |
| ... | ... | ... |

---

## Key Observations

- Bullet list (3–5 items): notable design decisions, data transformations, index config, error handling, or gaps
```

---

After writing the file, print its path and a one-line confirmation. Do not re-print the full document content in the conversation — just confirm it was written.

---

## Phase 6: Write a Runnable Sample App

Using everything learned in Phases 1–4, generate a **self-contained, runnable sample file** that exercises the framework's own API end-to-end. The goal is not to test Valkey in isolation — it is to start the app the way it is actually started, call the real functions the app exposes, and have Valkey get hit as a natural consequence. Someone who has never seen the repo should be able to follow the setup steps and run it cold.

### 6.1 — Determine the output file path and language

- Match the primary language detected in Phase 1.
- Place the file in the shared samples directory:

```
~/Documents/work/Projects/samples/<repo-name>-valkey-sample.<ext>
```

Where `<ext>` is `ts`, `py`, `go`, `js`, etc.

### 6.2 — Identify the framework entry point and invocation pattern

Before writing a single line of sample code, answer these questions from what you found in Phases 1–4:

- **How is the app started?** (e.g., `genkit start`, `uvicorn app:main`, `node index.js`, a specific CLI command, a test runner, an agent `.kickoff()`)
- **What is the top-level callable?** (e.g., a Genkit flow function, a LangChain chain, a LlamaIndex query engine, a PraisonAI `PraisonAIAgents` instance, a FastAPI route handler called directly)
- **What input does it take and what does it return?** (the exact types from the source signatures)
- **Does it need to be bootstrapped first?** (e.g., Genkit requires `configureGenkit(...)`, LangChain may need an `OpenAI` client, PraisonAI needs agent definitions loaded)

Do not proceed to 6.3 until you have answered all four questions from actual source you read in earlier phases. If any answer is unclear, re-read the relevant file now.

### 6.3 — Write the sample file

Structure the file in this order:

1. **Header comment block** containing:
   - One sentence: what this sample does at the framework level (e.g., "Runs the Genkit RAG flow with Valkey as the vector store, indexes 3 documents, then queries them.")
   - **Prerequisites** — every requirement to run this from scratch:
     - Valkey/Redis: exact `docker run` or `valkey-server` command to start it
     - Required env vars (names only, no real values) — pulled from actual env var usage in source
     - Install command (`npm install`, `pip install -r requirements.txt`, `go mod tidy`, etc.) with exact package names/versions from the project manifest
   - **How to run**: the single command to execute this file from the repo root

2. **App / framework bootstrap** — replicate the real initialisation sequence from the source:
   - Configure the framework (e.g., `configureGenkit(...)`, `Settings.llm = ...`, agent config)
   - Instantiate the Valkey-backed store/plugin/retriever using the same constructor and options as in source
   - Do not open a raw Valkey connection directly unless the source does it that way

3. **Indexing step** — call the framework's own ingestion API (e.g., `index([...docs])`, `addDocuments(...)`, `agent.index(...)`) with 3–5 hardcoded example documents whose content is meaningful relative to the query you'll run next. Valkey gets written to as a side effect.

4. **Query / retrieval step** — call the framework's own query API (e.g., invoke the flow, call `chain.invoke(...)`, run `queryEngine.query(...)`, trigger the agent). Use a query string that should match at least one of the indexed documents. Do not call `FT.SEARCH` directly.

5. **Print the result** — log the framework-level response (the answer, retrieved docs, agent output) so the user can see the full round-trip worked.

6. **Cleanup** — if the framework exposes a teardown (close connections, drop index) do it here so re-runs are idempotent. Fall back to the raw client only if no framework-level teardown exists.

### 6.4 — Rules for the sample

- **Prefer Ollama for models.** Use Ollama-backed models wherever the framework allows (LLM, embedder, or both). Check the source for any existing Ollama configuration first and reuse it. If Ollama is not already wired in, substitute it in the sample using the framework's standard Ollama integration (e.g., `ollama` plugin for Genkit, `OllamaEmbeddings` / `Ollama` for LangChain, `OllamaEmbedding` / `Ollama` for LlamaIndex, `ollama/` model prefix for PraisonAI). Add `ollama pull <model>` to the Prerequisites comment so the user knows which model to pull before running. Only fall back to a cloud provider (OpenAI, Gemini, etc.) if the framework has no Ollama integration path.
- **Never bypass the framework.** Valkey must be reached through the same call stack the real app uses. No raw `hset`/`FT.SEARCH` calls unless the source itself has no abstraction layer.
- Use only libraries that already appear in the project's dependency manifest. Do not introduce new packages.
- Use real function names, class names, option keys, and env var names from the source — never invent names.
- If the project uses TypeScript, the sample must compile with the project's existing `tsconfig.json` (or note the flag needed).
- **Cover every distinct integration point in one file.** If the project uses both an embedder and an LLM, the sample must exercise both — not just one. The same applies to any other separable capability found in the flow (e.g., a reranker, a summariser, a tool call, an agent step). Each integration point gets its own clearly labelled section in the file. Concise means no padding, not missing parts.
- Keep it under ~150 lines by being tight, not by omitting integration points. If the file must go over to cover everything, go over — but cut any line that doesn't directly demonstrate a distinct capability.
- Do **not** hardcode credentials. Use `process.env.VAR` / `os.environ["VAR"]` / `os.Getenv("VAR")` matching the env var names already in source.

### 6.5 — After writing

Write an identical copy of the sample file to the project root of the repo being analyzed (i.e., the working directory passed as `$ARGUMENTS`, or the current working directory if blank):

```
<project-root>/valkey-sample.<ext>
```

This lets the user run it directly from their IDE or terminal without navigating anywhere. The file at the project root and the file under `Projects/samples/` must be byte-for-byte identical — write one and copy, do not maintain two separate versions.

Then print both file paths and a one-line confirmation. Do not print the file contents in the conversation.
