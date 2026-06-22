"""
GitHub Event Notifier: polls GitHub notifications, summarizes with llama3.2,
sends native macOS notifications, optionally spawns a Claude session.

Flow: Poll → Fetch context → Summarize (llama3.2) → Notify → (action)

Usage:
  uv run python github_notifier.py [--interval 60]
"""

import json
import os
import subprocess
import sys
import tempfile
import time
from typing import TypedDict

import httpx
from langchain_core.messages import HumanMessage, SystemMessage
from langchain_ollama import ChatOllama
from langgraph.graph import END, StateGraph

# --- Config ---
GITHUB_API = "https://api.github.com/notifications"
POLL_INTERVAL = 60

_seen_ids: set[str] = set()


# --- State ---
class NotifierState(TypedDict):
    notifications: list[dict]
    triaged: list[dict]
    notified: list[str]


# --- Helpers ---
def _github_headers() -> dict:
    token = os.environ.get("GITHUB_PERSONAL_ACCESS_TOKEN", "")
    return {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
    }


def _mark_read(thread_id: str) -> None:
    try:
        httpx.patch(
            f"https://api.github.com/notifications/threads/{thread_id}",
            headers=_github_headers(),
            timeout=10,
        )
    except httpx.HTTPError:
        pass


def _api_url_to_html(api_url: str, notif_type: str) -> str:
    url = api_url.replace("https://api.github.com/repos/", "https://github.com/")
    if notif_type == "PullRequest":
        url = url.replace("/pulls/", "/pull/")
    return url


def _fetch_resource_context(api_url: str) -> str:
    """Fetch full context from a GitHub API resource URL."""
    if not api_url:
        return ""
    try:
        resp = httpx.get(api_url, headers=_github_headers(), timeout=10)
        resp.raise_for_status()
        data = resp.json()
    except (httpx.HTTPError, json.JSONDecodeError):
        return ""

    parts = []

    body = data.get("body") or ""
    if body:
        parts.append(f"Description:\n{body[:2000]}")

    user = data.get("user", {}).get("login", "")
    if user:
        parts.append(f"Author: {user}")

    state = data.get("state", "")
    if state:
        parts.append(f"State: {state}")

    labels = [l.get("name", "") for l in data.get("labels", [])]
    if labels:
        parts.append(f"Labels: {', '.join(labels)}")

    reviewers = [r.get("login", "") for r in data.get("requested_reviewers", [])]
    if reviewers:
        parts.append(f"Reviewers requested: {', '.join(reviewers)}")
    if data.get("changed_files"):
        parts.append(f"Files changed: {data['changed_files']}")
    if data.get("additions") is not None:
        parts.append(f"Diff: +{data.get('additions', 0)} -{data.get('deletions', 0)}")

    comments_url = data.get("comments_url")
    if comments_url:
        try:
            cresp = httpx.get(
                comments_url,
                headers=_github_headers(),
                params={"per_page": 3, "sort": "created", "direction": "desc"},
                timeout=10,
            )
            if cresp.status_code == 200:
                comments = cresp.json()
                if comments:
                    parts.append("Recent comments:")
                    for c in reversed(comments):
                        author = c.get("user", {}).get("login", "unknown")
                        cbody = (c.get("body") or "")[:300]
                        parts.append(f"  @{author}: {cbody}")
        except httpx.HTTPError:
            pass

    return "\n".join(parts)


def _action_type_from_reason(reason: str) -> str:
    """Map GitHub notification reason to an action type for display."""
    return {
        "review_requested": "review_requested",
        "assign": "assigned",
        "ci_activity": "ci_failure",
        "mention": "mentioned_fyi",
        "author": "other",
        "subscribed": "other",
        "comment": "other",
    }.get(reason, "other")


