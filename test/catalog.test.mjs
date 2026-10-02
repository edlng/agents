import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import {
  mkdtemp,
  mkdir,
  readdir,
  readFile,
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
  buildInstallSet,
  loadCatalog,
  loadModelPolicy,
  parseClaudeAgent,
  parseCodexAgent,
  sha256,
  validateCatalog,
} from '../scripts/catalog-lib.mjs';

const POLICY = {
  minimumClaudeEffort: 'medium',
  profiles: {
    haiku: {
      claude: { model: 'haiku', effort: 'medium' },
      codex: { model: 'openai.gpt-5.6-luna', effort: 'xhigh' },
    },
    sonnet: {
      claude: { model: 'sonnet', effort: 'medium' },
      codex: { model: 'openai.gpt-5.6-luna', effort: 'xhigh' },
    },
    opus: {
      claude: { model: 'opus', effort: 'medium' },
      codex: { model: 'openai.gpt-5.6-sol', effort: 'high' },
    },
  },
};

const DESCRIPTION = 'Worker agent that executes one scoped implementation task.';
const REPOSITORY_ROOT = fileURLToPath(new URL('..', import.meta.url));
const execFileAsync = promisify(execFile);

const AGENT_MATRIX = [
  ['builder', 'implementation', 'sonnet', 'workspace-write'],
  ['code-reviewer', 'quality-assurance', 'sonnet', 'read-only'],
  ['explore', 'research', 'haiku', 'read-only'],
  ['photon-lead', 'orchestration', 'opus', 'workspace-write'],
  ['research-validator', 'research', 'sonnet', 'read-only'],
  ['researcher', 'research', 'sonnet', 'read-only'],
  ['tester', 'quality-assurance', 'sonnet', 'workspace-write'],
  ['validator', 'quality-assurance', 'opus', 'read-only'],
].map(([name, category, profile, sandbox]) => ({
  name,
  category,
  profile,
  sandbox,
}));


const UNIVERSAL_SKILL_NAMES = [
  'code-review-excellence',
  'onboard',
  'review-addressed-comments',
  'update-sysprompts',
  'write-pr',
];

const SUPERPOWERS_SKILL_NAMES = new Set([
  'brainstorming',
  'dispatching-parallel-agents',
  'executing-plans',
  'finishing-a-development-branch',
  'receiving-code-review',
  'requesting-code-review',
  'subagent-driven-development',
  'systematic-debugging',
  'test-driven-development',
  'using-git-worktrees',
  'using-superpowers',
  'verification-before-completion',
  'writing-plans',
  'writing-skills',
]);

const UNIVERSAL_SOURCE_SNAPSHOT = [
  'code-review-excellence/SKILL.md a3b2b01ad6d0b26eb402014f7734fe3fb9d98f7809cb386ccdf6fe2d4cc82604 0644',
  'onboard/references/diagram-style.md 50e42bca32ee64e20dd9d134711e6c1843d2886c9c98452b4dcf2f535c3e57de 0644',
  'onboard/references/guide-template.md fcd69ad8100d0ec0db71b90391c7c078cc6d103c81c98822aa25e36a4692cdfd 0644',
  'onboard/references/icons/app-window.svg 2c0e089eaada4cdae6395c570100a8ebffea08f19636e0b1d53b773b9c5b58ac 0644',
  'onboard/references/icons/boxes.svg 87786d80f6eca2602aad5a6a5d608dc3386219ba71f3cf2746e69b2bb99e7214 0644',
  'onboard/references/icons/braces.svg ba2f6df733ad2a2833f935681cf6b0c29fe4502b39aa5e2b4f38154789d0e81d 0644',
  'onboard/references/icons/cloud.svg 3b31d710d15ed6ea4cb0cea314a6541a7c42f00e7af1a3028e0cb83539c4217f 0644',
  'onboard/references/icons/code-xml.svg b16eb571e2cc579be8a0158bb61b05211774a605b892d1bd3a250a958f4dd6c6 0644',
  'onboard/references/icons/container.svg ef237ab205bbe0d294896ac20252c89789c8a54c5701d2efb6b84cf296c1fa3b 0644',
  'onboard/references/icons/database.svg 813151983345d0c31316029f91453d01d76cc6ccaa38904bd51f5f41e7427441 0644',
  'onboard/references/icons/globe.svg e08a82c0e9862b2a3e4003d14c2c517398ca5776e7e04db2756401d50e734d59 0644',
  'onboard/references/icons/key-round.svg 619d0e31f5215bca00e63e0c4d19192f822904cd1c787611d95a1823a4a7d7fc 0644',
  'onboard/references/icons/LICENSE b495047bd93a9b06913511076f504daba17d5bbeb3e0650f3bb53a4220329c57 0644',
  'onboard/references/icons/lock-keyhole.svg 47f99e5eb871e7c54b047c7987e3af39d06928123a75efa12e950e31b55a2b5d 0644',
  'onboard/references/icons/MANIFEST.md 4d42601ab803763b4ae9920061a6c805d18066e593f35cc9950e7f09c460ee10 0644',
  'onboard/references/icons/server.svg e8c4f808587df6c7790336fdf4e5d462d164f12f70ba3d781e0f06d3ae2dc27c 0644',
  'onboard/references/icons/shield-check.svg d7cfb1de96312a72b987a75cf8ac23aa9518e1665e043120e50a6792b3665ceb 0644',
  'onboard/references/icons/users.svg e7736ae816f2cc2c1a105113b55d3f9ac53b59d98e776020898a7bfd235c7d64 0644',
  'onboard/references/icons/webhook.svg d387f07f01327fde0a402eb73837b653ede8d8f802cf3c6256ec8ddd1945899e 0644',
  'onboard/references/icons/workflow.svg 7c4cec1ddefdfb48369b8db25bc56155da880f813101df324effd574dd58c796 0644',
  'onboard/SKILL.md 4cdc43e9fd771fb8f2ab3debd88ae3698c0e9858a7dddcdcf0557f29610aa1f6 0644',
  'review-addressed-comments/SKILL.md 0322a2d8b8593cf09b15bbda4dceb823f9eadec28a1a94bae3df08f8e1db235f 0644',
  'update-sysprompts/SKILL.md 65289f52ff11071345d06eb7366d1062d2935618f6e79bf934a6836958d5387b 0644',
  'write-pr/SKILL.md 76e561e0e02b75c334441d5b36940efe9392ff74597391a065b22e53964d3f32 0644',
].map((snapshot) => {
  const [relativePath, hash, mode] = snapshot.split(' ');
  return {
    relativePath,
    hash,
    mode: Number.parseInt(mode, 8),
  };
});

