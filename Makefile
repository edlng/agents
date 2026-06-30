# Sync skills and agents across four roots:
#   Root 1 (canonical): ~/.kiro/
#   Root 2:             ~/.claude/
#   Root 3:             ~/agents/ (this repo)
#   Root 4 (skills only): ~/.config/devin/skills/
#
# push: agents/ -> ~/.kiro + ~/.claude + ~/.config/devin  (promote local edits)
# pull: ~/.kiro  -> agents/ (pull in new skills/agents from canonical)
#
# Agents skip Root 4 — devin-cli uses --agent-config instead of agent markdown files.

KIRO_SKILLS   := $(HOME)/.kiro/skills
KIRO_AGENTS   := $(HOME)/.kiro/agents
CLAUDE_SKILLS := $(HOME)/.claude/skills
CLAUDE_AGENTS := $(HOME)/.claude/agents
DEVIN_SKILLS  := $(HOME)/.config/devin/skills
LOCAL_SKILLS  := skills
LOCAL_AGENTS  := agents

.PHONY: push pull status eval eval-smoke eval-agent eval-view eval-reset eval-cost

# Promote local changes to all roots (additive — never deletes from targets)
push:
	rsync -av $(LOCAL_SKILLS)/ $(KIRO_SKILLS)/
	rsync -av $(LOCAL_AGENTS)/ $(KIRO_AGENTS)/
	rsync -av $(LOCAL_SKILLS)/ $(CLAUDE_SKILLS)/
	rsync -av $(LOCAL_AGENTS)/ $(CLAUDE_AGENTS)/
	@mkdir -p $(DEVIN_SKILLS)
	rsync -av $(LOCAL_SKILLS)/ $(DEVIN_SKILLS)/

# Pull ~/.kiro changes into local repo (dry-run first; confirm with: make pull CONFIRM=1)
pull:
ifndef CONFIRM
	@echo "--- DRY RUN (run 'make pull CONFIRM=1' to apply) ---"
	rsync -avnL $(KIRO_SKILLS)/ $(LOCAL_SKILLS)/
	rsync -avnL $(KIRO_AGENTS)/ $(LOCAL_AGENTS)/
else
	rsync -avL $(KIRO_SKILLS)/ $(LOCAL_SKILLS)/
	rsync -avL $(KIRO_AGENTS)/ $(LOCAL_AGENTS)/
endif

# Show what's out of sync without changing anything
status:
	@echo "=== skills: agents/ vs ~/.kiro ==="
	@rsync -avn $(LOCAL_SKILLS)/ $(KIRO_SKILLS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@rsync -avn $(KIRO_SKILLS)/ $(LOCAL_SKILLS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@echo "=== agents: agents/ vs ~/.kiro ==="
	@rsync -avn $(LOCAL_AGENTS)/ $(KIRO_AGENTS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@rsync -avn $(KIRO_AGENTS)/ $(LOCAL_AGENTS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@echo "=== skills: agents/ vs ~/.claude ==="
	@rsync -avn $(LOCAL_SKILLS)/ $(CLAUDE_SKILLS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@echo "=== agents: agents/ vs ~/.claude ==="
	@rsync -avn $(LOCAL_AGENTS)/ $(CLAUDE_AGENTS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@echo "=== skills: agents/ vs ~/.config/devin ==="
	@rsync -avn $(LOCAL_SKILLS)/ $(DEVIN_SKILLS)/ 2>/dev/null | grep -v "/$\|^sending\|^sent\|^total" || true

# ── Evals ────────────────────────────────────────────────────────────────────

# Run the full eval suite and print a cost summary when done.
eval:
	@mkdir -p evals/metrics
	@rm -f evals/metrics/token_usage.jsonl
	./run-eval.sh; EXIT=$$?; node evals/scripts/cost-summary.js; exit $$EXIT

# Run the cost-efficient smoke suite (fewer tests, smaller tasks, same behavioral checks).
eval-smoke:
	@mkdir -p evals/metrics
	@rm -f evals/metrics/token_usage.jsonl
	EVAL_MAX_BUDGET=0.15 npx promptfoo eval -c promptfooconfig.smoke.yaml --no-cache; EXIT=$$?; node evals/scripts/cost-summary.js; exit $$EXIT

# Run tests for a single agent. Usage: make eval-agent AGENT=code-reviewer
eval-agent:
	@if [ -z "$(AGENT)" ]; then echo "Usage: make eval-agent AGENT=<agent-name>"; exit 1; fi
	@mkdir -p evals/metrics
	@rm -f evals/metrics/token_usage.jsonl
	npx promptfoo eval --filter-pattern "$(AGENT)"
	@node evals/scripts/cost-summary.js

# Open the promptfoo results UI in a browser.
eval-view:
	npm run eval:view

# Clear promptfoo state and re-run.
eval-reset:
	npm run eval:reset
	$(MAKE) eval

# Print cost summary from the last run without re-running evals.
eval-cost:
	@node evals/scripts/cost-summary.js