# --- Nodes ---
def poll_github(state: NotifierState) -> dict:
    """Fetch unread GitHub notifications, filtering out already-seen ones."""
    token = os.environ.get("GITHUB_PERSONAL_ACCESS_TOKEN")
    if not token:
        print("[error] GITHUB_PERSONAL_ACCESS_TOKEN not set")
        return {"notifications": []}

    try:
        resp = httpx.get(
            GITHUB_API, headers=_github_headers(), params={"all": "false"}, timeout=15
        )
        resp.raise_for_status()
        all_notifs = resp.json()
    except httpx.HTTPError as e:
        print(f"[error] GitHub API: {e}")
        return {"notifications": []}

    new_notifs = [n for n in all_notifs if n.get("id") not in _seen_ids]
    if len(all_notifs) > len(new_notifs):
        print(f"  Skipped {len(all_notifs) - len(new_notifs)} already-seen")

    return {"notifications": new_notifs}


def fetch_context(state: NotifierState) -> dict:
    """Enrich notifications with full context from the GitHub API."""
    for notif in state["notifications"]:
        url = notif.get("subject", {}).get("url", "")
        notif["_context"] = _fetch_resource_context(url)
    return {"notifications": state["notifications"]}


def triage_with_llama(state: NotifierState) -> dict:
    """Generate a summary for each notification using llama3.2."""
    if not state["notifications"]:
        return {"triaged": []}

    llm = ChatOllama(model="llama3.2", temperature=0)
    system = SystemMessage(content=(
        "Summarize this GitHub notification in one sentence. "
        "Be specific about what happened based on the context. "
        "Respond ONLY with JSON: {\"summary\": \"...\"}"
    ))

    results = []
    for notif in state["notifications"]:
        repo = notif.get("repository", {}).get("full_name", "unknown")
        title = notif.get("subject", {}).get("title", "")
        reason = notif.get("reason", "")
        notif_type = notif.get("subject", {}).get("type", "")
        url = notif.get("subject", {}).get("url", "")
        thread_id = notif.get("id", "")
        context = notif.get("_context", "")

        prompt = HumanMessage(content=(
            f"Type: {notif_type}\nRepo: {repo}\n"
            f"Title: {title}\nReason: {reason}\n"
            f"Context:\n{context[:2000]}"
        ))

        try:
            response = llm.invoke([system, prompt])
            text = response.content
            start = text.find("{")
            end = text.rfind("}") + 1
            if start >= 0 and end > start:
                parsed = json.loads(text[start:end])
                summary = parsed.get("summary", title)
            else:
                summary = title
        except Exception:
            summary = title

        results.append({
            "title": title,
            "repo": repo,
            "reason": reason,
            "type": notif_type,
            "action_type": _action_type_from_reason(reason),
            "summary": summary,
            "url": url,
            "thread_id": thread_id,
            "html_url": _api_url_to_html(url, notif_type),
            "context": context,
        })

        _seen_ids.add(thread_id)

    return {"triaged": results}


def notify_mac(state: NotifierState) -> dict:
    """Send macOS notification for each triaged item."""
    notified = []

    action_labels = {
        "review_requested": "👀 Review Requested",
        "question_asked": "❓ Question for You",
        "ci_failure": "🔴 CI Failed",
        "assigned": "📌 Assigned to You",
        "mentioned_fyi": "💬 Mentioned",
        "other": "📣 Notification",
    }

    for item in state["triaged"]:
        action_type = item.get("action_type", "other")
        title = f"{action_labels.get(action_type, '📣')} [{item['repo']}]"

        notifier_bin = os.path.expanduser(
            "~/personal/notifier/Notifier.app/Contents/MacOS/notifier"
        )
        cmd = [
            notifier_bin,
            "--title", title,
            "--message", item["summary"],
            "--subtitle", item["reason"],
            "--actions", "Open in Claude,Open in Browser,Dismiss",
            "--group", f"gh-{item['thread_id']}",
            "--timeout", "60",
        ]

        try:
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=65)
            action = result.stdout.strip()

            if action == "Open in Claude":
                spawn_claude(item)
                _mark_read(item["thread_id"])
            elif action == "Open in Browser":
                subprocess.Popen(["open", item["html_url"]])
                _mark_read(item["thread_id"])
            elif action in ("Dismiss", "@DISMISSED"):
                _mark_read(item["thread_id"])

            notified.append(item["title"])
        except (subprocess.TimeoutExpired, FileNotFoundError) as e:
            print(f"[notify error] {e}")

    return {"notified": notified}