const INTENTIONAL_UNIVERSAL_HASHES = new Map();
const CONCRETE_PROVIDER_ID =
  /\b(?:claude-(?:haiku|sonnet|opus)-[\w.-]+|openai\.gpt-[\w.-]+)\b/i;

const KIRO_PROMPT_SHA256 = {
  builder: '340286b6f9b8cc69ed680fc2b9a06a887cd02f0ff2f3b012d882ec8dde09f907',
  'code-reviewer': '715d934faf08f5122bd8b9f7f9c181f85d1c59d4492e940f3db43ec45ced86e5',
  explore: '2f2a85ea295368ea8a84055ee0c0857f98ff300078826f6774fafac76d2d4cd6',
  'photon-lead': '6f8a573d678e5eccf1652ea080d1a2d1c2bb467ddd8f14165285d050f2f77d0c',
  'research-validator': 'c0b17309751b84a549e5587cd5cfdbc2be897ac74d8259948ad12ecd36024dfe',
  researcher: '0adcd287058b3e998032463dd41c43aaadfd5eddef413030cd5f5ef636a1a263',
  tester: '0f2e6019409d2d404afc08bdcc5420a126b57bbe3f73765d6cb748796cbd0a45',
  validator: '18e629fb194c2f266abc01c5d6d6c3111fc4c7a7be1eda5d89e702ed88163b23',
};

const KIRO_JSON_SHA256 = {
  builder: 'f455232eb0cbdbe8c00d93fe6e8f9970fddd1cd3651abe00a8784df887fda078',
  'code-reviewer': 'da5a22b3cec3c0cd30fb63506b2f9530c2050d23ab749ecbc000b172cfc1feae',
  explore: '423830a5ad8a5ac9e0a99a2d1c1f7d0be8f27e55a2c5678e51dc97babe22febc',
  'photon-lead': '4ab967e132f6d932314e23fd72bfc2bd27d9d0f7f30878123836c49399d850e6',
  'research-validator': '36bddb25df064a14c45daef63f658e1823c04aafb82537abfa90cb469c93d30c',
  researcher: 'd234a3721fe7a6b666baf92fdcfb3b5512f3225a1d4076f21c1338882fe3a2e9',
  tester: '2dadf5d1b11dd5d24360f5d3a82cc22cb56574a8ed17cceebd62fe0764557cc8',
  validator: 'a34ec784b7e897149778e023ad0bcfe1c8a631acda9b8d6c428282723a249c5a',
};

