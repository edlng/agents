import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import {
  chmod,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  stat,
  symlink,
  writeFile,
} from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import {
  getCatalogRevision,
  install,
  parseArgs,
  USAGE,
} from '../scripts/install.mjs';

const execFileAsync = promisify(execFile);
const REPOSITORY_ROOT = fileURLToPath(new URL('..', import.meta.url));
const INSTALL_SCRIPT = path.join(REPOSITORY_ROOT, 'scripts/install.mjs');
const STATE_FILE = '.agents-catalog-install.json';
const DESCRIPTION = 'Worker agent that executes one scoped implementation task.';

const FIXTURE_FILES = {
  'platforms/model-policy.json': `${JSON.stringify({
    minimumClaudeEffort: 'medium',
    profiles: {
      sonnet: {
        claude: { model: 'sonnet', effort: 'medium' },
        codex: { model: 'openai.gpt-5.6-luna', effort: 'xhigh' },
      },
    },
  }, null, 2)}\n`,
  'agents/builder/manifest.json': `${JSON.stringify({
    name: 'builder',
    description: DESCRIPTION,
    category: 'implementation',
    profile: 'sonnet',
    platforms: ['claude', 'codex'],
  }, null, 2)}\n`,
  'agents/builder/claude.md': Buffer.from(`---
name: builder
description: ${DESCRIPTION}
model: sonnet
effort: medium
---

Build the requested feature.
`),
  'agents/builder/codex.toml': Buffer.from(`name = "builder"
description = "${DESCRIPTION}"
model = "openai.gpt-5.6-luna"
model_reasoning_effort = "xhigh"
sandbox_mode = "workspace-write"
developer_instructions = "Build the requested feature."
`),
  'skills/universal/write-pr/SKILL.md': Buffer.from(`---
name: write-pr
description: Write a pull request description.
---

Read references/guide.md.
`),
  'skills/universal/write-pr/references/guide.md': Buffer.from('# Guide\r\nKeep CRLF.\r\n'),
  'skills/universal/write-pr/assets/blob.bin': Buffer.from([0, 1, 2, 13, 10, 255]),
  'skills/claude/review-pr/SKILL.md': Buffer.from(`---
name: review-pr
description: Review a pull request with Claude.
---

Review it.
`),
  'skills/codex/review-pr/SKILL.md': Buffer.from(`---
name: review-pr
description: Review a pull request with Codex.
---

Review it.
`),
  'skills/_shared/common.md': Buffer.from('# Shared\n'),
  'skills/_shared/nested/data.bin': Buffer.from([255, 0, 10, 13, 7]),
};

function asBuffer(contents) {
  return Buffer.isBuffer(contents) ? contents : Buffer.from(contents);
}

async function writeFixtureFile(root, relativePath, contents) {
  const absolutePath = path.join(root, ...relativePath.split('/'));
  await mkdir(path.dirname(absolutePath), { recursive: true });
  await writeFile(absolutePath, asBuffer(contents));
}

async function listFiles(root, directory = root) {
  const files = [];
  let entries;
  try {
    entries = await readdir(directory, { withFileTypes: true });
  } catch (error) {
    if (error.code === 'ENOENT') {
      return [];
    }
    throw error;
  }
  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await listFiles(root, entryPath));
    } else {
      files.push(path.relative(root, entryPath).split(path.sep).join('/'));
    }
  }
  return files.sort();
}

async function runGit(root, args) {
  return execFileAsync('git', args, {
    cwd: root,
    env: {
      ...process.env,
      GIT_AUTHOR_NAME: 'Catalog Test',
      GIT_AUTHOR_EMAIL: 'catalog@example.test',
      GIT_COMMITTER_NAME: 'Catalog Test',
      GIT_COMMITTER_EMAIL: 'catalog@example.test',
    },
  });
}

async function recordFixtureHead(root) {
  await runGit(root, ['init', '--quiet']);
  for (const relativePath of await listFiles(root)) {
    if (relativePath.startsWith('.git/')) {
      continue;
    }
    const { stdout } = await runGit(root, ['hash-object', '-w', relativePath]);
    await runGit(root, [
      'update-index',
      '--add',
      '--cacheinfo',
      `100644,${stdout.trim()},${relativePath}`,
    ]);
  }
  const { stdout: tree } = await runGit(root, ['write-tree']);
  const { stdout: revision } = await runGit(
    root,
    ['commit-tree', tree.trim(), '-m', 'fixture'],
  );
  await runGit(root, ['update-ref', 'HEAD', revision.trim()]);
  return revision.trim();
}

