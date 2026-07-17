import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  buildInstallSet,
  loadCatalog,
  sha256,
} from '../scripts/catalog-lib.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const CODEX_FORBIDDEN = [
  /\bclaude-(?:haiku|sonnet|opus)-/i,
  /\b(?:Haiku|Sonnet|Opus)\b/,
  /\bmcp__[\w_]+/i,
  /\b(?:TodoWrite|AskUserQuestion|Task tool|Skill tool)\b/,
  /~\/\.claude\//,
  /~\/\.kiro\//,
];
const CLAUDE_FORBIDDEN_DESTINATIONS = [
  /~\/\.codex\/agents/,
  /\.codex\/agents/,
  /~\/\.agents\/skills/,
  /\.agents\/skills/,
];

async function variantText(variant) {
  const files = await Promise.all(
    variant.files.map(async (file) => ({
      name: file.name,
      contents: file.contents.toString('utf8'),
    })),
  );
  return files.map(({ name, contents }) => `${name}\n${contents}`).join('\n');
}

test('catalog contains one native Claude and Codex variant for every platform-specific skill', async () => {
  const catalog = await loadCatalog(root);

  assert.equal(catalog.skillVariants.universal.length, 17);
  assert.equal(catalog.skillVariants.claude.length, 23);
  assert.equal(catalog.skillVariants.codex.length, 23);
  assert.equal(catalog.skills.length, 40);
  assert.equal(buildInstallSet(catalog, 'claude').skills.length, 40);
  assert.equal(buildInstallSet(catalog, 'codex').skills.length, 40);

  const universalNames = new Set(
    catalog.skillVariants.universal.map((skill) => skill.name),
  );
  for (const name of universalNames) {
    assert.ok(
      !catalog.skillVariants.claude.some((skill) => skill.name === name),
      `${name} must not have a Claude-specific duplicate`,
    );
    assert.ok(
      !catalog.skillVariants.codex.some((skill) => skill.name === name),
      `${name} must not have a Codex-specific duplicate`,
    );
  }

  const claudeByName = new Map(
    catalog.skillVariants.claude.map((skill) => [skill.name, skill]),
  );
  for (const codexSkill of catalog.skillVariants.codex) {
    const claudeSkill = claudeByName.get(codexSkill.name);
    assert.ok(claudeSkill, `${codexSkill.name} is missing its Claude variant`);
    assert.match(
      codexSkill.contents,
      /> \*\*Codex runtime:\*\*/,
      `${codexSkill.name} must identify its Codex runtime`,
    );
    assert.notEqual(
      sha256(codexSkill.contents),
      sha256(claudeSkill.contents),
      `${codexSkill.name} must be independently maintained`,
    );
  }
});

test('Codex variants contain no Claude-specific model, tool, or path vocabulary', async () => {
  const catalog = await loadCatalog(root);

  for (const skill of catalog.skillVariants.codex) {
    const text = await variantText(skill);
    for (const pattern of CODEX_FORBIDDEN) {
      assert.doesNotMatch(
        text,
        pattern,
        `${skill.name} contains Codex-incompatible vocabulary: ${pattern}`,
      );
    }
  }
});

test('Claude variants contain no Codex installation destinations', async () => {
  const catalog = await loadCatalog(root);

  for (const skill of catalog.skillVariants.claude) {
    const text = await variantText(skill);
    for (const pattern of CLAUDE_FORBIDDEN_DESTINATIONS) {
      assert.doesNotMatch(
        text,
        pattern,
        `${skill.name} contains a Codex installation destination: ${pattern}`,
      );
    }
  }
});

test('platform-specific skill sources load as UTF-8 files', async () => {
  const catalog = await loadCatalog(root);

  for (const platform of ['claude', 'codex']) {
    for (const skill of catalog.skillVariants[platform]) {
      const source = await readFile(skill.path, 'utf8');
      assert.equal(source, skill.contents, `${skill.path} was not loaded exactly`);
    }
  }
});
