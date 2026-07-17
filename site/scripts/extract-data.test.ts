import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

const siteRoot = resolve(import.meta.dirname, '..');
const extractor = join(siteRoot, 'scripts', 'extract-data.ts');

function writeFixture(root: string, relativePath: string, contents: string): void {
  const filePath = join(root, relativePath);
  mkdirSync(resolve(filePath, '..'), { recursive: true });
  writeFileSync(filePath, contents);
}

test('extracts native agent models and grouped skill compatibility', () => {
  const root = mkdtempSync(join(tmpdir(), 'agents-site-fixture-'));
  const output = join(root, 'graph.json');
  try {
    writeFixture(root, 'agents/builder/manifest.json', JSON.stringify({
      name: 'builder',
      description: 'Builds one scoped task.',
      category: 'implementation',
      profile: 'sonnet',
      platforms: ['claude', 'codex', 'kiro'],
    }));
    writeFixture(root, 'agents/builder/claude.md', `---
name: builder
description: Builds one scoped task.
model: sonnet
effort: medium
---
# Builder
`);
    writeFixture(root, 'agents/builder/codex.toml', `name = "builder"
description = "Builds one scoped task."
model = "openai.gpt-5.6-luna"
model_reasoning_effort = "xhigh"
developer_instructions = "Build one scoped task."
`);
    writeFixture(root, 'agents/builder/kiro.json', '{"name":"builder","model":"global.anthropic.claude-sonnet-5"}');
    writeFixture(root, 'skills/universal/verification-before-completion/SKILL.md', `---
name: verification-before-completion
description: Use before making completion claims.
---
# Verify
`);
    writeFixture(root, 'skills/claude/review-pr/SKILL.md', `---
name: review-pr
description: Use when reviewing a pull request with Claude.
---
# Review
`);
    writeFixture(root, 'skills/codex/review-pr/SKILL.md', `---
name: review-pr
description: Use when reviewing a pull request with Codex.
---
# Review
`);

    execFileSync(join(siteRoot, 'node_modules', '.bin', 'tsx'), [extractor], {
      cwd: siteRoot,
      env: {
        ...process.env,
        CATALOG_ROOT: root,
        CATALOG_OUTPUT: output,
      },
      stdio: 'pipe',
    });

    const graph = JSON.parse(readFileSync(output, 'utf8')) as {
      nodes: Array<Record<string, unknown>>;
    };
    const builder = graph.nodes.find(node => node.id === 'agent:builder');
    const universal = graph.nodes.find(
      node => node.id === 'skill:verification-before-completion',
    );
    const reviewPr = graph.nodes.find(node => node.id === 'skill:review-pr');

    assert.deepEqual(builder?.platforms, ['claude', 'codex', 'kiro']);
    assert.deepEqual(builder?.models, {
      claude: { model: 'sonnet', effort: 'medium' },
      codex: { model: 'openai.gpt-5.6-luna', effort: 'xhigh' },
      kiro: { model: 'global.anthropic.claude-sonnet-5', effort: 'unspecified' },
    });
    assert.equal(universal?.compatibility, 'universal');
    assert.equal(reviewPr?.compatibility, 'variants');
    const sources = reviewPr?.sources as Record<string, string>;
    assert.equal(sources.codex, 'skills/codex/review-pr/SKILL.md');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