def spawn_claude(item: dict) -> None:
    """Open interactive Claude session in iTerm2 with context and skill routing."""
    context_block = item.get("context", "")

    prompt = (
        f"GitHub notification:\n"
        f"- Repo: {item['repo']}\n"
        f"- Type: {item['type']}\n"
        f"- Title: {item['title']}\n"
        f"- Reason: {item['reason']}\n"
        f"- Summary: {item['summary']}\n"
        f"- URL: {item['html_url']}\n"
    )
    if context_block:
        prompt += f"\nFull context:\n{context_block[:2000]}\n"

    prompt += (
        "\n---\n"
        "First, understand what this notification is about and what action is needed.\n"
        "Then determine the best approach from these options:\n"
        "- /review-pr - if someone needs you to review a PR\n"
        "- /review-code - if you need to self-review your own changes\n"
        "- /implement-jira - if there is a linked Jira ticket to implement\n"
        "- /systematic-debugging - if CI failed or there is a bug\n"
        "- /write-pr - if you need to write a PR description\n"
        "- /review-addressed-comments - if you need to check if review comments were addressed\n"
        "- Direct action - if it is a simple response or question\n\n"
        "Explain what you recommend and ask me how to proceed."
    )

    tmp = tempfile.NamedTemporaryFile(mode="w", suffix=".txt", prefix="gh-notif-", delete=False)
    tmp.write(prompt)
    tmp.close()

    allowed_tools = "read,pull_request_read,web_fetch,web_search,firecrawl_scrape,firecrawl_search,firecrawl_map"

    launcher = tempfile.NamedTemporaryFile(mode="w", suffix=".sh", prefix="gh-launch-", delete=False)
    launcher.write(f'#!/bin/bash\ncat "{tmp.name}" | claude --allowedTools "{allowed_tools}"\n')
    launcher.close()
    os.chmod(launcher.name, 0o755)

    script = (
        'tell application "iTerm2"\n'
        "    activate\n"
        "    tell current window\n"
        "        create tab with default profile\n"
        "        tell current session\n"
        f'            write text "{launcher.name}"\n'
        "        end tell\n"
        "    end tell\n"
        "end tell"
    )
    subprocess.Popen(["osascript", "-e", script])


# --- Graph ---
def build_graph() -> StateGraph:
    graph = StateGraph(NotifierState)

    graph.add_node("poll", poll_github)
    graph.add_node("fetch_context", fetch_context)
    graph.add_node("triage", triage_with_llama)
    graph.add_node("notify", notify_mac)

    graph.set_entry_point("poll")
    graph.add_edge("poll", "fetch_context")
    graph.add_edge("fetch_context", "triage")
    graph.add_edge("triage", "notify")
    graph.add_edge("notify", END)

    return graph.compile()


# --- Main loop ---
def main():
    interval = POLL_INTERVAL
    args = sys.argv[1:]
    if "--interval" in args:
        idx = args.index("--interval")
        interval = int(args[idx + 1])

    if not os.environ.get("GITHUB_PERSONAL_ACCESS_TOKEN"):
        print("Error: GITHUB_PERSONAL_ACCESS_TOKEN environment variable required")
        print("Create one at: https://github.com/settings/tokens")
        print("Needs 'notifications' scope")
        sys.exit(1)

    print(f"GitHub Notifier running (polling every {interval}s)")
    print(f"Model: llama3.2 | Notifier: ~/personal/notifier/Notifier.app\n")

    app = build_graph()

    while True:
        try:
            print(f"[{time.strftime('%H:%M:%S')}] Polling...")
            result = app.invoke({
                "notifications": [],
                "triaged": [],
                "notified": [],
            })

            if result["notified"]:
                print(f"  Notified: {len(result['notified'])} items")
            else:
                print("  Nothing new")

        except KeyboardInterrupt:
            print("\nStopped.")
            break
        except Exception as e:
            print(f"  [error] {e}")

        time.sleep(interval)


if __name__ == "__main__":
    main()