async function createCatalog(t, overrides = {}) {
  const root = await mkdtemp(path.join(tmpdir(), 'agents-install-catalog-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  for (const [relativePath, contents] of Object.entries({
    ...FIXTURE_FILES,
    ...overrides,
  })) {
    if (contents !== null) {
      await writeFixtureFile(root, relativePath, contents);
    }
  }
  const revision = await recordFixtureHead(root);
  return { root, revision };
}

async function createTarget(t, name = 'target') {
  const parent = await mkdtemp(path.join(tmpdir(), 'agents-install-target-'));
  t.after(() => rm(parent, { recursive: true, force: true }));
  return path.join(parent, name);
}

async function readState(root) {
  return JSON.parse(await readFile(path.join(root, STATE_FILE), 'utf8'));
}

function expectedFiles(platform) {
  const agentExtension = platform === 'claude' ? 'md' : 'toml';
  const agentSource = `agents/builder/${platform === 'claude' ? 'claude.md' : 'codex.toml'}`;
  const agentRoot = platform === 'claude' ? '.claude' : '.codex';
  const skillRoot = platform === 'claude' ? '.claude/skills' : '.agents/skills';
  return {
    [`${agentRoot}/agents/builder.${agentExtension}`]: agentSource,
    [`${skillRoot}/review-pr/SKILL.md`]: `skills/${platform}/review-pr/SKILL.md`,
    [`${skillRoot}/write-pr/SKILL.md`]: 'skills/universal/write-pr/SKILL.md',
    [`${skillRoot}/write-pr/assets/blob.bin`]:
      'skills/universal/write-pr/assets/blob.bin',
    [`${skillRoot}/write-pr/references/guide.md`]:
      'skills/universal/write-pr/references/guide.md',
    [`${skillRoot}/_shared/common.md`]: 'skills/_shared/common.md',
    [`${skillRoot}/_shared/nested/data.bin`]: 'skills/_shared/nested/data.bin',
  };
}

async function assertInstalledBytes(catalogRoot, targetRoot, platform) {
  for (const [destination, source] of Object.entries(expectedFiles(platform))) {
    assert.deepEqual(
      await readFile(path.join(targetRoot, ...destination.split('/'))),
      await readFile(path.join(catalogRoot, ...source.split('/'))),
      destination,
    );
  }
}

async function assertMissing(filePath) {
  await assert.rejects(lstat(filePath), { code: 'ENOENT' });
}

async function findTempDebris(root) {
  return (await listFiles(root)).filter((file) => file.includes('.tmp-'));
}

test('installs exact recursive bytes to user and project roots for both platforms', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  for (const platform of ['claude', 'codex']) {
    for (const scope of ['user', 'project']) {
      const targetRoot = await createTarget(t, `${platform}-${scope}`);
      const options = scope === 'user'
        ? { home: targetRoot, cwd: '/unused' }
        : { home: '/unused', cwd: targetRoot };

      const report = await install({
        platform,
        scope,
        catalogRoot,
        ...options,
      });

      assert.equal(report.root, path.resolve(targetRoot));
      await assertInstalledBytes(catalogRoot, targetRoot, platform);
    }
  }
});

test('installs executable and normal skill files with their source modes', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const executableSource = path.join(
    catalogRoot,
    'skills/universal/write-pr/assets/blob.bin',
  );
  await chmod(executableSource, 0o755);
  const target = await createTarget(t);

  await install({ platform: 'claude', target, catalogRoot });

  const skillRoot = path.join(target, '.claude/skills/write-pr');
  assert.equal(
    (await stat(path.join(skillRoot, 'assets/blob.bin'))).mode & 0o777,
    0o755,
  );
  assert.equal(
    (await stat(path.join(skillRoot, 'SKILL.md'))).mode & 0o777,
    0o644,
  );
});

