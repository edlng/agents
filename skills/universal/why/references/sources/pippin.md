# Pippin Docs

## What this source contains

- Design documents and technical specs
- Review comments left on those documents
- Project folders that group a feature's docs together
- Meeting notes and decision records saved as artifacts

Pippin is where "why" often lives in long-form before it becomes code. A significant feature usually has a design doc.

## How to search it

Use the Pippin MCP read tools.

1. **Keyword searches with `pippin_search`.** Try:
   - The feature name
   - Key symbols and class names from the target code
   - Author handles from the PR (design docs are often written before the code lands)
   - Error strings or user-visible terms
2. **Fetch candidate artifacts with `pippin_get_artifact`.** Read the full content, not the preview. Rationale is often buried mid-document. Pass a Pippin link as `url` instead of splitting it into ids.
3. **Read the review comments with `pippin_get_artifact_comments`.** Reviewers often ask "why not X?" and the author's reply is the rationale.
4. **Walk the project folder.** Use `pippin_get_project`, `pippin_list_artifacts`, and `pippin_get_folder_children` to find sibling docs: alternatives considered, appendices, follow-up designs.

## What good evidence looks like here

- A design doc with a "Problem", "Motivation", or "Tenets" section that matches the target code's purpose
- An "Alternatives considered" or "Rejected approaches" section
- A review comment thread that settles the decision the code reflects
- A decision record tied to the same author and date range as the PR

## Common pitfalls

- **Outdated docs.** Designs are often written before implementation and not updated. Cross-check against the actual PR.
- **Doc vs. reality drift.** The doc may say "we'll do X" while the code does Y. Flag the divergence. The synthesis will surface the contradiction.
- **Multiple drafts.** If a topic has several docs, find the one that was approved or most recently updated. Check dates and review status.
- **Access-restricted artifacts.** If you can't open one, note it as a gap.

## What to return

For each relevant doc:
- Title and Pippin link
- Authors and last-updated date
- The motivation text (verbatim quote), with section location
- Relevant review comments (verbatim, with author)
- Whether the doc was approved or still a draft
