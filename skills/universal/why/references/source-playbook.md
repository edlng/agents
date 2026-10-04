# Source playbooks

The why skill spawns one investigator per source, each reading the single playbook below that matches its source.

| Source | Playbook | Tools |
|---|---|---|
| Source control history | [`code-archaeology.md`](./sources/code-archaeology.md) | git, `gh` |
| Pippin design docs | [`pippin.md`](./sources/pippin.md) | Pippin MCP read tools |
| Amazon internal wiki | [`wiki.md`](./sources/wiki.md) | `InternalSearch`, `ReadInternalWebsites` |

Cross-cutting:

- [`incident-postmortem.md`](./sources/incident-postmortem.md). Add this if the target code looks defensive (null checks, retry, timeout, rate limit, feature flag, egress guard, OOM handler).
