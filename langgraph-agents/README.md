# LangGraph Agents

Local LangGraph pipelines for developer workflow automation.

## GitHub Notifier

Polls GitHub notifications, triages with llama3.2, and sends native macOS notifications with action routing to Claude.

### Flow

```
Poll GitHub API → Fetch full context → llama3.2 triage → macOS notification → (action)
                                                                                  ↓
                                                              Open in Claude (with skill routing)
                                                              Open in Browser
                                                              Dismiss
```

### Features

- **Deterministic triage**: `review_requested`, `assign`, `ci_failure` are auto-classified without LLM
- **Context-enriched**: Fetches PR descriptions, comments, reviewers, diff stats before triaging
- **Custom Notifier.app**: Native macOS notifications with action dropdown, custom icon/sound, no focus steal
- **Claude routing**: "Open in Claude" spawns an iTerm2 session with full context and recommends the right skill (`/review-pr`, `/systematic-debugging`, etc.)
- **Deduplication**: Won't re-notify the same event within a session
- **Marks read**: Interacting with a notification marks it read on GitHub

### Usage

```bash
# Polling loop (default 60s interval)
uv run python github_notifier.py

# Custom interval
uv run python github_notifier.py --interval 30

# Via LangGraph Studio
uv run langgraph dev
```

### Requirements

- `GITHUB_PERSONAL_ACCESS_TOKEN` env var (needs `notifications` scope)
- Ollama running with `llama3.2`
- `~/personal/notifier/Notifier.app` (custom notification app)
- iTerm2 (for Claude sessions)
- `claude` CLI

### Setup

```bash
uv sync
```
