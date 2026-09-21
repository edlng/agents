import assert from 'node:assert/strict';
import test from 'node:test';

import {
  parseClaudeSession,
  parseCodexSession,
} from '../scripts/runtime-smoke.mjs';

test('parses a Claude child session with exact model and effort', () => {
  const parsed = parseClaudeSession([
    {
      type: 'system',
      model: 'global.anthropic.claude-opus-4-8',
      effort: 'high',
      role: 'parent',
    },
    {
      type: 'agent_spawned',
      agent: 'context-curator',
      model: 'global.anthropic.claude-haiku-4-5-20251001-v1:0',
      effort: 'medium',
      loadedSkills: ['code-review-excellence'],
      sourceLocators: ['.claude/skills/code-review-excellence/SKILL.md'],
    },
  ]);

  assert.deepEqual(parsed, {
    role: 'context-curator',
    model: 'global.anthropic.claude-haiku-4-5-20251001-v1:0',
    effort: 'medium',
    sandbox: null,
    loadedSkills: ['code-review-excellence'],
    sourceLocators: ['.claude/skills/code-review-excellence/SKILL.md'],
  });
});

test('parses a Codex child session with sandbox and source locator', () => {
  const parsed = parseCodexSession(`{"type":"session_meta","role":"parent","model":"openai.gpt-5.6-sol","model_reasoning_effort":"high"}
{"type":"agent_spawned","role":"builder","model":"openai.gpt-5.6-luna","model_reasoning_effort":"xhigh","sandbox_mode":"workspace-write","loaded_skills":["crash-course"],"source_locators":[".agents/skills/crash-course/SKILL.md"]}
`);

  assert.deepEqual(parsed, {
    role: 'builder',
    model: 'openai.gpt-5.6-luna',
    effort: 'xhigh',
    sandbox: 'workspace-write',
    loadedSkills: ['crash-course'],
    sourceLocators: ['.agents/skills/crash-course/SKILL.md'],
  });
});

test('rejects generic model aliases', () => {
  assert.throws(
    () => parseClaudeSession([
      { type: 'agent_spawned', agent: 'builder', model: 'sonnet', effort: 'medium' },
    ]),
    /exact provider model/,
  );
});

test('rejects missing child role instead of using a parent session model', () => {
  assert.throws(
    () => parseCodexSession([
      { type: 'session_meta', model: 'openai.gpt-5.6-sol', model_reasoning_effort: 'high' },
    ]),
    /child agent role/,
  );
});

test('rejects Codex sessions without sandbox metadata', () => {
  assert.throws(
    () => parseCodexSession([
      {
        type: 'agent_spawned',
        role: 'builder',
        model: 'openai.gpt-5.6-luna',
        model_reasoning_effort: 'xhigh',
      },
    ]),
    /sandbox mode/,
  );
});
