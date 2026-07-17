# Testing

## Static Checks

```bash
npm test
node scripts/validate-catalog.mjs
```

## Go and Site Checks

```bash
go test ./...
cd site
npm test
npm run lint
npm run build
```

The full Go command also scans optional GLIDE sample assets; a missing sample
dependency is separate from the focused Litmus package check.

## Litmus

Run deterministic replays without model cost:

```bash
make litmus-replay AGENT=code-reviewer CASE=eval-exec-injection
```

## Runtime Smoke

Runtime smoke is opt-in and uses isolated temporary home/config directories:

```bash
node scripts/runtime-smoke.mjs claude
node scripts/runtime-smoke.mjs codex
```

It checks discovery, loading, and selected model metadata. It does not replace
the static validator or run the full evaluation suite.

Claude smoke uses the native `haiku`, `sonnet`, and `opus` family aliases and
checks the provider-resolved model family. An organization policy that blocks a
family can legitimately cause a smoke failure when Claude substitutes another
family; the smoke output identifies that substitution. The catalog cannot
override that account-level policy.
