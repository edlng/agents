# Obsidian Guide Contract

Use portable Markdown that renders in a default Obsidian installation and in
other common Markdown readers.

## Main-Note Frontmatter

Use exactly these fields for the main note only:

```yaml
---
title: "<repository-name> Onboarding"
repository: "<absolute-path>"
analyzed_commit: "<full-commit-id>"
generated: "YYYY-MM-DD"
confidence: "<high|medium|low>"
tags:
  - onboarding
  - architecture
---
```

## Subsystem-Note Frontmatter

Use exactly these fields for every adaptive subsystem note:

```yaml
---
title: "<repository-name> - <subsystem> Onboarding"
repository: "<absolute-path>"
analyzed_commit: "<full-commit-id>"
generated: "YYYY-MM-DD"
confidence: "<high|medium|low>"
parent: "[[Onboarding]]"
tags:
  - onboarding
  - architecture
  - subsystem
---
```

## Main Note Order

Keep the main note in this order:

1. At a glance
2. Internal navigation
3. Purpose
4. Run and verify
5. Architecture and flows
6. Repository map
7. Concepts
8. Subsystems and interfaces
9. Data, state, configuration, and dependencies
10. Errors, observability, security, and operations
11. Tests and verification
12. Reading order
13. Concerns, contradictions, and unknowns
14. Evidence index and metadata

## At A Glance

The opening summary must identify the repository and analyzed commit, explain
its purpose and primary users, name the proven languages and frameworks, list
entry points and major services, summarize the principal execution paths and
state boundaries, give the shortest current run or verification path, and
state overall confidence plus the most consequential concern or unknown.

The main note is the navigation hub. Follow the summary with compact internal
navigation linking to every main section. Before linking any note, including
each adaptive note, summarize that note's purpose, interfaces, and principal
flow so readers can orient themselves without opening it. A compact table with
purpose, interfaces, flow, and note link is preferred when several linked
notes exist.

## Semantic Callouts

Use callouts to communicate evidence and risk, not decoration:

```markdown
> [!abstract] At a glance
> A compact repository summary.

> [!success] Verified
> Observed behavior, including the command and result.

> [!info] Source-backed
> A claim supported by a cited current file or document.

> [!example]- Verification log
> Command, scope, exit status, relevant output, time, and limitations.

> [!warning] Concern
> A concrete contradiction, security issue, or operational risk.

> [!question] Unknown
> What remains unresolved, why, and how it could be established safely.
```

Do not use decorative callouts, nested cards, walls of badges, custom CSS,
Dataview, or community plugins.

## Evidence Index

End with a compact index that lets a reader audit material claims:

| ID | Claim or artifact | Status | Evidence | Location or command | Notes or limitations |
| --- | --- | --- | --- | --- | --- |
| E-01 | `<claim>` | Verified | `<observed result>` | `<command or path>` | `<scope>` |

Use **Verified**, **Source-backed**, **Inferred**, or **Unknown** exactly. Link
diagram nodes and edges, verification logs, security observations, and
important architectural claims to index IDs. Metadata must also disclose the
analyzed commit, generation date, repository status before and after, tools
used, diagram renderer, and persistence result.

## Adaptive Linked Notes

Create `Onboarding - <Subsystem>.md` only when a subsystem has enough distinct
responsibility, interfaces, flows, or evidence to make the main note hard to
scan. Keep it beside `Onboarding.md`, link it from internal navigation and the
relevant main section, and link back to the main note. Give it enough context
to stand alone, use the same evidence labels and index IDs, and avoid copying
large sections from the main note. Keep detailed subsystem diagrams with that
note; keep the high-level architecture diagram in the main note.

## Chat Output

Include the main repository orientation and architecture explanation, a
verification summary with observed outcomes and limitations, important
concerns and unknowns, confidence, and every generated note and asset path.
Also report the target repository and commit and any unexpected repository
changes.

For an ordinary repository, chat may contain the full portable main note. For
a very large repository, chat may contain a readable condensed guide only when
the exhaustive guide and subsystem details were persisted in Obsidian; list
all persisted paths. When Obsidian tools are unavailable, return the complete
portable Markdown in chat and explicitly say that persistence did not occur.
Never infer or guess a vault path, and never claim a note or asset exists
without observing persistence.
