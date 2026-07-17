.PHONY: validate test install-claude install-codex smoke-claude smoke-codex \
	litmus-list litmus-replay litmus-probe litmus-batch litmus-grade litmus-compare

validate:
	node scripts/validate-catalog.mjs

test:
	npm test
	go test ./...
	cd site && npm test && npm run lint && npm run build

install-claude:
	node scripts/install.mjs claude

install-codex:
	node scripts/install.mjs codex

smoke-claude:
	node scripts/runtime-smoke.mjs claude

smoke-codex:
	node scripts/runtime-smoke.mjs codex

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