const FIXTURE_FILES = {
  'platforms/model-policy.json': `${JSON.stringify(POLICY, null, 2)}\n`,
  'agents/builder/manifest.json': `${JSON.stringify({
    name: 'builder',
    description: DESCRIPTION,
    category: 'implementation',
    profile: 'sonnet',
    platforms: ['claude', 'codex', 'kiro'],
  }, null, 2)}\n`,
  'agents/builder/claude.md': `---
name: builder
description: ${DESCRIPTION}
model: sonnet
effort: medium
tools:
  - Read
  - Write
metadata:
  owner: catalog
---

Implement the scoped task.
`,
  'agents/builder/codex.toml': `name = "builder"
description = "${DESCRIPTION}"
model = "openai.gpt-5.6-luna"
model_reasoning_effort = "xhigh"
sandbox_mode = "workspace-write"
developer_instructions = """
Implement the scoped task.
"""
`,
  'skills/universal/write-pr/SKILL.md': `---
name: write-pr
description: >-
  Write a pull request description from the current changes.
compatibility:
  - claude
  - codex
metadata:
  source: catalog
---

Read [the local guide](references/guide.md), \`_shared/common.md\`, and
\`../_shared/common.md\`.
`,
  'skills/universal/write-pr/references/guide.md': '# Guide\n',
  'skills/claude/review-pr/SKILL.md': `---
name: review-pr
description: Review a pull request with Claude-native tools.
---

Review the pull request.
`,
  'skills/codex/review-pr/SKILL.md': `---
name: review-pr
description: Review a pull request with Codex-native tools.
---

Review the pull request.
`,
  'skills/_shared/common.md': '# Shared guidance\n',
};

async function writeFixtureFile(root, relativePath, contents) {
  const absolutePath = path.join(root, relativePath);
  await mkdir(path.dirname(absolutePath), { recursive: true });
  await writeFile(absolutePath, contents);
}

async function createFixture(
  t,
  overrides = {},
  prefix = path.join(tmpdir(), 'agents-catalog-'),
) {
  const root = await mkdtemp(prefix);
  t.after(() => rm(root, { recursive: true, force: true }));

  const files = { ...FIXTURE_FILES, ...overrides };
  for (const [relativePath, contents] of Object.entries(files)) {
    if (contents !== null) {
      await writeFixtureFile(root, relativePath, contents);
    }
  }
  return root;
}

async function createCompleteCliFixture(t) {
  const root = await createFixture(
    t,
    {},
    path.join(REPOSITORY_ROOT, 'test', '.catalog-cli-'),
  );

  for (let index = 1; index <= 7; index += 1) {
    const name = `agent-${String(index).padStart(2, '0')}`;
    const description = `Fixture agent ${index}.`;
    await writeFixtureFile(root, `agents/${name}/manifest.json`, `${JSON.stringify({
      name,
      description,
      category: 'testing',
      profile: 'haiku',
      platforms: ['claude', 'codex'],
    }, null, 2)}\n`);
    await writeFixtureFile(root, `agents/${name}/claude.md`, `---
name: ${name}
description: ${description}
model: haiku
effort: medium
---

Test the fixture.
`);
    await writeFixtureFile(root, `agents/${name}/codex.toml`, `name = "${name}"
description = "${description}"
model = "openai.gpt-5.6-luna"
model_reasoning_effort = "xhigh"
sandbox_mode = "read-only"
developer_instructions = "Test the fixture."
`);
  }

  for (let index = 1; index <= 4; index += 1) {
    const name = `universal-${String(index).padStart(2, '0')}`;
    await writeFixtureFile(root, `skills/universal/${name}/SKILL.md`, `---
name: ${name}
description: Universal fixture skill ${index}.
---
`);
  }

  for (let index = 1; index <= 7; index += 1) {
    const name = `platform-${String(index).padStart(2, '0')}`;
    for (const platform of ['claude', 'codex']) {
      await writeFixtureFile(root, `skills/${platform}/${name}/SKILL.md`, `---
name: ${name}
description: ${platform} fixture skill ${index}.
---
`);
    }
  }

  for (const script of ['catalog-lib.mjs', 'validate-catalog.mjs']) {
    await writeFixtureFile(
      root,
      `scripts/${script}`,
      await readFile(path.join(REPOSITORY_ROOT, 'scripts', script), 'utf8'),
    );
  }
  return root;
}

async function createTestSymlink(t, target, linkPath, type) {
  try {
    await symlink(target, linkPath, type);
    return true;
  } catch (error) {
    if (error.code === 'EPERM' || error.code === 'EACCES') {
      t.skip(`symlink creation unavailable: ${error.code}`);
      return false;
    }
    throw error;
  }
}

function messagesFor(catalog, policy = POLICY) {
  return validateCatalog(catalog, policy);
}

