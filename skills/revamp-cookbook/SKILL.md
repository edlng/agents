---
name: revamp-cookbook
description: Use when refreshing an existing Valkey cookbook from archive/main-pre-reorg to current CONTRIBUTING.md guidance, checking for duplicate or overlapping cookbook PRs, adding runnable validation, or preparing local draft materials for language-specific CI.
---

# Revamp Cookbook

## Overview

Update one existing Valkey-Samples cookbook into a locally reviewable draft.
The source is the original cookbook on `archive/main-pre-reorg`; the
acceptance contract comes from the current CONTRIBUTING.md guidance, including
any open contributing-guidelines PR.

The first deliverable is a validated local diff and draft description. Do not submit it for final review until the RFC, contributing guidance, cookbook content, and language CI are settled.

The archived cookbook is the baseline. Preserve its learning progression,
examples, wording, and layout, and add only changes required by the current
contribution contract, correctness fixes, dependency/runtime updates, or
runnable validation. Record every intentional deviation from the archived
files.

## Non-Negotiable Rules

- Check all repository PRs before selecting work. Do not create duplicate cookbook, regeneration, or CI PRs.
- Treat an existing PR for the same cookbook as the canonical owner. Report the overlap and coordinate changes into it; never open a second draft for the same target.
- Treat existing language CI PRs as shared infrastructure. Reuse a workflow
  already merged on the current base; if it exists only in an open PR, copy it
  locally with a removal marker instead of creating a parallel remote workflow.
- Keep all work local. Never run `git commit` or `git push`; never create or edit a GitHub PR, submit a review, or change remote branches.
- Report suggested changes for an existing cookbook or CI PR instead of modifying that PR.
- Keep the local draft clearly unsubmitted. Do not request final review or represent unverified work as ready. Until the contributing-guidelines PR is finalized, all cookbook and CI work is draft-only.
- Do not claim that an example, dependency, Docker tag, link, or CI job was validated unless the command actually ran and its result is recorded.
- Cookbook prose must demonstrate the integrated project's API. Do not replace project calls with raw Valkey commands or comments that simulate them.
- Start from the exact files in `archive/main-pre-reorg`; do not rewrite an archived cookbook wholesale.
- Complete an archive-fidelity review before any runtime testing. A read-only validator subagent may perform the comparison.
- Make exhaustive fenced-code-block execution the final technical validation step. Run every Markdown code block, including shell setup, application code, configuration, verification, and teardown.
- Treat the implementation contract, lifecycle safety, clean-clone run, and CI
  matrix checks below as hard gates. A draft with an unresolved hard-gate
  failure is not validated; record it as blocked or revise the draft.
- Preserve unrelated user changes and existing repository conventions.

## Phase 1: Repository and PR Preflight

Identify the repository:

```bash
git remote -v
```

For `valkey-io/Valkey-Samples`, inventory every PR, not only open PRs.
Paginate through the repository's pull-request list and inspect titles, bodies,
head branches, changed cookbook names, and related CI references. Search terms
should include:

- `cookbook`, `archive/main-pre-reorg`, `re-introduces`, `regenerat`, `CONTRIBUTING`
- the target framework/use case and its directory name
- `ci`, `validation`, and the target language
- `security`, `timeout`, `TTL`, `retention`, `socket_timeout`, `loopback`, and the integration's public API name

Classify each relevant result:

| Result | Action |
| --- | --- |
| Existing PR updates the same cookbook | Stop duplicate work; use that PR as the owner and report its URL, branch, author, and overlap. |
| Existing PR owns CI for the target language | Reuse or extend it; do not create a parallel workflow. |
| Existing PR is a related contributing-guidelines or RFC change | Read it as an input contract and record its PR number in the draft description. |
| No overlapping cookbook PR | Continue with a local diff and draft PR materials; do not create the remote PR. |

