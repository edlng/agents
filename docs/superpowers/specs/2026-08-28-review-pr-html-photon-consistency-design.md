# PR Review HTML Report and Photon Consistency Design

## Objective

Update the shared PR review workflow so `review-pr` and
`review-cookbook-pr` produce a temporary, self-contained HTML report instead
of an Obsidian note. When the reviewed repository is exactly
`awslabs/photon`, the workflow must also compare changed behavior with other
Photon clients and report evidence-backed alignment or divergence.

The review remains read-only on GitHub and continues to present its findings
in chat.

## Ownership and Scope

The behavior belongs in `_shared/pr-review-base.md` because both PR review
skills use that file as their single source of truth. The thin skill wrappers
will be updated only where their descriptions or summaries still mention
Obsidian or the old report behavior.

`_shared/no-github-writes.md` will describe local HTML output without implying
that review skills save Obsidian notes.

The change applies to:

- `review-pr`
- `review-cookbook-pr`
- `_shared/pr-review-base.md`
- `_shared/no-github-writes.md`

The canonical copies are authored under `~/.kiro/skills/` and replicated
byte-for-byte to the other four skill roots. Missing target directories are
created so the five roots finish in sync.

## Photon Detection and Evidence Collection

The Photon-specific phase runs only when the normalized GitHub repository name
is `awslabs/photon`. It does not run for downstream repositories that use
Photon.

After the requirements and codebase context are built, the workflow identifies:

1. The client or clients changed by the PR.
2. The externally observable behavior affected by each meaningful change.
3. Other Photon clients that implement the same behavior.

The reviewer inspects implementations on the repository's `main` branch
first, even if the reviewed PR targets another branch. It may then inspect
open pull requests only when their title, body, changed paths, or diff clearly
concern the same behavior. It must not scan unrelated open pull requests or
treat a merely similar keyword as evidence.

Each comparison record contains:

- Compared client.
- Behavior or contract being compared.
- Source type: `main` branch or relevant pull request.
- Source ref or pull request number.
- Repository path and line or line range.
- Stable GitHub URL when one can be formed.
- Observed behavior.
- Verdict: `ALIGNS`, `DIVERGES`, `MIXED`, or `INSUFFICIENT_EVIDENCE`.
- Reasoning that connects the evidence to the verdict.

Absence of an implementation is not divergence unless the shared contract
requires every client to implement it. Unavailable files, ambiguous behavior,
or contradictory evidence produce `INSUFFICIENT_EVIDENCE`, not a guess.

The evidence set is cached under
`pr:$RUNID:photon_client_consistency` and is available to the initial reviewer,
validator, chat report, and HTML report.

## Findings and Validation

For Photon reviews, each finding includes a consistency section after its
normal claim, evidence, reasoning, and suggested fix. That section states the
cross-client verdict and cites the comparison records that support it.

A finding may still be valid when cross-client evidence is insufficient.
Cross-client divergence is supporting context, not a substitute for proving
the underlying defect against the PR's requirements and code.

The validator checks that:

- The stated client behavior is supported by the cited path and lines.
- Relevant pull request evidence is actually about the same behavior.
- The verdict follows from the evidence.
- The report does not generalize from one client to all clients.

Unsupported consistency conclusions are changed to
`INSUFFICIENT_EVIDENCE`; the underlying finding is rejected or downgraded only
under the existing skeptic-pass rules.

## HTML Report

The report is one portable HTML file written to:

`/tmp/pr-review-<repo>-<number>-<timestamp>.html`

It contains no remote fonts, scripts, stylesheets, images, or other runtime
dependencies. CSS and any small enhancement script are inline. All PR text,
code, paths, author names, Jira content, and finding text are HTML-escaped
before interpolation.

The selected evidence-dashboard hierarchy is:

1. PR title, metadata, review action, and severity counts.
2. What the PR does and the requirements it addresses.
3. Change map grouped by subsystem or directory.
4. Validated findings ordered by severity.
5. Test coverage and notable untested behavior.
6. Photon client consistency matrix when applicable.
7. Review methodology and source links.

Each finding shows its location, claim, concrete evidence, reasoning,
suggested fix, and Photon consistency conclusion when applicable. The
consistency matrix links each verdict to `main` branch lines or relevant pull
requests.

The report uses a restrained, high-contrast visual system suitable for a
technical review: compact typography, clear severity accents, stable
two-column desktop layout, and a single-column mobile layout. It must remain
fully readable with JavaScript disabled and when opened directly from disk.

After generation, the workflow opens the report in the default browser when
the environment supports it. Otherwise, it provides the absolute file path
and a clickable local file link. The file remains under `/tmp` until normal
system cleanup.

## Chat Output

The assistant still leads with findings ordered by severity. For each Photon
finding it provides:

1. The finding and evidence.
2. The reasoning.
3. The consistency verdict.
4. The client evidence and relevant pull request links.

The final chat response also summarizes what the PR does, the recommended
review action, and the HTML report path. It does not claim that an Obsidian
note was created.

## Failure Handling

- If no linked Jira issue exists, use the PR description as already specified.
- If Photon comparison evidence cannot be fetched, continue the review and
  mark the affected comparisons `INSUFFICIENT_EVIDENCE`.
- If a relevant open pull request cannot be read, cite its identity and state
  that its behavior could not be verified.
- If browser launch fails, keep the generated report and provide its path.
- If HTML generation fails, still present the complete findings in chat and
  report the local-output failure explicitly.
- No failure path permits a GitHub write or an Obsidian fallback.

## Verification

Static verification will confirm:

- No affected review skill instructs the agent to write an Obsidian note.
- The Photon gate names only `awslabs/photon`.
- Evidence requirements cover `main` branch code and relevant open PRs.
- Every consistency verdict requires concrete source evidence.
- The report path is under `/tmp` and the HTML is self-contained.
- Chat output remains mandatory.
- GitHub writes remain prohibited.
- All five copies of each changed skill or shared reference are byte-identical.

A representative generated report should also be checked for valid structure,
escaped untrusted content, responsive layout, and readable no-JavaScript
behavior.