test('dry-run fully plans an install without creating the target', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t);

  const report = await install({
    platform: 'claude',
    target,
    catalogRoot,
    dryRun: true,
  });

  assert.equal(report.dryRun, true);
  assert.equal(report.files.length, Object.keys(expectedFiles('claude')).length);
  assert.ok(report.files.every(({ action }) => action === 'create'));
  await assertMissing(target);
});

test('first install records exact hashes and revision, then reruns idempotently', async (t) => {
  const { root: catalogRoot, revision } = await createCatalog(t);
  const target = await createTarget(t);

  const first = await install({ platform: 'claude', target, catalogRoot });
  const stateBytes = await readFile(path.join(target, STATE_FILE));
  const state = JSON.parse(stateBytes);

  assert.equal(first.catalogRevision, revision);
  assert.deepEqual(
    {
      schemaVersion: state.schemaVersion,
      platform: state.platform,
      catalogRevision: state.catalogRevision,
    },
    { schemaVersion: 1, platform: 'claude', catalogRevision: revision },
  );
  assert.deepEqual(Object.keys(state.files), Object.keys(expectedFiles('claude')).sort());
  for (const [destination, source] of Object.entries(expectedFiles('claude'))) {
    const sourceBytes = await readFile(path.join(catalogRoot, ...source.split('/')));
    assert.deepEqual(state.files[destination], {
      sha256: (await import('node:crypto'))
        .createHash('sha256')
        .update(sourceBytes)
        .digest('hex'),
      source,
    });
  }

  const second = await install({ platform: 'claude', target, catalogRoot });
  assert.ok(second.files.every(({ action }) => action === 'unchanged'));
  assert.equal(second.stateAction, 'unchanged');
  assert.deepEqual(await readFile(path.join(target, STATE_FILE)), stateBytes);
});

test('a preflight collision prevents every planned write', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t);
  const collision = path.join(target, '.claude/skills/write-pr/SKILL.md');
  await mkdir(path.dirname(collision), { recursive: true });
  await writeFile(collision, 'local version\n');

  await assert.rejects(
    install({ platform: 'claude', target, catalogRoot }),
    /unowned destination differs.*\.claude\/skills\/write-pr\/SKILL\.md/s,
  );

  assert.equal(await readFile(collision, 'utf8'), 'local version\n');
  await assertMissing(path.join(target, '.claude/agents/builder.md'));
  await assertMissing(path.join(target, STATE_FILE));
});

test('adopts byte-equal files, force replaces unowned files, and owned files update', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t);
  const destination = '.claude/agents/builder.md';
  const destinationPath = path.join(target, destination);
  await mkdir(path.dirname(destinationPath), { recursive: true });
  await writeFile(destinationPath, FIXTURE_FILES['agents/builder/claude.md']);

  const adopted = await install({ platform: 'claude', target, catalogRoot });
  assert.equal(
    adopted.files.find(({ path: file }) => file === destination).action,
    'adopt',
  );

  const forceTarget = await createTarget(t, 'force');
  const forcePath = path.join(forceTarget, destination);
  await mkdir(path.dirname(forcePath), { recursive: true });
  await writeFile(forcePath, 'local version\n');
  const forced = await install({
    platform: 'claude',
    target: forceTarget,
    catalogRoot,
    force: true,
  });
  assert.equal(
    forced.files.find(({ path: file }) => file === destination).action,
    'replace',
  );
  assert.deepEqual(
    await readFile(forcePath),
    FIXTURE_FILES['agents/builder/claude.md'],
  );

  const updated = Buffer.from(
    FIXTURE_FILES['agents/builder/claude.md'].toString().replace('feature', 'change'),
  );
  await writeFixtureFile(catalogRoot, 'agents/builder/claude.md', updated);
  const owned = await install({ platform: 'claude', target, catalogRoot });
  assert.equal(
    owned.files.find(({ path: file }) => file === destination).action,
    'replace',
  );
  assert.deepEqual(await readFile(destinationPath), updated);
});