For every related PR, inspect review threads and check runs, not only the PR
body and diff. Treat recurring review findings as acceptance-contract input.
Recent cookbook reviews identified hard-gate classes that basic CI may miss:
loopback-only ports, bounded client and download timeouts, closed clients on
all test paths, TTL or retention guidance for persistent data, complete
configuration tables, honest minimum runtime and Valkey versions, actual
matrix interpolation, workflow-file path filters, and behavior-focused
assertions.

Known examples in this repository are useful signals, not permanent assumptions:

- PR #40: contributing-guidelines acceptance criteria and review process.
- PR #42: archived Node/TypeScript cookbook regenerated under the new guidance.
- PR #43: Node/TypeScript cookbook validation CI.
- PR #44: archived LangChain4j cookbook regenerated with Java CI.
- PR #45: archived semantic-caching cookbook regenerated with Python CI.

If the target already matches one of these or another open PR, do not regenerate it a second time.

## Phase 2: Select and Inspect the Source

Inputs may be a cookbook path, framework/use-case name, PR URL, or no explicit target.

Fetch the relevant refs without changing the user's work:

```bash
git fetch origin main archive/main-pre-reorg
```

When no target is supplied:

1. List cookbook directories on `archive/main-pre-reorg`.
2. Remove targets with an existing cookbook or regeneration PR.
3. Prefer a target with a runnable example and a language covered by existing CI. If the target language is not TypeScript/Node.js, Python, or Java, plan a new path-filtered CI workflow in the local draft.
4. If several targets are equally suitable, state the candidates and choose one conservatively; do not silently overwrite an owned target.

Compare the archived target with current `main` and read its README, numbered pages, metadata, examples, dependency files, and any setup scripts. Record:

- The cookbook category and directory.
- The framework/use case and the Valkey feature demonstrated.
- Language, runtime, package manager, and dependency versions.
- Whether the archived code calls the integrated project's API or merely mirrors Valkey internals.
- Which examples can run without paid credentials, GPUs, or private source branches.
- Which runtime capabilities are required by the integration (modules, minimum
  Valkey version, Python/runtime version, model service, or network access).
- Which package APIs are public and which implementation attributes are
  private, version-coupled, or unsupported.

Before editing, create a read-only archive snapshot of the exact target files:

```bash
archive_snapshot="$(mktemp -d)"
git archive archive/main-pre-reorg -- "$target" | tar -x -C "$archive_snapshot"
find "$archive_snapshot/$target" -type f -print | sort
```

If the target already exists in the working tree, compare it with the
snapshot and preserve unrelated user changes. If it does not exist, copy the
snapshot into the working tree as the starting point. Do not delete or
overwrite user changes to reconcile the baseline silently.

## Phase 3: Extract the Current Acceptance Contract

Read the current `CONTRIBUTING.md` from `main`. If the latest
contributing-guidelines change is still in a PR, also read that PR's head
branch and use its proposed requirements for the draft. Clearly label
requirements that are still pending.

Build a checklist before editing. At minimum, check for:

- Clean-clone setup with pinned, available dependencies.
- Stable or explicitly justified `valkey/valkey-bundle` tag.
- Runnable examples and a deterministic CI/mock path for paid services.
- No private links, internal paths, or uncited performance/marketing claims.
- Vendor-neutral defaults; vendor-specific integrations as optional addenda.
- Audience statement, prerequisites, security guidance near the first connection, configuration reference, and teardown.
- Correct cookbook navigation and index entries.
- Metadata only if the current repository still consumes it.
- Markdown lint and link-check compatibility.

Add a source-fidelity checklist containing the archived file list, the working file list, and each planned deviation. Do not begin runtime testing until this checklist has a verdict.

Do not cargo-cult requirements from an older PR. Verify them against the current branch, the contributing-guidelines PR, and the repository's actual build/rendering pipeline.

### Implementation Contract Gate

Before editing, create a compact contract table in the draft notes. Every row
must have a source and a validation command or test:

