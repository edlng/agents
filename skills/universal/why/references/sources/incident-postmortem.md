# Incident & Postmortem Context

Not a separate source, a **cross-cutting angle**. Incidents often motivate defensive code ("we added this check after the X outage"), so if the target looks defensive (null checks, retry logic, timeout handling, rate limiting, feature flags), specifically hunt for incident history across every available source:

- **Git**: commits with messages like "fix for incident", "add defensive check", "revert" followed by "re-apply with..." are strong signals. PR bodies that link a ticket, COE, or postmortem.
- **Pippin**: postmortems or COE documents mentioning the target file, feature, or error string, and design docs written as follow-up action items
- **Wiki**: runbook pages and incident retrospectives that name the target behavior, edited around the dates the target code was added

If you find an incident link, fetch the full postmortem. Postmortems typically have an "Action Items" section that ties directly to code changes. When multiple sources corroborate (a PR links a COE in Pippin, and a wiki runbook cites the same incident), the evidence is especially strong.

Worth spending time on when the code's defensive character makes an incident-driven origin plausible. Skip it for code that doesn't look defensive.
