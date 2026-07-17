# Compatibility

## Universal Skills

A skill is universal only when its instructions, references, tools, paths, and
model guidance work unchanged in both Claude Code and Codex. Universal skills
must not name a concrete provider model ID.

## Platform Variants

Use a native variant when a workflow depends on platform-specific dispatch,
user-input, MCP, installation, or model-selection behavior. Claude and Codex
variants keep the same behavioral contract but use their native vocabulary.

Shared references live in `skills/_shared/` and are copied beside either
platform's skills. A skill name may exist in the universal tree or in both
platform trees, but never in universal and a platform tree at the same time.

## Kiro

Kiro agent definitions remain in each `agents/<role>/` family. Kiro is not a
target of the Claude/Codex installer and does not change the universal versus
variant classification.

## Model Policy

Claude agents use the Claude Code family aliases `haiku`, `sonnet`, and
`opus`, so the catalog does not freeze dated Claude model IDs. Haiku and
Sonnet use at least `medium` effort; Opus uses `high`.

Codex Haiku and Sonnet use `openai.gpt-5.6-luna` at `xhigh`. Codex Opus uses
`openai.gpt-5.6-sol` at `high`. No Claude profile may use reasoning below
`medium`.

Claude Code resolves family aliases according to the provider and account
policy. Deployments that need explicit routing can set
`ANTHROPIC_DEFAULT_HAIKU_MODEL`, `ANTHROPIC_DEFAULT_SONNET_MODEL`, and
`ANTHROPIC_DEFAULT_OPUS_MODEL` outside this repository.
