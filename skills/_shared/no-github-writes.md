# Shared: No GitHub Writes (read-only output)

> Shared reference used by the local-only review skills (`review-pr`, `review-code`). Not a standalone skill. Single source of truth for the read-only output rule.

The review is local-only. Print it in chat and create only the local artifacts specified by the consuming skill. Do not perform any write operation against GitHub. The user decides what to post after reviewing the output.

## Prohibited write operations

- Post a review (`gh pr review`, the GitHub MCP `pull_request_review_write` tool)
- Add a comment (`gh pr comment`, the GitHub MCP `add_issue_comment` or `add_comment_to_pending_review` tools)
- Approve or request changes
- Update a PR title or body (`gh pr edit`, the GitHub MCP `update_pull_request` tool)
- Push to any branch (the GitHub MCP `push_files` or `create_or_update_file` tools)
- Open or merge PRs (`gh pr create`, `gh pr merge`, the GitHub MCP `create_pull_request` or `merge_*` tools)
- Any other write method (`*_write`, `create_*`, `update_*`, `merge_*`, `delete_*`, or `gh api` with a method other than GET)

## Read access

Both skills read GitHub with the `gh` CLI. Local `git` for inspecting the working tree is always fine.