test('retains stale files and records while merging a second platform', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t);
  await install({ platform: 'claude', target, catalogRoot });
  const stalePath = '.claude/skills/write-pr/references/guide.md';
  const staleBytes = await readFile(path.join(target, ...stalePath.split('/')));

  await rm(path.join(catalogRoot, 'skills/universal/write-pr/references/guide.md'));
  const staleReport = await install({ platform: 'claude', target, catalogRoot });
  assert.ok(staleReport.stale.includes(stalePath));
  assert.deepEqual(await readFile(path.join(target, ...stalePath.split('/'))), staleBytes);
  assert.ok((await readState(target)).files[stalePath]);

  await install({ platform: 'codex', target, catalogRoot });
  const merged = await readState(target);
  assert.equal(merged.platform, 'codex');
  assert.ok(merged.files['.claude/agents/builder.md']);
  assert.ok(merged.files['.codex/agents/builder.toml']);
  assert.ok(merged.files[stalePath]);
});

test('fails closed on malformed or incompatible state', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  for (const contents of [
    '{bad json',
    `${JSON.stringify({ schemaVersion: 2, platform: 'claude', files: {} })}\n`,
    `${JSON.stringify({
      schemaVersion: 1,
      platform: 'claude',
      catalogRevision: 'abc',
      files: { '../escape': { sha256: 'bad', source: '/absolute' } },
    })}\n`,
  ]) {
    const target = await createTarget(t, `state-${Math.random()}`);
    await mkdir(target, { recursive: true });
    await writeFile(path.join(target, STATE_FILE), contents);
    await assert.rejects(
      install({ platform: 'claude', target, catalogRoot }),
      /invalid install state|unsupported install state/,
    );
    assert.equal(await readFile(path.join(target, STATE_FILE), 'utf8'), contents);
    await assertMissing(path.join(target, '.claude/agents/builder.md'));
  }
});

test('rejects destination directories and symlinks even with force', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);

  const directoryTarget = await createTarget(t, 'directory');
  const destination = path.join(directoryTarget, '.claude/agents/builder.md');
  await mkdir(destination, { recursive: true });
  await assert.rejects(
    install({
      platform: 'claude',
      target: directoryTarget,
      catalogRoot,
      force: true,
    }),
    /destination is a directory.*\.claude\/agents\/builder\.md/s,
  );
  await assertMissing(path.join(directoryTarget, STATE_FILE));

  const symlinkTarget = await createTarget(t, 'symlink');
  const external = await createTarget(t, 'external');
  await mkdir(external, { recursive: true });
  await mkdir(path.join(symlinkTarget, '.claude'), { recursive: true });
  await symlink(external, path.join(symlinkTarget, '.claude/agents'));
  await assert.rejects(
    install({
      platform: 'claude',
      target: symlinkTarget,
      catalogRoot,
      force: true,
    }),
    /symlink.*\.claude\/agents/s,
  );
  assert.deepEqual(await listFiles(external), []);

  const leafTarget = await createTarget(t, 'leaf-symlink');
  const externalFile = path.join(external, 'external.md');
  await writeFile(externalFile, 'external\n');
  await mkdir(path.join(leafTarget, '.claude/agents'), { recursive: true });
  await symlink(externalFile, path.join(leafTarget, '.claude/agents/builder.md'));
  await assert.rejects(
    install({
      platform: 'claude',
      target: leafTarget,
      catalogRoot,
      force: true,
    }),
    /symlink.*\.claude\/agents\/builder\.md/s,
  );
  assert.equal(await readFile(externalFile, 'utf8'), 'external\n');
});

test('materializes an approved legacy Claude skill symlink only with migration enabled', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t, 'legacy-symlink');
  const legacySkill = path.join(target, '.agents/skills/write-pr');
  const legacySkillFile = path.join(legacySkill, 'SKILL.md');
  const claudeSkill = path.join(target, '.claude/skills/write-pr');

  await mkdir(legacySkill, { recursive: true });
  await writeFile(legacySkillFile, 'legacy skill\n');
  await mkdir(path.dirname(claudeSkill), { recursive: true });
  await symlink(
    path.relative(path.dirname(claudeSkill), legacySkill),
    claudeSkill,
  );

  await assert.rejects(
    install({ platform: 'claude', target, catalogRoot }),
    /parent path is a symlink.*\.claude\/skills\/write-pr/s,
  );

  const report = await install({
    platform: 'claude',
    target,
    catalogRoot,
    migrateLegacy: true,
  });

  assert.deepEqual(report.migrations, [{
    path: '.claude/skills/write-pr',
    target: '.agents/skills/write-pr',
  }]);
  assert.equal((await lstat(claudeSkill)).isSymbolicLink(), false);
  assert.deepEqual(
    await readFile(path.join(claudeSkill, 'SKILL.md')),
    FIXTURE_FILES['skills/universal/write-pr/SKILL.md'],
  );
  assert.equal(await readFile(legacySkillFile, 'utf8'), 'legacy skill\n');
});

