---
name: check-valkey-search-compatibility
description: Analyze a repository for Redis/Valkey Search compatibility. Determines whether an existing Redis/Valkey integration or vector store exists, identifies the best implementation blueprint, recommends the specific implementation path, and evaluates applicable Valkey use cases vs. competing providers. Use when asked to add Valkey/Redis vector search to a project or assess search compatibility.
---

# Check Valkey Search Compatibility

Analyze the repository `$ARGUMENTS` (or the current working directory if blank) for Redis/Valkey Search compatibility. Run both phases in order. Do not skip phases.

This is a two-phase workflow: **Phase 1 scans and collects evidence**, **Phase 2 synthesizes and validates findings** before producing the final report.

---

## Phase 1: Evidence Collection

**Model: sonnet-4-6**

### 1.1 — Detect Existing Redis / Valkey Integration

Search the codebase for direct Redis or Valkey usage:

```bash
# Dependency files
grep -rn "redis\|valkey\|valkey-glide\|ioredis\|redis-py\|aioredis\|jedis\|lettuce\|stackexchange.redis" \
  --include="*.json" --include="*.toml" --include="*.txt" --include="*.gradle" \
  --include="*.xml" --include="*.lock" -i .

# Source code imports and connections
grep -rn "import redis\|import valkey\|from redis\|from valkey\|require.*redis\|require.*valkey\|RedisClient\|ValkeyClient\|createClient\|StrictRedis\|ConnectionPool" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.rb" --include="*.cs" -i .

# Vector search commands (RediSearch / VSS)
grep -rn "FT\.CREATE\|FT\.SEARCH\|FT\.AGGREGATE\|HNSW\|FLAT.*VECTOR\|vector_field\|SearchIndex\|VectorField\|TextField\|NumericField\|TagField" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.yml" --include="*.yaml" -i .
```

Record: file paths, line numbers, library versions found.

### 1.2 — Detect Existing Vector Store / Database Implementations

Search for other vector store providers that could serve as implementation blueprints:

```bash
# Vector store providers
grep -rn "pinecone\|weaviate\|qdrant\|milvus\|chromadb\|chroma\|pgvector\|opensearch.*vector\|elasticsearch.*vector\|faiss\|annoy\|scann\|lancedb\|zilliz" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.toml" --include="*.txt" --include="*.json" \
  --include="*.lock" -i .

# LLM framework vector store patterns
grep -rn "VectorStore\|vectorstore\|vector_store\|VectorDB\|EmbeddingStore\|similarity_search\|add_documents\|add_texts\|from_documents\|from_texts\|as_retriever" \
  --include="*.py" --include="*.ts" --include="*.js" -i .

# Framework-specific store classes
grep -rn "class.*VectorStore\|class.*Retriever\|class.*EmbeddingStore\|BaseVectorStore\|extends VectorStore\|implements VectorStore" \
  --include="*.py" --include="*.ts" --include="*.js" -i .
```

Record: which providers are present, the file paths of their implementation classes.

### 1.3 — Identify Framework and Language Context

```bash
# Framework detection
find . -maxdepth 3 \( -name "package.json" -o -name "pyproject.toml" \
  -o -name "requirements*.txt" -o -name "setup.py" -o -name "go.mod" \
  -o -name "pom.xml" -o -name "build.gradle" \) | head -20

# LangChain / LlamaIndex / Haystack / AutoGen / PraisonAI
grep -rn "langchain\|llama.index\|llama_index\|haystack\|autogen\|praisonai\|praison" \
  --include="*.json" --include="*.toml" --include="*.txt" -i . | head -30
```

Identify the primary framework (LangChain, LlamaIndex, Haystack, PraisonAI, raw SDK, etc.) and primary language.

### 1.4 — Inspect Best Blueprint Candidate

If a vector store provider was found in 1.2, read the implementation files for the most structurally complete provider (pick the one with the most methods matching `similarity_search`, `add_documents`, etc.):

- Read up to 3 implementation files (max 200 lines each)
- Note: constructor signature, connection setup, index creation pattern, embedding handling, search method signature, CRUD operations present

### 1.5 — Cache Phase 1 Findings

Write a structured JSON summary to `/tmp/valkey_compat_scan.json`:

```json
{
  "redis_valkey_found": true,
  "redis_files": ["path:line"],
  "vector_search_commands_found": true,
  "vector_store_providers": ["pinecone"],
  "blueprint_file": "path/to/best/candidate.py",
  "framework": "langchain|llamaindex|haystack|praisonai|raw|unknown",
  "language": "python|typescript|java|go|...",
  "embedding_pattern": "how embeddings are passed in the blueprint",
  "blueprint_methods": ["similarity_search", "add_documents"]
}
```

---

## Phase 2: Synthesis, Validation & Report

**Model: sonnet-4-6**

Read `/tmp/valkey_compat_scan.json` before starting this phase.

### 2.1 — Validate Findings

For each positive signal from Phase 1, do a quick spot-check:

- If `redis_valkey_found=true`: read one of the flagged files to confirm it's active code (not a comment or test fixture).
- If a blueprint was identified: verify the file still exists and re-read its class signature.
- If `vector_search_commands_found=true`: confirm at least one match is in a live code path, not test data.

Adjust findings if spot-checks reveal false positives.

### 2.2 — Determine Implementation Approach

Based on validated findings, determine the recommended implementation path:

**Path A — Extend Existing Redis/Valkey Client**
Use when: `redis_valkey_found=true` and a connection/client already exists.
Action: extend the existing client to add VSS index creation, vector field mapping, and `FT.SEARCH` with KNN query.