function canonicalizeJson(value) {
  if (Array.isArray(value)) {
    return value.map(canonicalizeJson);
  }
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, canonicalizeJson(value[key])]),
    );
  }
  return value;
}

test('production agent matrix has complete native families and retained Kiro behavior', async () => {
  const catalog = await loadCatalog(REPOSITORY_ROOT);
  const policy = await loadModelPolicy(REPOSITORY_ROOT);
  const expectedNames = AGENT_MATRIX.map(({ name }) => name);

  assert.equal(catalog.agents.length, 8);
  assert.deepEqual(
    catalog.agents.map(({ manifest }) => manifest.name),
    expectedNames,
  );

  for (const expected of AGENT_MATRIX) {
    const agent = catalog.agents.find(
      ({ manifest }) => manifest.name === expected.name,
    );
    assert.ok(agent, `${expected.name}: missing agent family`);

    assert.equal(agent.manifest.category, expected.category);
    assert.equal(agent.manifest.profile, expected.profile);
    assert.deepEqual(agent.manifest.platforms, ['claude', 'codex', 'kiro']);

    assert.equal(agent.claude.name, agent.manifest.name);
    assert.equal(agent.claude.description, agent.manifest.description);
    assert.equal(
      agent.claude.model,
      policy.profiles[expected.profile].claude.model,
    );
    assert.equal(
      agent.claude.effort,
      policy.profiles[expected.profile].claude.effort,
    );
    assert.ok(
      agent.claude.instructions.trim().length > 0,
      `${expected.name}: Claude instructions must not be empty`,
    );
    assert.doesNotMatch(
      agent.claude.instructions,
      /~\/\.kiro|_shared\/security-constraints/,
      `${expected.name}: Claude instructions contain unresolved Kiro paths`,
    );

    assert.equal(agent.codex.name, agent.manifest.name);
    assert.equal(agent.codex.description, agent.manifest.description);
    assert.equal(
      agent.codex.model,
      policy.profiles[expected.profile].codex.model,
    );
    assert.equal(
      agent.codex.model_reasoning_effort,
      policy.profiles[expected.profile].codex.effort,
    );
    assert.equal(agent.codex.sandbox_mode, expected.sandbox);
    assert.ok(
      agent.codex.developer_instructions.trim().length > 0,
      `${expected.name}: Codex instructions must not be empty`,
    );
    assert.doesNotMatch(
      agent.codex.developer_instructions,
      /~\/\.kiro|_shared\/security-constraints|mcp__|firecrawl_(?:search|scrape|map|extract)|\|\s*Agent\s*\|\s*Role\s*\|\s*Tools|\b(?:Haiku|Sonnet|Opus)\b|claude-(?:haiku|sonnet|opus)-[\w.-]+/i,
      `${expected.name}: Codex instructions contain provider-specific vocabulary`,
    );

    const familyDirectory = path.join(
      REPOSITORY_ROOT,
      'agents',
      expected.name,
    );
    const kiro = JSON.parse(await readFile(
      path.join(familyDirectory, 'kiro.json'),
      'utf8',
    ));
    const kiroPrompt = await readFile(path.join(
      familyDirectory,
      'kiro-prompt.md',
    ));
    assert.equal(kiro.name, expected.name);
    assert.ok(
      kiroPrompt.toString('utf8').trim().length > 0,
      `${expected.name}: retained Kiro prompt must not be empty`,
    );
    assert.equal(
      sha256(kiroPrompt),
      KIRO_PROMPT_SHA256[expected.name],
      `${expected.name}: retained Kiro prompt bytes changed`,
    );
    assert.equal(kiro.prompt, 'file://./kiro-prompt.md');
    kiro.prompt = `file://./${expected.name}-prompt.md`;
    assert.equal(
      sha256(JSON.stringify(canonicalizeJson(kiro))),
      KIRO_JSON_SHA256[expected.name],
      `${expected.name}: retained Kiro JSON behavior changed`,
    );
  }

  const flatAgentFiles = (await readdir(path.join(REPOSITORY_ROOT, 'agents'), {
    withFileTypes: true,
  }))
    .filter((entry) => entry.isFile())
    .map(({ name }) => name)
    .filter((name) => name.endsWith('.json') || name.endsWith('-prompt.md'));
  assert.deepEqual(flatAgentFiles, []);
});


test('does not distribute skills sourced from obra/superpowers', async () => {
  for (const relativeRoot of ['skills/universal', 'skills/claude', 'skills/codex']) {
    const entries = await readdir(path.join(REPOSITORY_ROOT, relativeRoot), {
      withFileTypes: true,
    });
    const importedSkills = entries
      .filter((entry) => entry.isDirectory() && SUPERPOWERS_SKILL_NAMES.has(entry.name))
      .map((entry) => entry.name)
      .sort();

    assert.deepEqual(importedSkills, [], `${relativeRoot} still contains Superpowers skills`);
  }
});