test('materializes external legacy skill symlinks without writing through them', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t, 'external-legacy-symlink');
  const external = await createTarget(t, 'external-skill');
  const claudeSkill = path.join(target, '.claude/skills/write-pr');
  const externalFile = path.join(external, 'SKILL.md');

  await mkdir(path.dirname(claudeSkill), { recursive: true });
  await mkdir(external, { recursive: true });
  await writeFile(externalFile, 'external skill\n');
  await symlink(external, claudeSkill);

  const report = await install({
    platform: 'claude',
    target,
    catalogRoot,
    migrateLegacy: true,
  });

  assert.equal(report.migrations.length, 1);
  assert.equal((await lstat(claudeSkill)).isSymbolicLink(), false);
  assert.deepEqual(
    await readFile(path.join(claudeSkill, 'SKILL.md')),
    FIXTURE_FILES['skills/universal/write-pr/SKILL.md'],
  );
  assert.equal(await readFile(externalFile, 'utf8'), 'external skill\n');
});

test('dry-run reports legacy migrations without changing links or state', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t, 'dry-run-legacy-migration');
  const legacySkill = path.join(target, '.agents/skills/write-pr');
  const claudeSkill = path.join(target, '.claude/skills/write-pr');

  await mkdir(legacySkill, { recursive: true });
  await writeFile(path.join(legacySkill, 'SKILL.md'), 'legacy skill\n');
  await mkdir(path.dirname(claudeSkill), { recursive: true });
  await symlink(
    path.relative(path.dirname(claudeSkill), legacySkill),
    claudeSkill,
  );

  const report = await install({
    platform: 'claude',
    target,
    catalogRoot,
    dryRun: true,
    migrateLegacy: true,
  });

  assert.equal(report.migrations.length, 1);
  assert.equal((await lstat(claudeSkill)).isSymbolicLink(), true);
  assert.equal(await readFile(path.join(legacySkill, 'SKILL.md'), 'utf8'), 'legacy skill\n');
  await assertMissing(path.join(target, STATE_FILE));
});

test('rejects migration symlinks whose targets are not directories', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t, 'file-legacy-symlink');
  const externalFile = path.join(target, 'external-skill.md');
  const claudeSkill = path.join(target, '.claude/skills/write-pr');

  await mkdir(target, { recursive: true });
  await writeFile(externalFile, 'not a skill directory\n');
  await mkdir(path.dirname(claudeSkill), { recursive: true });
  await symlink(externalFile, claudeSkill);

  await assert.rejects(
    install({
      platform: 'claude',
      target,
      catalogRoot,
      migrateLegacy: true,
    }),
    /legacy migration target must be a directory/,
  );
  assert.equal((await lstat(claudeSkill)).isSymbolicLink(), true);
});

test('rejects broken migration symlinks', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t, 'broken-legacy-symlink');
  const claudeSkill = path.join(target, '.claude/skills/write-pr');

  await mkdir(path.dirname(claudeSkill), { recursive: true });
  await symlink('../missing-skill', claudeSkill);

  await assert.rejects(
    install({
      platform: 'claude',
      target,
      catalogRoot,
      migrateLegacy: true,
    }),
    /legacy migration target is unavailable/,
  );
  assert.equal((await lstat(claudeSkill)).isSymbolicLink(), true);
});

