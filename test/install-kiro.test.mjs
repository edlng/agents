import assert from 'node:assert/strict';
import { mkdir, mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';

import { installKiro, mergeKiroConfig } from '../scripts/install-kiro.mjs';

test('catalog keys win and installed-only keys are kept', () => {
  const merged = mergeKiroConfig(
    { name: 'builder', tools: ['read'], toolsSettings: { shell: { timeout: 120000 } } },
    {
      name: 'old',
      tools: ['read', 'write'],
      toolsSettings: { shell: { timeout: 60000, deniedCommands: ['.*cat.*/\\.aws/.*'] }, execute_bash: { x: 1 } },
    },
  );
  assert.deepEqual(merged, {
    name: 'builder',
    tools: ['read'],
    toolsSettings: { shell: { timeout: 120000, deniedCommands: ['.*cat.*/\\.aws/.*'] }, execute_bash: { x: 1 } },
  });
});

test('installs agents and skills, preserves policy keys, and leaves symlinks alone', async (t) => {
  const kiroRoot = await mkdtemp(path.join(tmpdir(), 'kiro-install-'));
  t.after(() => rm(kiroRoot, { recursive: true, force: true }));

  await mkdir(path.join(kiroRoot, 'agents'), { recursive: true });
  await writeFile(
    path.join(kiroRoot, 'agents', 'builder.json'),
    JSON.stringify({ toolsSettings: { shell: { deniedCommands: ['blocked'] } } }),
  );
  const linkTarget = path.join(kiroRoot, 'target.md');
  await writeFile(linkTarget, 'linked source\n');
  await mkdir(path.join(kiroRoot, 'skills', 'write-pr'), { recursive: true });
  await symlink(linkTarget, path.join(kiroRoot, 'skills', 'write-pr', 'SKILL.md'));

  const dryRun = await installKiro({ kiroRoot, dryRun: true });
  assert.ok(dryRun.some(({ action }) => action === 'create'));
  await assert.rejects(readFile(path.join(kiroRoot, 'agents', 'builder-prompt.md')), { code: 'ENOENT' });

  const entries = await installKiro({ kiroRoot, dryRun: false });
  const builder = JSON.parse(await readFile(path.join(kiroRoot, 'agents', 'builder.json'), 'utf8'));
  assert.equal(builder.name, 'builder');
  assert.equal(builder.prompt, 'file://./builder-prompt.md');
  assert.deepEqual(builder.toolsSettings.shell.deniedCommands, ['blocked']);
  assert.ok((await readFile(path.join(kiroRoot, 'agents', 'builder-prompt.md'), 'utf8')).length > 0);
  assert.ok((await readFile(path.join(kiroRoot, 'skills', 'review-pr', 'SKILL.md'), 'utf8')).includes('name: review-pr'));
  assert.equal(await readFile(linkTarget, 'utf8'), 'linked source\n');
  assert.equal(
    entries.find(({ path: file }) => file.endsWith(path.join('write-pr', 'SKILL.md'))).action,
    'skip-symlink',
  );

  const rerun = await installKiro({ kiroRoot, dryRun: true });
  assert.ok(rerun.every(({ action }) => action === 'unchanged' || action === 'skip-symlink'));
});