test('production universal skills preserve the complete source snapshot for both platforms', async () => {
  const catalog = await loadCatalog(REPOSITORY_ROOT);
  const policy = await loadModelPolicy(REPOSITORY_ROOT);
  const universal = catalog.skillVariants.universal;

  assert.deepEqual(
    universal.map(({ name }) => name),
    UNIVERSAL_SKILL_NAMES,
  );
  assert.equal(
    universal.reduce((count, variant) => count + variant.files.length, 0),
    24,
  );

  const expectedFiles = UNIVERSAL_SOURCE_SNAPSHOT.map((source) => ({
    ...source,
    relativePath: source.relativePath,
    hash: INTENTIONAL_UNIVERSAL_HASHES.get(source.relativePath) ?? source.hash,
  })).sort((left, right) => left.relativePath.localeCompare(right.relativePath));
  const actualFiles = [];
  for (const variant of universal) {
    for (const file of variant.files) {
      const relativePath = `${variant.name}/${file.name}`;
      assert.doesNotMatch(
        file.contents.toString('utf8'),
        CONCRETE_PROVIDER_ID,
        `${relativePath}: contains a concrete provider model ID`,
      );
      actualFiles.push({
        relativePath,
        hash: sha256(file.contents),
        mode: (await stat(file.path)).mode & 0o777,
      });
    }
  }
  actualFiles.sort((left, right) => left.relativePath.localeCompare(right.relativePath));
  assert.deepEqual(actualFiles, expectedFiles);

  const universalErrors = validateCatalog(catalog, policy).filter(
    (error) => error.includes('universal skill')
      || error.startsWith('skills/universal/'),
  );
  assert.deepEqual(universalErrors, []);

  const installFiles = (platform) => buildInstallSet(catalog, platform).skills
    .filter(({ name }) => UNIVERSAL_SKILL_NAMES.includes(name))
    .flatMap((skill) => skill.files.map((file) => ({
      skill: skill.name,
      name: file.name,
      source: path.relative(REPOSITORY_ROOT, file.path).split(path.sep).join('/'),
      hash: sha256(file.contents),
      mode: file.mode,
    })))
    .sort((left, right) => left.source.localeCompare(right.source));
  const claudeFiles = installFiles('claude');
  const codexFiles = installFiles('codex');
  const expectedInstallFiles = expectedFiles
    .map((file) => {
      const [skill, ...nameParts] = file.relativePath.split('/');
      return {
        skill,
        name: nameParts.join('/'),
        source: `skills/universal/${file.relativePath}`,
        hash: file.hash,
        mode: file.mode,
      };
    })
    .sort((left, right) => left.source.localeCompare(right.source));
  assert.equal(claudeFiles.length, 24);
  assert.deepEqual(claudeFiles, expectedInstallFiles);
  assert.deepEqual(claudeFiles, codexFiles);

  for (const name of UNIVERSAL_SKILL_NAMES) {
    if (name === 'update-sysprompts') {
      assert.deepEqual(
        await readFile(
          path.join(REPOSITORY_ROOT, 'skills', name, 'SKILL.md'),
          'utf8',
        ),
        await readFile(
          path.join(REPOSITORY_ROOT, 'skills', 'universal', name, 'SKILL.md'),
          'utf8',
        ),
        'update-sysprompts direct sync source must match the catalog source',
      );
      continue;
    }
    await assert.rejects(
      stat(path.join(REPOSITORY_ROOT, 'skills', name)),
      { code: 'ENOENT' },
      `${name}: legacy source directory still exists`,
    );
  }
});

test('loads the exact model policy and a complete catalog fixture', async (t) => {
  const root = await createFixture(t);

  const policy = await loadModelPolicy(REPOSITORY_ROOT);
  const catalog = await loadCatalog(root);

  assert.deepEqual(policy, POLICY);
  assert.equal(catalog.agents.length, 1);
  assert.equal(catalog.skills.length, 2);
  assert.deepEqual(
    catalog.skillVariants,
    {
      universal: [catalog.skills[1].variants.universal],
      claude: [catalog.skills[0].variants.claude],
      codex: [catalog.skills[0].variants.codex],
    },
  );
  assert.deepEqual(validateCatalog(catalog, policy), []);
});

