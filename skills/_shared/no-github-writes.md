# Shared: No GitHub Writes (read-only output)

> Shared reference used by the local-only review skills (`review-pr`, `review-cookbook-pr`, `review-code`). Not a standalone skill. Single source of truth for the read-only output rule.

The review is **local-only**. Print it in chat and (where the skill specifies) save it to Obsidian. **DO NOT** perform any write operation against GitHub. The user will copy and post manually after reviewing the output.

## Prohibited write operations

- Post a review (`gh pr review`, `mcp__github__pull_request_review_write`)
- Add a comment (`gh pr comment`, `mcp__github__add_issue_comment`, `mcp__github__add_comment_to_pending_review`)
- Approve or request changes
- Update PR title/body (`mcp__github__update_pull_request`)
- Push to any branch (`mcp__github__push_files`, `mcp__github__create_or_update_file`)
- Open or merge PRs (`mcp__github__create_pull_request`, `mcp__github__merge_*`)
- Any other write method (`*_write`, `create_*`, `update_*`, `merge_*`, `delete_*`)

## Read access

Each skill specifies its read mechanism (some use the `gh` CLI for reads, some use `mcp__github__*` read tools only). Follow the consuming skill's stated read convention. Local `git` for inspecting the working tree is always fine.