| Contract area | Required evidence |
| --- | --- |
| Runtime | Package metadata or lockfile supports the documented Python/runtime version; use the lowest version the sample actually supports. |
| Valkey | Pinned image exists; required modules and minimum server version are documented; test every supported version in the intended CI matrix. |
| Configuration | Every environment variable, constructor option, URL query parameter, model name, and default used in code appears in a configuration table or is explicitly internal. |
| API boundary | Learner-facing examples call the integration's public API. Private attributes require a version pin, an inline warning, a reason, and a narrowly scoped test. |
| Connectivity | Local services bind to loopback; clients have bounded connect/read/request timeouts; credentials and TLS guidance are present for non-local connections. |
| Lifecycle | Clients, indexes, keys, containers, temporary files, and model services are cleaned up in `finally`/fixtures or an equivalent failure-safe path. |
| Retention | Persistent session, cache, or memory data has TTL/retention guidance and a cleanup strategy. |
| CI | Every matrix variable is interpolated into paths and commands; path filters include all inputs that affect the job, including workflow and Compose files; every job and external download has a timeout. |
| Documentation | Relative links, manifests, generated-output references, JSON/config blocks, and documented commands resolve or are explicitly marked illustrative. |

Do not begin implementation until each contract row is either satisfied or
marked as a hard blocker with an owner and follow-up. Do not silently weaken a
row because an existing PR passed CI.

## Phase 4: Implement the Revamp

Work from current `main` or the user's existing local branch and update only the selected cookbook plus required indexes and validation files. Do not create commits or push the changes.

For each page:

1. Preserve the archived page as the starting point and keep its learning progression and useful Valkey explanations.
2. Add the required audience, prerequisites, security, configuration, and teardown content.
3. Replace stale, vendor-locked, or untestable defaults with local deterministic alternatives where practical.
4. Keep optional provider/API-key paths clearly separated from the default path.
5. Keep code examples aligned with the integrated project's public API.
6. Keep raw Valkey commands in explanatory prose or verification tests only; do not use them to impersonate framework calls.
7. Make navigation, titles, leads, and metadata consistent.
8. Update the source-fidelity checklist with the reason for every changed or added file.

Apply these implementation safeguards:

- Bind unauthenticated local services to `127.0.0.1`, not all interfaces.
- Set explicit connection, read, request, and subprocess timeouts whenever the
  sample accepts a host, port, URL, model service, or external provider.
- Use failure-safe cleanup. Tests must close clients and remove sample data
  even when an assertion or API call fails. Prefer fixtures or `try/finally`
  over cleanup placed only after assertions.
- Assert the behavior the cookbook teaches: persistence, retrieval quality,
  filtering, isolation, resume behavior, environment overrides, and cleanup.
  Avoid exact internal counts or message shapes unless they are the public
  contract; prefer semantic assertions and minimum bounds.
- Do not use an internal framework attribute merely to make a mock or cleanup
  path work. If no public API exists, isolate the compatibility shim, pin the
  dependency, document the coupling, and add a focused regression test.
- Avoid duplicating a code block when a reference or shared setup is enough.
  Every example must either be self-contained or explicitly declare the state
  inherited from an earlier step.
- Keep the documented configuration table synchronized with the code. Add a
  test or static check that fails when a required documented override is
  ignored or when the table omits a used option.