test('parses native agent formats and builds platform install sets', async (t) => {
  const root = await createFixture(t);
  const claudePath = path.join(root, 'agents/builder/claude.md');
  const codexPath = path.join(root, 'agents/builder/codex.toml');
  const claude = parseClaudeAgent(await readFile(claudePath, 'utf8'), claudePath);
  const codex = parseCodexAgent(await readFile(codexPath, 'utf8'), codexPath);
  const catalog = await loadCatalog(root);

  assert.equal(claude.model, POLICY.profiles.sonnet.claude.model);
  assert.deepEqual(claude.tools, ['Read', 'Write']);
  assert.match(claude.instructions, /Implement the scoped task/);
  assert.equal(codex.model_reasoning_effort, 'xhigh');
  assert.match(codex.developer_instructions, /Implement the scoped task/);
  assert.equal(sha256('catalog'), '652f55016243bf1b9f1bbea46d5749ef892dbe394e46de9d66ab1aacf0b4af57');

  const claudeSet = buildInstallSet(catalog, 'claude');
  const codexSet = buildInstallSet(catalog, 'codex');
  assert.equal(claudeSet.agents[0].path, claudePath);
  assert.equal(codexSet.agents[0].path, codexPath);
  assert.deepEqual(claudeSet.skills.map(({ name }) => name), ['review-pr', 'write-pr']);
  assert.deepEqual(codexSet.skills.map(({ name }) => name), ['review-pr', 'write-pr']);
  assert.equal(
    claudeSet.skills.find(({ name }) => name === 'write-pr').path,
    codexSet.skills.find(({ name }) => name === 'write-pr').path,
  );
  assert.equal(claudeSet.sharedFiles.length, 1);
  assert.throws(() => buildInstallSet(catalog, 'kiro'), /unsupported platform "kiro"/);
});