test('restores migrated symlinks when a migrated install fails', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t, 'failed-legacy-migration');
  const legacySkill = path.join(target, '.agents/skills/write-pr');
  const claudeSkill = path.join(target, '.claude/skills/write-pr');
  const failedPath = path.join(claudeSkill, 'SKILL.md');

  await mkdir(legacySkill, { recursive: true });
  await writeFile(path.join(legacySkill, 'SKILL.md'), 'legacy skill\n');
  await mkdir(path.dirname(claudeSkill), { recursive: true });
  await symlink(
    path.relative(path.dirname(claudeSkill), legacySkill),
    claudeSkill,
  );
  const fs = await import('node:fs/promises');

  await assert.rejects(
    install({
      platform: 'claude',
      target,
      catalogRoot,
      migrateLegacy: true,
      fsOps: {
        async rename(from, to) {
          if (to === failedPath) {
            throw new Error('injected migrated copy failure');
          }
          return fs.rename(from, to);
        },
      },
    }),
    /injected migrated copy failure/,
  );

  assert.equal((await lstat(claudeSkill)).isSymbolicLink(), true);
  assert.equal(await readFile(path.join(legacySkill, 'SKILL.md'), 'utf8'), 'legacy skill\n');
  assert.equal(await listFiles(target).then(
    (files) => files.some((file) => file.includes('.agents-catalog-legacy-')),
  ), false);
  await assertMissing(path.join(target, STATE_FILE));
});

test('late copy failure keeps old state, cleans temp files, and reports progress', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t);
  await install({ platform: 'claude', target, catalogRoot });
  const oldState = await readFile(path.join(target, STATE_FILE));
  const firstPath = '.claude/agents/builder.md';
  const failedPath = '.claude/skills/review-pr/SKILL.md';
  const firstUpdate = Buffer.from(
    FIXTURE_FILES['agents/builder/claude.md'].toString().replace('feature', 'updated feature'),
  );
  const failedUpdate = Buffer.from(
    FIXTURE_FILES['skills/claude/review-pr/SKILL.md'].toString().replace('Review it.', 'Updated.'),
  );
  await writeFixtureFile(catalogRoot, 'agents/builder/claude.md', firstUpdate);
  await writeFixtureFile(catalogRoot, 'skills/claude/review-pr/SKILL.md', failedUpdate);
  const fs = await import('node:fs/promises');

  await assert.rejects(
    install({
      platform: 'claude',
      target,
      catalogRoot,
      fsOps: {
        async rename(from, to) {
          if (to === path.join(target, ...failedPath.split('/'))) {
            throw new Error('injected late copy failure');
          }
          return fs.rename(from, to);
        },
      },
    }),
    (error) => {
      assert.match(error.message, /injected late copy failure/);
      assert.deepEqual(error.report.copied, [firstPath]);
      assert.equal(error.report.failed, failedPath);
      assert.ok(error.report.pending.length > 0);
      return true;
    },
  );

  assert.deepEqual(await readFile(path.join(target, ...firstPath.split('/'))), firstUpdate);
  assert.notDeepEqual(await readFile(path.join(target, ...failedPath.split('/'))), failedUpdate);
  assert.deepEqual(await readFile(path.join(target, STATE_FILE)), oldState);
  assert.deepEqual(await findTempDebris(target), []);
});

test('state-write failure leaves copied content with old state and no temp debris', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t);
  await install({ platform: 'codex', target, catalogRoot });
  const oldState = await readFile(path.join(target, STATE_FILE));
  const destination = '.codex/agents/builder.toml';
  const updated = Buffer.from(
    FIXTURE_FILES['agents/builder/codex.toml'].toString().replace('feature', 'state failure'),
  );
  await writeFixtureFile(catalogRoot, 'agents/builder/codex.toml', updated);
  const fs = await import('node:fs/promises');

  await assert.rejects(
    install({
      platform: 'codex',
      target,
      catalogRoot,
      fsOps: {
        async rename(from, to) {
          if (to === path.join(target, STATE_FILE)) {
            throw new Error('injected state failure');
          }
          return fs.rename(from, to);
        },
      },
    }),
    (error) => {
      assert.match(error.message, /injected state failure/);
      assert.ok(error.report.copied.includes(destination));
      assert.equal(error.report.failed, STATE_FILE);
      assert.deepEqual(error.report.pending, []);
      return true;
    },
  );

  assert.deepEqual(await readFile(path.join(target, ...destination.split('/'))), updated);
  assert.deepEqual(await readFile(path.join(target, STATE_FILE)), oldState);
  assert.deepEqual(await findTempDebris(target), []);
});

