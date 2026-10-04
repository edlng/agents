# Amazon Internal Wiki

## What this source contains

- Team pages, runbooks, and onboarding guides
- Architecture overviews and decision records
- Operational notes that explain defensive code (limits, retries, failover)
- Links out to design docs, tickets, and code reviews

The wiki holds rationale that never reached a design doc: why a limit is set, what an operator must not do, which incident a guard came from.

## How to search it

1. **Keyword searches with `InternalSearch`.** Try:
   - The feature, service, or package name
   - Key symbols, config keys, and error strings from the target code
   - The team name plus "design", "runbook", or "decision"
2. **Fetch candidate pages with `ReadInternalWebsites`.** Read the full page, not the search snippet. Follow child pages and "see also" links that stay on the wiki.
3. **Check page history** when the page states a decision, to date it against the PR.

## What good evidence looks like here

- A runbook step that explains the behavior the target code enforces
- A decision record with context, decision, and consequences filled in specifically
- An architecture page whose diagram or text matches the target code's role
- A page edited in the same date range as the PR by the same author or team

## Common pitfalls

- **Stale pages.** Wiki pages drift. Check the last-edited date and cross-check against the code.
- **Generic pages.** A team template with boilerplate "why" text is not evidence. Look for specificity.
- **Links to other sources.** When a page links a Pippin doc or a code review, record it under "Additional Leads" instead of chasing it.
- **Access-restricted pages.** If you can't open one, note it as a gap.

## What to return

For each relevant page:
- Title and URL
- Last editor and last-edited date
- The motivation text (verbatim quote), with section location
- Linked design docs, tickets, or reviews (as leads)