test('rejects Claude effort below the configured minimum', async (t) => {
  const root = await createFixture(t, {
    'agents/builder/claude.md': FIXTURE_FILES['agents/builder/claude.md'].replace(
      'effort: medium',
      'effort: low',
    ),
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes('builder: Claude effort "low" is below required "medium"'));
});

test('rejects universal skill names duplicated in either platform install set', async (t) => {
  const root = await createFixture(t, {
    'skills/universal/review-pr/SKILL.md': `---
name: review-pr
description: A duplicate universal review skill.
---
`,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes('skill "review-pr" exists in universal and claude'));
  assert.ok(errors.includes('skill "review-pr" exists in universal and codex'));
});

test('reports malformed YAML frontmatter with its source path', async (t) => {
  const root = await createFixture(t, {
    'agents/builder/claude.md': `---
name: [builder
---
Broken YAML.
`,
  });

  await assert.rejects(
    () => loadCatalog(root),
    /agents[/\\]builder[/\\]claude\.md: invalid YAML frontmatter/,
  );
});

test('reports malformed TOML with its source path', async (t) => {
  const root = await createFixture(t, {
    'agents/builder/codex.toml': 'name = "builder"\nmodel = [\n',
  });

  await assert.rejects(
    () => loadCatalog(root),
    /agents[/\\]builder[/\\]codex\.toml: invalid TOML/,
  );
});

test('rejects native metadata and models that differ from the manifest policy', async (t) => {
  const root = await createFixture(t, {
    'agents/builder/claude.md': FIXTURE_FILES['agents/builder/claude.md'].replace(
      `description: ${DESCRIPTION}`,
      'description: Different description.',
    ),
    'agents/builder/codex.toml': FIXTURE_FILES['agents/builder/codex.toml'].replace(
      'openai.gpt-5.6-luna',
      'openai.gpt-5.6-sol',
    ),
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes('builder: Claude description does not match manifest'));
  assert.ok(errors.includes(
    'builder: Codex model "openai.gpt-5.6-sol" does not match profile "sonnet" model "openai.gpt-5.6-luna"',
  ));
});

test('reports missing provider sections in a recognized model profile', async (t) => {
  const root = await createFixture(t);
  const policy = structuredClone(POLICY);
  delete policy.profiles.sonnet.claude;
  delete policy.profiles.sonnet.codex;

  const errors = messagesFor(await loadCatalog(root), policy);
  assert.ok(errors.includes('model profile "sonnet": missing Claude policy'));
  assert.ok(errors.includes('model profile "sonnet": missing Codex policy'));
});

test('reports missing required fields in recognized model provider policies', async (t) => {
  const root = await createFixture(t);
  const policy = structuredClone(POLICY);
  policy.profiles.sonnet.claude = {};
  policy.profiles.sonnet.codex = {};

  const errors = messagesFor(await loadCatalog(root), policy);
  assert.ok(errors.includes(
    'model profile "sonnet": Claude policy requires a non-empty model',
  ));
  assert.ok(errors.includes(
    'model profile "sonnet": Claude policy requires a non-empty effort',
  ));
  assert.ok(errors.includes(
    'model profile "sonnet": Codex policy requires a non-empty model',
  ));
  assert.ok(errors.includes(
    'model profile "sonnet": Codex policy requires a non-empty effort',
  ));
});

test('rejects concrete provider model IDs in universal skills', async (t) => {
  const root = await createFixture(t, {
    'skills/universal/write-pr/SKILL.md':
      `${FIXTURE_FILES['skills/universal/write-pr/SKILL.md']}\nUse global.anthropic.claude-opus-4-8 here.\n`,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes('universal skill "write-pr" contains concrete provider model ID "global.anthropic.claude-opus-4-8"'));
});

test('rejects invalid skill names and frontmatter name mismatches', async (t) => {
  const root = await createFixture(t, {
    'skills/claude/Bad_Name/SKILL.md': `---
name: another-name
description: Invalid skill metadata.
---
`,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes('skills/claude/Bad_Name/SKILL.md: skill name "Bad_Name" is invalid'));
  assert.ok(errors.includes(
    'skills/claude/Bad_Name/SKILL.md: frontmatter name "another-name" does not match directory "Bad_Name"',
  ));
});

test('rejects absolute and escaping skill references', async (t) => {
  const root = await createFixture(t, {
    'skills/codex/review-pr/SKILL.md': `---
name: review-pr
description: Review a pull request with unsafe references.
---

Read [an absolute file](/etc/passwd) and [an escaping file](../../secrets.md).
`,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes(
    'skills/codex/review-pr/SKILL.md: unsafe absolute reference "/etc/passwd"',
  ));
  assert.ok(errors.includes(
    'skills/codex/review-pr/SKILL.md: reference "../../secrets.md" escapes the skill or shared directory',
  ));
});

test('rejects Windows drive-letter and UNC absolute skill references', async (t) => {
  const root = await createFixture(t, {
    'skills/codex/review-pr/SKILL.md': `---
name: review-pr
description: Review a pull request with unsafe Windows references.
---

Read [a drive path](C:\\temp\\guide.md) and
[a UNC path](\\\\server\\share\\guide.md).
`,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes(
    'skills/codex/review-pr/SKILL.md: unsafe absolute reference "C:\\temp\\guide.md"',
  ));
  assert.ok(errors.includes(
    'skills/codex/review-pr/SKILL.md: unsafe absolute reference "\\\\server\\share\\guide.md"',
  ));
});

test('rejects Windows-style escaping skill references', async (t) => {
  const root = await createFixture(t, {
    'skills/codex/review-pr/SKILL.md': `---
name: review-pr
description: Review a pull request with an escaping Windows reference.
---

Read [an escaping file](..\\..\\secrets.md).
`,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes(
    'skills/codex/review-pr/SKILL.md: reference "..\\..\\secrets.md" escapes the skill or shared directory',
  ));
});

test('rejects case-insensitive file URIs and Windows drive-relative references', async (t) => {
  const root = await createFixture(t, {
    'skills/codex/review-pr/SKILL.md': `---
name: review-pr
description: Review a pull request with unsafe URI-like references.
---

Read [a file URI](FILE:/etc/passwd) and
[a drive-relative path](C:..\\..\\secret.md).
`,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes(
    'skills/codex/review-pr/SKILL.md: unsafe absolute reference "FILE:/etc/passwd"',
  ));
  assert.ok(errors.includes(
    'skills/codex/review-pr/SKILL.md: unsafe absolute reference "C:..\\..\\secret.md"',
  ));
});

test('ignores inline code examples that are not supported relative file references', async (t) => {
  const root = await createFixture(t, {
    'skills/universal/write-pr/SKILL.md': `---
name: write-pr
description: A skill with command and API examples.
---

Use \`/\`, \`/api/v1/pulls\`, and \`/tmp/review-output.md\` as examples.
`,
  });

  assert.deepEqual(messagesFor(await loadCatalog(root)), []);
});

test('preserves an in-tree symlinked auxiliary skill file', async (t) => {
  const root = await createFixture(t);
  const linkPath = path.join(
    root,
    'skills/universal/write-pr/references/linked-guide.md',
  );
  if (!await createTestSymlink(t, 'guide.md', linkPath)) {
    return;
  }

  const catalog = await loadCatalog(root);
  const writePr = catalog.skillVariants.universal.find(
    ({ name }) => name === 'write-pr',
  );
  const linkedGuide = writePr.files.find(
    ({ name }) => name === 'references/linked-guide.md',
  );

  assert.ok(linkedGuide);
  assert.equal(linkedGuide.contents.toString('utf8'), '# Guide\n');
});

test('rejects an out-of-tree symlinked auxiliary skill file', async (t) => {
  const root = await createFixture(t);
  const externalRoot = await mkdtemp(path.join(tmpdir(), 'agents-catalog-external-'));
  t.after(() => rm(externalRoot, { recursive: true, force: true }));
  const externalPath = path.join(externalRoot, 'secret.md');
  await writeFile(externalPath, '# External secret\n');
  const linkPath = path.join(
    root,
    'skills/universal/write-pr/references/escape.md',
  );
  if (!await createTestSymlink(t, externalPath, linkPath)) {
    return;
  }

  await assert.rejects(
    () => loadCatalog(root),
    /skills[/\\]universal[/\\]write-pr[/\\]references[/\\]escape\.md: resolved path escapes owning directory/,
  );
});

test('rejects an out-of-tree symlinked required catalog input', async (t) => {
  const root = await createFixture(t, {
    'skills/codex/review-pr/SKILL.md': null,
  });
  const externalRoot = await mkdtemp(path.join(tmpdir(), 'agents-catalog-external-'));
  t.after(() => rm(externalRoot, { recursive: true, force: true }));
  const externalPath = path.join(externalRoot, 'SKILL.md');
  await writeFile(externalPath, `---
name: review-pr
description: External required input.
---
`);
  const skillDirectory = path.join(root, 'skills/codex/review-pr');
  await mkdir(skillDirectory, { recursive: true });
  const linkPath = path.join(skillDirectory, 'SKILL.md');
  if (!await createTestSymlink(t, externalPath, linkPath)) {
    return;
  }

  await assert.rejects(
    () => loadCatalog(root),
    /skills[/\\]codex[/\\]review-pr[/\\]SKILL\.md: resolved path escapes owning directory/,
  );
});

test('rejects an out-of-tree symlinked model policy directory', async (t) => {
  const root = await createFixture(t);
  const externalRoot = await mkdtemp(path.join(tmpdir(), 'agents-catalog-external-'));
  t.after(() => rm(externalRoot, { recursive: true, force: true }));
  await writeFile(
    path.join(externalRoot, 'model-policy.json'),
    `${JSON.stringify(POLICY)}\n`,
  );
  const platformsPath = path.join(root, 'platforms');
  await rm(platformsPath, { recursive: true });
  if (!await createTestSymlink(t, externalRoot, platformsPath, 'dir')) {
    return;
  }

  await assert.rejects(
    () => loadModelPolicy(root),
    /platforms: resolved path escapes owning directory/,
  );
});

test('reports missing required native agent variants', async (t) => {
  const root = await createFixture(t, {
    'agents/builder/codex.toml': null,
  });

  const errors = messagesFor(await loadCatalog(root));
  assert.ok(errors.includes('builder: missing Codex agent variant'));
});

test('validation CLI prints the exact success output', async (t) => {
  const root = await createCompleteCliFixture(t);

  const { stdout, stderr } = await execFileAsync(
    process.execPath,
    ['scripts/validate-catalog.mjs'],
    { cwd: root },
  );

  assert.equal(stderr, '');
  assert.equal(
    stdout,
    'Catalog valid: 8 agents, 13 skills (5 universal, 8 Claude, 8 Codex)\n',
  );
});

test('README and focused documentation links resolve locally', async () => {
  const files = [
    'README.md',
    'docs/getting-started.md',
    'docs/compatibility.md',
    'docs/platforms/claude.md',
    'docs/platforms/codex.md',
    'docs/authoring.md',
    'docs/testing.md',
  ];
  const localLink = /\[[^\]]+]\(([^)]+)\)/g;

  const readme = await readFile(path.join(REPOSITORY_ROOT, 'README.md'), 'utf8');
  assert.ok(readme.split(/\r?\n/).length - 1 < 180);
  assert.match(readme, /node scripts\/install\.mjs claude/);
  assert.match(readme, /node scripts\/install\.mjs codex/);

  for (const relativeFile of files) {
    const sourcePath = path.join(REPOSITORY_ROOT, relativeFile);
    const contents = await readFile(sourcePath, 'utf8');
    for (const match of contents.matchAll(localLink)) {
      const target = match[1].trim().split('#', 1)[0];
      if (!target || /^[a-z][a-z\d+.-]*:/i.test(target) || target.startsWith('#')) {
        continue;
      }
      await assert.doesNotReject(
        () => stat(path.resolve(path.dirname(sourcePath), target)),
        `${relativeFile} links to missing target ${target}`,
      );
    }
  }
});