Add or repair a runnable `sample/` (or the repository's established equivalent) that:

- Installs exact or policy-compliant dependency versions.
- Starts Valkey with a health check.
- Runs the same project-level calls shown in the cookbook.
- Uses deterministic fixtures or mocks for paid services and unavailable models.
- Is idempotent and cleans up keys, indexes, clients, and containers.
- Exits non-zero on failed assertions.
- Uses explicit socket/request timeouts and closes every client it creates.
- Provides retention guidance for data that can grow without bound.
- Does not expose an unauthenticated service beyond loopback by default.
- Tests cleanup after both success and failure paths.

## Phase 5: Coordinate CI by Language

Determine the language from the actual sample files and package manager, not from the framework name.

Existing CI ownership in this repository currently includes:

- PR #43: TypeScript/Node.js cookbook validation.
- PR #44: Java cookbook and Java validation.
- PR #45: Python cookbook and Python validation.

For TypeScript/Node.js, Python, or Java:

1. Inspect the owning PR's workflow, including path filters, dependency installation, lint/build steps, Valkey image, health check, test command, and version matrix.
2. If the workflow is already merged on the current base, reuse it and do not copy or add a duplicate.
3. If the workflow exists only in an open PR, copy it into the local draft, add a clear `TODO` comment to remove the temporary copy after the owning PR merges, and reproduce its commands locally.
4. Record the exact commands and results. If a workflow step cannot run locally, record the blocker rather than claiming the workflow passed.
5. Validate the workflow structurally with `actionlint` and inspect the
   expanded matrix. Confirm that each `matrix.*` value appears in
   `working-directory`, cache paths, test file paths, service configuration,
   or other job inputs. Confirm that `paths-filter` includes every relevant
   source, dependency, Compose, workflow, and configuration path.
6. Add `timeout-minutes` to every job and a bounded timeout to model pulls,
   package setup, and other external-service bootstrap steps.

For Go, Rust, or another language without an existing shared workflow:

1. Search all PRs for a language-CI owner before adding files.
2. If no merged or open-PR workflow exists, add the smallest path-filtered build and integration-test workflow to the local draft.
3. Use a pinned `valkey/valkey-bundle` image, a health check, dependency installation, the language's build/test command, and cleanup.
4. Keep that CI addition draft-only and unsubmitted while the contributing-guidelines PR is pending.

For every target language, inspect both the current base and relevant open PRs before deciding. Use this decision table:

| Workflow state | Action |
| --- | --- |
| Language workflow is already merged on the current base | Reuse it; do not copy or add a duplicate. |
| Language workflow exists only in an open PR | Copy the workflow into the local draft, add a clear `TODO` comment to remove the temporary copy after the owning PR merges, and run its commands locally. |
| No merged or open-PR workflow exists | Add the smallest path-filtered workflow to the local draft and run its commands locally where possible. |

Record the workflow owner, PR number, merge state, local path, and removal
decision. Never modify a remote CI PR, create a second workflow for a language
whose workflow is already merged, or push a branch. The cookbook and any
copied or new non-owned-language CI should remain locally reviewable and
clearly marked as draft work.

## Phase 6: Review Archive Fidelity Before Testing

Run this phase after implementation and before any sample, CI, or snippet execution.

1. Compare the archived snapshot with the working cookbook, including file lists and content:

   ```bash
   diff -ru "$archive_snapshot/$target" "$target" || true
   ```

2. Review every difference against the source-fidelity checklist and the PR #40 acceptance contract.
3. If a validator subagent is available, give it read-only access to the archive snapshot and working target and ask it to return:
   - a `PASS` or `REVISE` verdict;
   - changes that are broader than required;
   - missing required changes; and
   - a per-change justification or narrowing recommendation.
4. Narrow unjustified changes before continuing, or record why each broader change is necessary.
5. Do not call the cookbook or claim runtime validation from this phase. Record the verdict and the final intentional-deviation list in the draft notes.

## Phase 7: Validate Before Opening a Draft

Run the narrowest complete checks from a clean or isolated environment:

```bash
# Examples; use the project's actual commands.
markdownlint-cli2 '**/*.md'
lychee --config lychee.toml '**/*.md'
npm ci && npm test
python -m pytest -q
mvn verify
go test ./...
```

Also verify:

- The archive-fidelity review passed, or every `REVISE` item has been corrected or explicitly justified.
- The workflow ownership decision was recorded. Any workflow copied from an open PR contains a removal `TODO` and its owning PR is named.
- The sample starts against the pinned Valkey image.
- For TypeScript/Node.js, Python, or Java, the relevant existing CI workflow was inspected and its runnable steps were reproduced locally, with blocked steps recorded.
- For another language, the new local CI workflow was inspected and its commands were run locally where possible.
- Every documented command and path exists.
- Dependency versions resolve.
- Docker image tags exist when a specific tag is used.
- No credentials or local absolute paths leaked into the diff.
- Index/readme links point to the new or updated cookbook.
- Every implementation-contract row has executable evidence or an explicitly
  recorded hard blocker.
- Tests cover failure-safe client cleanup, environment overrides, idempotence,
  and the primary behavior demonstrated by the cookbook.
- All local service ports are loopback-bound unless authentication is enabled.
- Configuration tables include every variable and option used by the sample.
- No unsupported private API or orphaned HTML/manifest reference remains
  unexplained.

### Clean-Clone Gate

Run the sample from a fresh temporary worktree or clean-clone-style directory,
not only from the edited checkout:

1. Copy or check out the exact draft files into a temporary directory.
2. Follow the documented setup commands literally, using only documented
   environment variables and prerequisites.
3. Start the pinned services, install dependencies, run the documented sample
   and tests, and execute teardown.
4. Remove the temporary environment and confirm the documented commands did not
   depend on an untracked local file, an already-running service, a cached
   dependency, or hidden shell state.

If a true clean-clone run is impossible, mark the gate `BLOCKED` and explain
why. A successful run from the existing checkout is not a substitute.

### Behavioral and Lifecycle Gate

Before the final snippet pass, run focused tests that prove:

- the main integration behavior and the result asserted in the walkthrough;
- environment overrides are actually propagated, using explicit patched values;
- clients are closed and sample resources are removed after failures;
- a second run is idempotent after a successful run and after a partial setup;
- scoped data cannot cross user/session/tenant boundaries where applicable;
- retention or TTL behavior is documented and, when implemented, tested.

Treat a test that only checks call completion, exact unstable SDK counts, or
the existence of a response without checking its content as insufficient.

As the final technical validation step, enumerate and run every fenced code
block in every Markdown file in the selected cookbook, in documented order.
Include shell setup and teardown commands, application-language snippets,
configuration examples, and verification commands. Use the documented
container runtime with the pinned `valkey/valkey-bundle` image, wait for the
container health check, preserve required state between sequential snippets,
and clean up containers and other resources afterward. Prefer loopback-bound
ports. Fail on every unexpected non-zero exit.

Before execution, classify each block as:

- **Runnable:** execute the exact block in documented order.
- **Illustrative:** parse or compile it and verify referenced names/configuration;
  do not claim runtime execution.
- **Credential/model gated:** do not invoke an external provider with ambient
  credentials. Run a deterministic equivalent or syntax/import check and
  record why the original was not executed.
- **Configuration/data:** parse JSON, YAML, TOML, or other structured content
  with a parser; do not count a visually valid block as validated.

Reject hidden state: a block that references variables, clients, files,
containers, or services not created by the current block or an explicitly
recorded predecessor must be fixed or marked illustrative. Record each
block's file, line or sequence number, language, exact command, classification,
exit result, and limitation.

If a check cannot run, say why and label the limitation. Never convert an unrun check into a passing claim.

## Phase 8: Prepare Draft PR Materials

Search for the repository's PR template:

```bash
find .github -iname 'pull_request_template.md' -o -path '*/PULL_REQUEST_TEMPLATE/*'
```

Use the template if present. The draft description must include:

- Source cookbook and `archive/main-pre-reorg` provenance.
- Archive-fidelity verdict and the complete intentional-deviation list.
- Contributing-guidelines PR/branch used.
- Summary of changes from the archived version.
- Framework/use case, Valkey feature, language, and sample location.
- Exact validation commands and results.
- Related cookbook and CI PRs, including ownership decisions.
- Any workflow copied from an open PR, its removal `TODO`, and the owning PR.
- Per-fenced-block execution results, including blocked or intentionally non-executable blocks.
- Known limitations, pending RFC/guideline decisions, and required follow-up.

After the duplicate scan and validation pass, prepare the proposed draft title,
body, file list, test evidence, and follow-up notes locally. Do not create or
update the remote PR. If an overlapping PR exists, report the canonical owner
and provide the handoff notes instead.

Before finishing, confirm that no write operation was attempted:

- No `git commit`.
- No `git push`.
- No branch push or remote branch mutation.
- No PR creation, PR edit, review request, or review submission.

## Output Checklist

Report:

- Target cookbook and archive path.
- Existing PRs checked and any overlap found.
- Files changed and whether metadata/indexes changed.
- Validation commands with pass/fail/blocked status.
- CI ownership decision: existing TypeScript/Node.js, Python, or Java workflow
  reused and run locally, or a new non-owned-language CI workflow added to the
  local draft with its validation evidence. Do not modify remote CI PRs.
- Proposed draft PR title/body and local file list; no new PR URL is expected.
- Canonical existing PR URL when work was consolidated.
- Explicit items that must wait for RFC/contributing-guidelines finalization.

## Red Flags

Stop and re-check the preflight if any of these occur:

- "We can check duplicate PRs later."
- A second PR would add the same cookbook directory.
- A second CI workflow covers paths already owned by PR #43, #44, or #45.
- The relevant existing CI workflow was not inspected and reproduced locally before claiming validation.
- A new language CI workflow is treated as merge-ready while the contributing-guidelines PR is still pending.
- The working cookbook is substantially rewritten without an archive comparison and per-change justification.
- Runtime testing begins before the archive-fidelity review is complete.
- The final report says every snippet passed without an inventory and per-snippet execution record.
- The draft says "tested" but only syntax or manual inspection ran.
- A README example calls `valkey-cli` instead of the framework/project API.
- A specific Docker tag or package version was copied without verifying availability.
- An open contributing-guidelines PR is treated as merged policy.
- Someone suggests committing, pushing, creating/editing a PR, or submitting review from this workflow.

## Common Mistakes

| Mistake | Correction |
| --- | --- |
| Choosing an archived cookbook already being regenerated | Inventory all PRs first; consolidate with the canonical owner. |
| Adding duplicate CI for TypeScript/Node.js, Python, or Java | Inspect the owner workflow and reproduce its relevant steps locally; add no CI file to the draft. |
| Choosing Go, Rust, or another language without CI | Add a small path-filtered CI workflow to the local draft and run its commands locally where possible. |
| Copying a workflow from an open PR without a removal marker | Add a `TODO` naming the owning PR and remove the temporary workflow after that PR merges. |
| Rewriting the archived cookbook before comparing it | Restore the archive baseline, apply the smallest required patch, and document each deviation. |
| Running only the sample entry point | Inventory and execute every fenced code block, including setup, verification, and teardown. |
| Leaving vendor-specific setup as the default | Use local deterministic defaults and document optional providers separately. |
| Keeping `meta.json` automatically | Confirm whether the current renderer consumes it. |
| Using raw Valkey commands as a stand-in for framework calls | Call the project's API in prose examples; reserve raw commands for verification or explanatory prose. |
| Calling an unrun sample "validated" | Record the blocker and keep the local draft unsubmitted. |
| Binding an unauthenticated service to all interfaces | Bind published ports to `127.0.0.1` and document TLS/authentication for non-local use. |
| Letting a client or model pull wait indefinitely | Add explicit client and subprocess/network timeouts and a bounded CI job timeout. |
| Cleaning up only after assertions | Use fixtures or `try/finally` so clients, keys, indexes, and services are removed on failure. |
| Asserting only exact SDK-internal counts | Assert the demonstrated behavior and use stable minimum/semantic assertions. |
| Testing defaults instead of overrides | Patch explicit environment values and inspect the resulting client/model configuration. |
| Documenting a setting only in code | Add every used variable and option to the configuration reference and validate the table against code. |
| Using private framework attributes silently | Isolate, pin, label, and test the compatibility shim, or replace it with a public API. |
| Trusting a CI matrix declaration | Run `actionlint` and inspect that matrix values actually drive paths, commands, and services. |
| Triggering on a workflow path but filtering it out | Include workflow, Compose, dependency, and configuration files in both event paths and `paths-filter`. |
| Claiming a clean-clone result from a dirty checkout | Run literal setup commands in a fresh temporary worktree and remove its environment afterward. |
