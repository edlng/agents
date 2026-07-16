# Sync skills and agents across five roots:
#   Root 1 (canonical): ~/.kiro/
#   Root 2:             ~/.claude/
#   Root 3:             ~/agents/ (this repo)
#   Root 4 (skills only): ~/.config/devin/skills/
#   Root 5 (skills only): ~/.codex/skills/
#
# push: agents/ -> ~/.kiro + ~/.claude + ~/.config/devin + ~/.codex  (promote local edits)
# pull: ~/.kiro  -> agents/ (pull in new skills/agents from canonical)
#
# Agents skip Root 4 and Root 5 — devin-cli uses --agent-config and Codex does
# not use agent markdown files.

KIRO_SKILLS   := $(HOME)/.kiro/skills
KIRO_AGENTS   := $(HOME)/.kiro/agents
CLAUDE_SKILLS := $(HOME)/.claude/skills
CLAUDE_AGENTS := $(HOME)/.claude/agents
DEVIN_SKILLS  := $(HOME)/.config/devin/skills
CODEX_SKILLS  := $(HOME)/.codex/skills
LOCAL_SKILLS  := skills
LOCAL_AGENTS  := agents

.PHONY: push pull status litmus-list litmus-replay litmus-probe litmus-batch litmus-grade litmus-compare

# Promote local changes to all roots (additive — never deletes from targets)
push:
	rsync -av $(LOCAL_SKILLS)/ $(KIRO_SKILLS)/
	rsync -av $(LOCAL_AGENTS)/ $(KIRO_AGENTS)/
	rsync -av $(LOCAL_SKILLS)/ $(CLAUDE_SKILLS)/
	rsync -av $(LOCAL_AGENTS)/ $(CLAUDE_AGENTS)/
	@mkdir -p $(DEVIN_SKILLS)
	rsync -av $(LOCAL_SKILLS)/ $(DEVIN_SKILLS)/
	@mkdir -p $(CODEX_SKILLS)
	rsync -av $(LOCAL_SKILLS)/ $(CODEX_SKILLS)/

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
	@echo "=== skills: agents/ vs ~/.codex ==="
	@rsync -avn $(LOCAL_SKILLS)/ $(CODEX_SKILLS)/ 2>/dev/null | grep -v "/$\|^sending\|^sent\|^total" || true

# ── Litmus evals ─────────────────────────────────────────────────────────────

litmus-list:
	GOPROXY=direct go run ./litmus/cmd/litmus-eval list

litmus-replay:
	@if [ -z "$(AGENT)" ] || [ -z "$(CASE)" ]; then echo "Usage: make litmus-replay AGENT=<agent> CASE=<case>"; exit 1; fi
	GOPROXY=direct go run ./litmus/cmd/litmus-eval replay "$(AGENT)" "$(CASE)"

litmus-probe:
	@if [ -z "$(AGENT)" ] || [ -z "$(CASE)" ] || [ -z "$(BUDGET)" ]; then echo "Usage: make litmus-probe AGENT=<agent> CASE=<case> BUDGET=<usd>"; exit 1; fi
	GOPROXY=direct go run ./litmus/cmd/litmus-eval probe "$(AGENT)" "$(CASE)" --budget "$(BUDGET)"

litmus-batch:
	@if [ -z "$(MANIFEST)" ] || [ -z "$(BUDGET)" ]; then echo "Usage: make litmus-batch MANIFEST=<manifest> BUDGET=<usd>"; exit 1; fi
	GOPROXY=direct go run ./litmus/cmd/litmus-eval batch "$(MANIFEST)" --budget "$(BUDGET)" $(BATCH_ARGS)

litmus-grade:
	@if [ -z "$(RUN)" ] || [ -z "$(BUDGET)" ]; then echo "Usage: make litmus-grade RUN=<run-dir> BUDGET=<usd>"; exit 1; fi
	GOPROXY=direct go run ./litmus/cmd/litmus-eval grade "$(RUN)" --budget "$(BUDGET)" $(GRADE_ARGS)

litmus-compare:
	@if [ -z "$(BASELINE)" ] || [ -z "$(CURRENT)" ]; then echo "Usage: make litmus-compare BASELINE=<run-dir> CURRENT=<run-dir>"; exit 1; fi
	GOPROXY=direct go run ./litmus/cmd/litmus-eval compare "$(BASELINE)" "$(CURRENT)"