test('revision ignores untracked files and marks staged or unstaged tracked changes dirty', async (t) => {
  const { root: catalogRoot, revision } = await createCatalog(t);
  assert.equal(await getCatalogRevision(catalogRoot), revision);

  await writeFile(path.join(catalogRoot, 'untracked.txt'), 'ignored\n');
  assert.equal(await getCatalogRevision(catalogRoot), revision);

  const tracked = path.join(catalogRoot, 'skills/_shared/common.md');
  await writeFile(tracked, '# Dirty\n');
  assert.equal(await getCatalogRevision(catalogRoot), `${revision}-dirty`);

  const { stdout: blob } = await runGit(
    catalogRoot,
    ['hash-object', '-w', 'skills/_shared/common.md'],
  );
  await runGit(catalogRoot, [
    'update-index',
    '--cacheinfo',
    `100644,${blob.trim()},skills/_shared/common.md`,
  ]);
  assert.equal(await getCatalogRevision(catalogRoot), `${revision}-dirty`);
});

test('state paths are POSIX-style and catalog validation errors create nothing', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t);
  await install({ platform: 'codex', target, catalogRoot });
  const state = await readState(target);
  for (const [destination, metadata] of Object.entries(state.files)) {
    assert.equal(destination.includes('\\'), false);
    assert.equal(metadata.source.includes('\\'), false);
  }

  const invalid = await createCatalog(t, {
    'agents/builder/codex.toml': Buffer.from(
      FIXTURE_FILES['agents/builder/codex.toml']
        .toString()
        .replace('openai.gpt-5.6-luna', 'openai.gpt-wrong'),
    ),
  });
  const invalidTarget = await createTarget(t, 'invalid');
  await assert.rejects(
    install({
      platform: 'codex',
      target: invalidTarget,
      catalogRoot: invalid.root,
    }),
    /catalog validation failed.*does not match profile/s,
  );
  await assertMissing(invalidTarget);
});

test('Codex unowned collisions require force', async (t) => {
  const { root: catalogRoot } = await createCatalog(t);
  const target = await createTarget(t, 'codex-collision');
  const collision = path.join(target, '.agents/skills/write-pr/SKILL.md');
  await mkdir(path.dirname(collision), { recursive: true });
  await writeFile(collision, 'local version\n');

  await assert.rejects(
    install({ platform: 'codex', target, catalogRoot }),
    /unowned destination differs.*\.agents\/skills\/write-pr\/SKILL\.md/s,
  );

  await install({ platform: 'codex', target, catalogRoot, force: true });
  assert.deepEqual(
    await readFile(collision),
    FIXTURE_FILES['skills/universal/write-pr/SKILL.md'],
  );
});

test('parses the exact CLI and rejects invalid, duplicate, or missing arguments', () => {
  assert.deepEqual(parseArgs(['claude']), {
    platform: 'claude',
    scope: 'user',
    target: undefined,
    dryRun: false,
    force: false,
    migrateLegacy: false,
  });
  assert.deepEqual(
    parseArgs([
      'claude',
      '--scope',
      'project',
      '--target',
      './out',
      '--dry-run',
      '--force',
      '--migrate-legacy',
    ]),
    {
      platform: 'claude',
      scope: 'project',
      target: './out',
      dryRun: true,
      force: true,
      migrateLegacy: true,
    },
  );
  for (const args of [
    [],
    ['kiro'],
    ['--force', 'claude'],
    ['claude', 'codex'],
    ['claude', '--scope'],
    ['claude', '--scope', 'team'],
    ['claude', '--scope', 'user', '--scope', 'project'],
    ['claude', '--target'],
    ['claude', '--target', 'a', '--target', 'b'],
    ['claude', '--dry-run', '--dry-run'],
    ['claude', '--force', '--force'],
    ['claude', '--migrate-legacy', '--migrate-legacy'],
    ['codex', '--migrate-legacy'],
    ['claude', '--unknown'],
  ]) {
    assert.throws(() => parseArgs(args), { message: USAGE });
  }
});

test('CLI prints usage and exits nonzero for invalid arguments', async () => {
  await assert.rejects(
    execFileAsync(process.execPath, [INSTALL_SCRIPT, 'claude', '--scope'], {
      cwd: REPOSITORY_ROOT,
    }),
    (error) => {
      assert.notEqual(error.code, 0);
      assert.equal(error.stdout, '');
      assert.equal(error.stderr, `${USAGE}\n`);
      return true;
    },
  );
});