**Path B — Port from Blueprint Provider**
Use when: `redis_valkey_found=false` but another vector store provider exists as a blueprint.
Action: mirror the blueprint's interface (`__init__`, `add_texts`/`add_documents`, `similarity_search`, `as_retriever`) and implement against Valkey Search commands.

**Path C — Greenfield Implementation**
Use when: neither Redis/Valkey nor any vector store provider found.
Action: implement from scratch following framework conventions for vector stores.

### 2.3 — Valkey Search Use Case Analysis

Regardless of implementation path, analyze these use cases specific to **this codebase**:

1. **Semantic search over existing data** — does the repo have document ingestion, embeddings, or a RAG pipeline? If yes, Valkey Search adds low-latency vector retrieval to an existing data flow.
2. **Session / conversation memory** — does the repo manage chat history or agent memory? Valkey's TTL + vector search enables semantic memory retrieval without a separate vector DB.
3. **Real-time filtering + vector search** — does the repo filter results by metadata (user ID, date, category)? Valkey's hybrid search (tag/numeric filters + KNN) outperforms pure vector databases for this.
4. **Caching embeddings** — are embedding calls expensive or repeated? Valkey can cache embedding vectors to avoid recomputation.
5. **Multi-modal or multi-tenant indexing** — are there multiple document types or tenants? Multiple RediSearch indexes map cleanly to this pattern.

For each use case, mark as **Applicable**, **Potentially applicable**, or **Not applicable** based on codebase evidence, with one sentence of justification.

### 2.4 — PR Acceptance Feasibility

Research whether the target repository would realistically accept a Valkey Search PR:

1. **Search recent PRs and issues** — look for merged PRs that added new integrations, vector store backends, or similar extensions. Note the maintainer who approved, what was required (tests, docs, examples), and turnaround time.
2. **Check for competing/rejected PRs** — search for closed-without-merge PRs that attempted Redis, Valkey, or other vector store integrations. Note rejection reasons.
3. **Identify contribution requirements** — look for `CONTRIBUTING.md`, PR templates, required CI checks, documentation sync requirements (e.g., translations), and any explicit extension points (plugin registries, factory patterns).
4. **Assess community signal** — are there open issues requesting vector search, Redis/Valkey support, or performance improvements that Valkey would address?

Synthesize into a confidence level (HIGH / MEDIUM / LOW) with evidence.

### 2.5 — Produce Final Report

Output the following report in full. Do not truncate.

---

```
# Valkey Search Compatibility Report
Repository: <path analyzed>
Date: <today>
Framework: <detected>    Language: <detected>

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 1. Existing Redis / Valkey Integration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Status: FOUND | NOT FOUND

<If found: list files, libraries, versions, and whether vector search (VSS)
commands are already present. If vector search IS already used, note what
index types and query patterns are in use.>

<If not found: confirm no Redis/Valkey dependency anywhere in the project.>

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 2. Existing Vector Store Implementations (Blueprint Candidates)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Providers found: <list or NONE>

<For each provider: file path, class name, methods present, index/embedding pattern.>

Best blueprint: <provider + file> OR none — greenfield implementation required.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 3. Recommended Implementation Path
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Path: A (Extend) | B (Port from Blueprint) | C (Greenfield)

### Implementation Overview

<Concise but specific description of what needs to be built. Include:>
- Which file(s) to create or modify
- Class/module name to match framework conventions
- Constructor parameters (connection URL, index name, embedding function)
- Index creation: HNSW vs FLAT, distance metric, vector dimension source
- Core methods to implement and their signatures in this project's language
- How to wire it into the existing retrieval/agent pipeline
- Any auth or config values needed (env var names already in use)

### Key Implementation Steps

1. <Step with file name>
2. <Step>
3. <Step>
4. <Validation step: how to confirm it works — a test call, CLI command, or existing test pattern to follow>

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 4. Use Cases: Valkey/Redis vs Other Providers
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

| Use Case                          | Applicability         | Justification |
|-----------------------------------|-----------------------|---------------|
| Semantic search / RAG             | <level>               | <one sentence> |
| Session / conversation memory     | <level>               | <one sentence> |
| Hybrid filter + vector search     | <level>               | <one sentence> |
| Embedding cache                   | <level>               | <one sentence> |
| Multi-index / multi-tenant        | <level>               | <one sentence> |

### When to Prefer Valkey/Redis Over Other Providers

<3–5 specific reasons grounded in what was found in this codebase. Focus on
operational advantages (single infra, latency, TTL, atomic operations, existing
Redis usage) rather than generic marketing claims.>

### When Another Provider May Be Better

<Honest 2–3 sentence assessment. E.g., if the codebase already has deep
Pinecone integration with no Redis anywhere, note the switching cost.>

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 5. Validation Checklist
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Before marking implementation complete:

- [ ] Valkey/Redis connection established and pinged
- [ ] Index created with correct vector dimension and distance metric
- [ ] At least one document ingested (add_texts / add_documents)
- [ ] similarity_search returns expected results
- [ ] Hybrid filter query tested (if applicable)
- [ ] Existing tests pass (no regressions)
- [ ] Framework integration confirmed (as_retriever / chain tested end-to-end)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 6. PR Acceptance Feasibility
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Confidence: HIGH | MEDIUM | LOW

### Evidence

<List concrete evidence: merged PRs that set precedent, maintainer patterns,
official extension points, contribution requirements found.>

### Positioning Strategy

<How to frame the PR for maximum acceptance likelihood. What angle resonates
with maintainers based on observed patterns.>

### Non-Negotiable Requirements

<List specific requirements the PR must meet based on precedent (tests, docs,
translations, examples, CI checks, etc.)>

### Risks & Blockers

<Honest assessment of what could get the PR rejected or stalled. Include
technical concerns maintainers are likely to raise and how to preempt them.>
```
