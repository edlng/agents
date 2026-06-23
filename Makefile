# Sync skills and agents between agents/ and ~/.kiro
#
# push: agents/ -> ~/.kiro  (promote local edits to installed)
# pull: ~/.kiro  -> agents/ (pull in new skills/agents from installed)
#
# ~/.claude/skills syncs automatically — it symlinks the two skills that
# only exist in ~/.kiro, and the rest already point to ~/.agents/skills.

KIRO_SKILLS  := $(HOME)/.kiro/skills
KIRO_AGENTS  := $(HOME)/.kiro/agents
LOCAL_SKILLS := skills
LOCAL_AGENTS := agents

.PHONY: push pull status eval eval-smoke eval-agent eval-view eval-reset eval-cost

# Promote local changes to ~/.kiro (additive — never deletes from ~/.kiro)
push:
	rsync -av $(LOCAL_SKILLS)/ $(KIRO_SKILLS)/
	rsync -av $(LOCAL_AGENTS)/ $(KIRO_AGENTS)/

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
	@echo "=== skills: agents/ -> ~/.kiro (would push) ==="
	@rsync -avn $(LOCAL_SKILLS)/ $(KIRO_SKILLS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@echo "=== skills: ~/.kiro -> agents/ (would pull) ==="
	@rsync -avn $(KIRO_SKILLS)/ $(LOCAL_SKILLS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@echo "=== agents: agents/ -> ~/.kiro (would push) ==="
	@rsync -avn $(LOCAL_AGENTS)/ $(KIRO_AGENTS)/ | grep -v "/$\|^sending\|^sent\|^total" || true
	@echo "=== agents: ~/.kiro -> agents/ (would pull) ==="
	@rsync -avn $(KIRO_AGENTS)/ $(LOCAL_AGENTS)/ | grep -v "/$\|^sending\|^sent\|^total" || true

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
