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
      claude: { model: 'opus', effort: 'high' },
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
  ['context-curator', 'orchestration', 'haiku', 'read-only'],
  ['developer', 'implementation', 'sonnet', 'workspace-write'],
  ['documenter', 'documentation', 'haiku', 'workspace-write'],
  ['explore', 'research', 'haiku', 'read-only'],
  ['glide-code-reviewer', 'quality-assurance', 'sonnet', 'read-only'],
  ['research-summarizer', 'research', 'sonnet', 'workspace-write'],
  ['research-validator', 'research', 'sonnet', 'read-only'],
  ['researcher', 'research', 'sonnet', 'read-only'],
  ['security-reviewer', 'quality-assurance', 'sonnet', 'read-only'],
  ['superhuman', 'implementation', 'opus', 'workspace-write'],
  ['team-lead', 'orchestration', 'sonnet', 'read-only'],
  ['team-leader', 'orchestration', 'sonnet', 'read-only'],
  ['tester', 'quality-assurance', 'sonnet', 'workspace-write'],
  ['validator', 'quality-assurance', 'opus', 'read-only'],
  ['valkey-glide-implementor', 'valkey-glide', 'sonnet', 'read-only'],
].map(([name, category, profile, sandbox]) => ({
  name,
  category,
  profile,
  sandbox,
}));

const UNIVERSAL_SKILL_NAMES = [
  'code-review-excellence',
  'diagram-valkey-flow',
  'find-skills',
  'finishing-a-development-branch',
  'glide-skill',
  'implement-cookbook',
  'onboard',
  'receiving-code-review',
  'revamp-cookbook',
  'review-addressed-comments',
  'systematic-debugging',
  'test-driven-development',
  'test-valkey',
  'using-git-worktrees',
  'valkey-spike',
  'verification-before-completion',
  'write-pr',
];

const UNIVERSAL_SOURCE_SNAPSHOT = [
  'code-review-excellence/SKILL.md a3b2b01ad6d0b26eb402014f7734fe3fb9d98f7809cb386ccdf6fe2d4cc82604 0644',
  'diagram-valkey-flow/skill.md 45bb7abd604244ce03f6d3293ec0355b7470ddc11a5b066307495abd11630c72 0644',
  'find-skills/SKILL.md 1e85f6f9686e145aca4a124e3b704b9bbea9aa87e08515c1e352eee70f6e6e7a 0644',
  'finishing-a-development-branch/SKILL.md 5c8d4b59aedb14c94e2f5d787a3265e858e8f53d4ceffe7ff1c15878a52b0e91 0644',
  'glide-skill/.gitignore 6cc5e5fa36c62b6e641ec05981938f0523d2084919ed5a75314178bffb918b93 0644',
  'glide-skill/assets/benchmarks/app-benchmark.js 264373f06b2288156f6292ea498e0873a00e799f6870e327393f12d73f9d0512 0644',
  'glide-skill/assets/benchmarks/package-lock.json 1128d6f2c60e5eeb94a5f1816a600b920ed1e664e44be783c31a06475eee3583 0644',
  'glide-skill/assets/benchmarks/package.json f1a46d4008535d0db266c40e1a0ec26f4157b68b7dff7f60f38b0537b01d0059 0644',
  'glide-skill/assets/benchmarks/README.md 191b9f52a17dc19f0831430f3c70f0995f5fc76bb9133107db759f613e916903 0644',
  'glide-skill/assets/benchmarks/sample-results.txt bc0c03577a0b418920da556791406a97bb3844588cda501a1178f4a457038a8c 0644',
  'glide-skill/assets/benchmarks/sample-valkey-baseline.js 4f897ed0356454103c2784c11904361b52febebd305abf80b5162ff7f8a70a90 0644',
  'glide-skill/assets/benchmarks/sample-valkey-skill.js ca75497814cb9b0cd02661e32680661fa84d0ad58c87c524f1640cc07e29e655 0644',
  'glide-skill/assets/benchmarks/valkey-template.js 08fb8e32398235bb8f55ed923b0038df6afc490860d4d0d810a9404265f35130 0644',
  'glide-skill/assets/config-templates.md d60f923d05ceda37a464e8258cbc6f06e972dd435e92895ed6f6fd7f1a061287 0644',
  'glide-skill/assets/csharp-config.cs 40a540a94559fb9ece9b4ec5ff4461409067d32d5ec04923322e076617408957 0644',
  'glide-skill/assets/go-config.go a1c2e15782726a0656f41c8b904bd273141476fe3253f0c8e9dd03eed8ce9413 0644',
  'glide-skill/assets/java-config.java 42349ad927d1234a560232404489caa9041685bdd967a1a46c4cb036c9996e56 0644',
  'glide-skill/assets/nodejs-config.ts c66f4eedecc0485e987c487d2eb59d8c04c903c7097a295d4dfb55ce32f2b81a 0644',
  'glide-skill/assets/php-config.php 69f6be13eec0204109448088fb1425dc865c699887a39160147c3ebd18544855 0644',
  'glide-skill/assets/python-config.py 54b9fedf0ba0a52f4f6f728c11a6b80d7acd20ac08ae72b8eb33d31dc5aa8ab4 0644',
  'glide-skill/evals/evals.json 6532016e909c7c47dd2607fb7e224004257dcfe6ca45e6c2d1bb8bd61c0d1b63 0644',
  'glide-skill/README.md 81353206207b6de27104d42f0af9ff580ee6b4c145af1a95395e32ad8bf0b4f1 0644',
  'glide-skill/references/csharp-anti-patterns.md 1ba976d9a09c2569e77d38eb372bd2cfd10e51286b6d9b4dbe30d9b1c9c960be 0644',
  'glide-skill/references/csharp.md 3c2f27e6aa799525e4b77aeca8f60ec3aec15b7e29618dd3834280e85f7938e8 0644',
  'glide-skill/references/go-anti-patterns.md 25a40095afe0cbdbd1c7695ee2adfb13971a2448c6262728d3731ccb2ffbab7f 0644',
  'glide-skill/references/go.md c281274a34c804a083a456b956362b74fbbaaf796f9cad10e45f083ec7746948 0644',
  'glide-skill/references/java-anti-patterns.md 05af7d04ae78b4e676fbbb26b23179319ad48985502d9a7beb8d97d333dbc4f4 0644',
  'glide-skill/references/java.md dc0d2138c2b7d79266e2ce933087d3238f70cb4569caa4e19b7cf94793b5af5e 0644',
  'glide-skill/references/nodejs-anti-patterns.md 00dd6013f133e5c0c0aaed46760591163cbad8b44d6a147e1daf5120b77cf605 0644',
  'glide-skill/references/nodejs.md 1d8811cd239ff17c94346da5d5a001987dd1803d92be98e8f752211c7aed4a84 0644',
  'glide-skill/references/php-anti-patterns.md 5b156a1721c2f2b21e45a8d96cea3e2ba887f63a1178e94ff9f74801c2fa0898 0644',
  'glide-skill/references/php.md 275cc8cd4187ea08b65dd5f9f0974ac08875d53e5fa768879845324c0c317777 0644',
  'glide-skill/references/python-anti-patterns.md 8888f0a90cd58061003631638cb4d1071024f9689f7fd31d144b990464e390f8 0644',
  'glide-skill/references/python-batch-async.py bc8c922aa23b138583e3b910f2323ef6261b99403c16df74392cd35a7665499a 0644',
  'glide-skill/references/python-batch-error-handling.py 8f4d19744ccbcef45e8c42ae047d262594299ce67b9b5a4d5ddb996be2b0bb5b 0644',
  'glide-skill/references/python-batch-sync.py fa5dbab7b3b894c4dd01f4940d16a63d8955bea3a1a7334ceb36c445a61d6e13 0644',
  'glide-skill/references/python-ft-api.md cb4db001c4eee32edcdaa4d541fee7f25606a6c9787d777dae0680d609449398 0644',
  'glide-skill/references/python.md 5c5e82c638ab3db8ea22e5692661d5fb971a5ffa08284ae404d6bf99231c5930 0644',
  'glide-skill/references/server-configuration-guide.md 338b13e230bc02f0e693ba6d76ab5626e07fbb4acb12be58e071340ef38c9d38 0644',
  'glide-skill/SKILL.md de903be05d0d70ec346dc69eb29ed51bce79d0fdc46444f8137e642f130a56e6 0644',
  'implement-cookbook/SKILL.md 61fce2eba716fd8f94880a5550853996424ab5305395c66fc92031f29c5e0513 0644',
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
  'receiving-code-review/SKILL.md c9382e92b8f32363566068ecfed19d3b2651eaf40d3942b24840f839dedfc406 0644',
  'revamp-cookbook/SKILL.md cb8ec573b1817599eea32d9242a6deaf3bed736c08b50b30a8cb76119bb83f77 0644',
  'review-addressed-comments/SKILL.md 0322a2d8b8593cf09b15bbda4dceb823f9eadec28a1a94bae3df08f8e1db235f 0644',
  'systematic-debugging/condition-based-waiting-example.ts 40ae5ebe497fdf310200e43fe986552546d0a22837c0d39e855db1cfd33eb88e 0644',
  'systematic-debugging/condition-based-waiting.md e89fec8400d6cd50f43407cec9fab50976ba4d55d0ec2eb51c0bd68036b54c26 0644',
  'systematic-debugging/CREATION-LOG.md c24733a5b1821bd6bed1fc950261f0b9f4e90097e0bbb96459d8179713730789 0644',
  'systematic-debugging/defense-in-depth.md 1e175fb86fc357e58c6aebf5441e481e1b7868b4380c0456b63a17eefbd18ba7 0644',
  'systematic-debugging/find-polluter.sh 6462747eae9b175ac145b78bcfaeab755654a75e32637f08eb633f065a9e1d7c 0755',
  'systematic-debugging/root-cause-tracing.md 6b0622269e098ca1399e123e553fd385f0b6412d88ef0e9c4f5a8ea9cf1cec7b 0644',
  'systematic-debugging/SKILL.md 4999cb851360485eca5074e727bbdd62ef20549c5d5b01216fcbf5831badb473 0644',
  'systematic-debugging/test-academic.md fe2ba480d78ac0d686dc025f41c2a32a43d642bf533f91b0c6053a04d35d6486 0644',
  'systematic-debugging/test-pressure-1.md 0b6a915db0054577819834c79be9eb614e97bddba10d73768e1fbe91cfed048a 0644',
  'systematic-debugging/test-pressure-2.md b2030aeffba07050e8ad573ddf87486457c4a016a786bb326235bebd856f2016 0644',
  'systematic-debugging/test-pressure-3.md 96b50a52e2c7989c9cf20fb752c47c1e9a3a70dc362f8f7989f8f5b64dac7708 0644',
  'test-driven-development/SKILL.md 7dee67b4af6bdccc7a914ca34533184d64592d0f5b23aeae631538168db14994 0644',
  'test-driven-development/testing-anti-patterns.md bde453bc258f06543987477c837939afaa774ea2acbd9f308d702fc452bc4283 0644',
  'test-valkey/SKILL.md 19afde7488d288f0fdcc0677b77522ef7d8bdfdb8313d6ccd735759bf5bb9596 0644',
  'using-git-worktrees/SKILL.md 085a45ee3de432bdb2768011591d9a882cb6c759e2317f379226451c5618fe8e 0644',
  'valkey-spike/SKILL.md 81427d95b8bee0b3dbcca1209819ab348657eeb078e250aef4f206199fdad17b 0644',
  'verification-before-completion/SKILL.md ea52d15aabaf72bc6b558efe2c126f161b53961090ddcd712000273bfe8c7b6c 0644',
  'write-pr/SKILL.md 3a67d5c5eabf34f9b6d6815f0710f1933db927e13c3face9c25c0baf93e22d99 0644',
].map((snapshot) => {
  const [relativePath, hash, mode] = snapshot.split(' ');
  return {
    relativePath,
    hash,
    mode: Number.parseInt(mode, 8),
  };
});

const INTENTIONAL_UNIVERSAL_HASHES = new Map([
  ['glide-skill/SKILL.md', 'b61aeea1c41a32c06ad9a1d36522dc4232beb9170db19c5a5113e15a50616073'],
]);
const CONCRETE_PROVIDER_ID =
  /\b(?:claude-(?:haiku|sonnet|opus)-[\w.-]+|openai\.gpt-[\w.-]+)\b/i;

const KIRO_PROMPT_SHA256 = {
  builder: 'b80abe7dc201152ddb9d3cd83e1d36b2f82b700f432feedb6f77281d62dabce8',
  'code-reviewer': 'c08928400e09e2736360d80d3a8a5047994e39c4efcf17c07fc0c6d3cd8661da',
  'context-curator': 'efe873b6e0beda64a391b0892a2944ad15103245fe76c47974d803e05a9db6ef',
  developer: '8cff13d09ff8c5120eded3a10a24706a5628aba84cc58836071758f523868094',
  documenter: '150337fe06944373304d4ebd1760ca50150fd8020ca1325d6ea0a9c0ddef6509',
  explore: '2f2a85ea295368ea8a84055ee0c0857f98ff300078826f6774fafac76d2d4cd6',
  'glide-code-reviewer': '0514631d6f5b9ceee5f1648ba09e90b03d9484d86a7c4a24a3f773cea9fa200c',
  'research-summarizer': '13bd7596905515e214067479362d8d024a1fae44862bfc0e1e15dadcdd90d18d',
  'research-validator': 'c0b17309751b84a549e5587cd5cfdbc2be897ac74d8259948ad12ecd36024dfe',
  researcher: '0adcd287058b3e998032463dd41c43aaadfd5eddef413030cd5f5ef636a1a263',
  'security-reviewer': '113724268cb337d933fad678fd57db524bfa2061af4b79e65671857d3f8f0acf',
  superhuman: '98d9cfb66f542a077bca0fa6ea3c08c4745f083612a32a4412ff4f4bbab48fc1',
  'team-lead': '3c37a833671af5ef46226e354da3f107d1de9ab510d9b92e1313d46339844ed2',
  'team-leader': '9b54446e49c78c19cbef17e9a8172823597a3e3920f59a057dc0d2ac00d97a3f',
  tester: 'f44834eca27637865dd30403f455be7107dba89ae676528e089df392c1ada63b',
  validator: '7df5fae679dbb5cf14b0f21d8344266a6d596b061b8c2ec78db18337fb01713a',
  'valkey-glide-implementor': 'e9704b6915d6a89672d25cef6f457286fa0c9c46e3a3f4f67407bde159cc64f6',
};

const KIRO_JSON_SHA256 = {
  builder: 'f455232eb0cbdbe8c00d93fe6e8f9970fddd1cd3651abe00a8784df887fda078',
  'code-reviewer': 'da5a22b3cec3c0cd30fb63506b2f9530c2050d23ab749ecbc000b172cfc1feae',
  'context-curator': '566ea92127d50a46c542d4a64b1c98681054828a2737f717c5b98fdd36070924',
  developer: '70c94d1663f71e9f6876a1c440290f025be2bfefda2f609b565cd62379252218',
  documenter: '4d4bbefb929b7ea991675afc999fd8fc8010310ca4a9438c5f547c9dd4e4a76e',
  explore: '423830a5ad8a5ac9e0a99a2d1c1f7d0be8f27e55a2c5678e51dc97babe22febc',
  'glide-code-reviewer': '6e7f8af47e9913af1a25986c1dc5408e27eae745513705ce0536882e7e8a29cd',
  'research-summarizer': 'c9607101d364ce350132b5fc99fa4e2e3b32141030961a5e370a825e91f967af',
  'research-validator': '36bddb25df064a14c45daef63f658e1823c04aafb82537abfa90cb469c93d30c',
  researcher: 'd234a3721fe7a6b666baf92fdcfb3b5512f3225a1d4076f21c1338882fe3a2e9',
  'security-reviewer': 'ffb711658966914f5148fa532caaba0cc4aa37a9f247c3b8174a22f6fcef13de',
  superhuman: 'f8902c05f25975604a761c8c73dacadf9f9c670909393b63bccdb78a25ecdec5',
  'team-lead': '0d888a58addbb0b1ee8c36cd34e6c43b2c52484e2b1b9934bbb7df7a9bc3ac1b',
  'team-leader': 'ee281cfa8cddd92bd7b4d1e1b0ee1b6d4fde023f32ae266543e723f79c62bc80',
  tester: '2dadf5d1b11dd5d24360f5d3a82cc22cb56574a8ed17cceebd62fe0764557cc8',
  validator: 'a34ec784b7e897149778e023ad0bcfe1c8a631acda9b8d6c428282723a249c5a',
  'valkey-glide-implementor': '251941fdc61c8656f4225e24ce48e30dece44ae6599cb78cf80e8b3436c3f22f',
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

  for (let index = 1; index <= 16; index += 1) {
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

  for (let index = 1; index <= 16; index += 1) {
    const name = `universal-${String(index).padStart(2, '0')}`;
    await writeFixtureFile(root, `skills/universal/${name}/SKILL.md`, `---
name: ${name}
description: Universal fixture skill ${index}.
---
`);
  }

  for (let index = 1; index <= 22; index += 1) {
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

  assert.equal(catalog.agents.length, 17);
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
    if (expected.name === 'developer') {
      assert.equal(
        Object.hasOwn(kiro, 'prompt'),
        false,
        'developer: Kiro prompt loading behavior must remain unchanged',
      );
    } else {
      assert.equal(kiro.prompt, 'file://./kiro-prompt.md');
      kiro.prompt = `file://./${expected.name}-prompt.md`;
    }
    assert.equal(
      sha256(JSON.stringify(canonicalizeJson(kiro))),
      KIRO_JSON_SHA256[expected.name],
      `${expected.name}: retained Kiro JSON behavior changed`,
    );
  }

  const teamLead = catalog.agents.find(
    ({ manifest }) => manifest.name === 'team-lead',
  );
  for (const [platform, instructions] of [
    ['Claude', teamLead.claude.instructions],
    ['Codex', teamLead.codex.developer_instructions],
  ]) {
    const normalizedInstructions = instructions.replace(/\s+/g, ' ');
    assert.match(
      normalizedInstructions,
      /Before any implementation, establish an isolated worktree using available native worktree support\./,
      `team-lead: ${platform} must require workspace isolation`,
    );
    assert.match(
      normalizedInstructions,
      /If isolation cannot be established safely, stop and report the blocker\./,
      `team-lead: ${platform} must stop when isolation fails`,
    );
    assert.match(
      normalizedInstructions,
      /A task has a maximum of three total implementation attempts\./,
      `team-lead: ${platform} must cap implementation attempts`,
    );
    assert.match(
      normalizedInstructions,
      /If an agent reports `UNCERTAIN` or the evidence remains ambiguous, provide clarifying context or stop and ask the user\./,
      `team-lead: ${platform} must escalate uncertainty`,
    );
  }

  const contextCurator = catalog.agents.find(
    ({ manifest }) => manifest.name === 'context-curator',
  );
  assert.match(
    contextCurator.codex.developer_instructions.replace(/\s+/g, ' '),
    /Use the configured Obsidian search capability with query and folder constraints/,
  );
  assert.doesNotMatch(
    contextCurator.codex.developer_instructions,
    /\bsearch_notes\b|searchContent\s*=|limit\s*=/,
    'context-curator: Codex instructions contain Kiro tool-call syntax',
  );

  const superhuman = catalog.agents.find(
    ({ manifest }) => manifest.name === 'superhuman',
  );
  assert.deepEqual(
    superhuman.claude.tools.filter((tool) => tool.startsWith('mcp__firecrawl__')),
    [
      'mcp__firecrawl__firecrawl_search',
      'mcp__firecrawl__firecrawl_scrape',
    ],
    'superhuman: Claude must allow exactly the Firecrawl tools its instructions require',
  );

  const flatAgentFiles = (await readdir(path.join(REPOSITORY_ROOT, 'agents'), {
    withFileTypes: true,
  }))
    .filter((entry) => entry.isFile())
    .map(({ name }) => name)
    .filter((name) => name.endsWith('.json') || name.endsWith('-prompt.md'));
  assert.deepEqual(flatAgentFiles, []);
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
    82,
  );

  const expectedFiles = UNIVERSAL_SOURCE_SNAPSHOT.map((source) => ({
    ...source,
    relativePath: source.relativePath === 'diagram-valkey-flow/skill.md'
      ? 'diagram-valkey-flow/SKILL.md'
      : source.relativePath,
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

  assert.equal(
    actualFiles.find(
      ({ relativePath }) => relativePath === 'revamp-cookbook/SKILL.md',
    ).hash,
    'cb8ec573b1817599eea32d9242a6deaf3bed736c08b50b30a8cb76119bb83f77',
  );
  assert.equal(
    actualFiles.find(
      ({ relativePath }) => relativePath === 'systematic-debugging/find-polluter.sh',
    ).mode,
    0o755,
  );

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
  assert.equal(claudeFiles.length, 82);
  assert.deepEqual(claudeFiles, expectedInstallFiles);
  assert.deepEqual(claudeFiles, codexFiles);

  for (const name of UNIVERSAL_SKILL_NAMES) {
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
    'Catalog valid: 17 agents, 40 skills (17 universal, 23 Claude, 23 Codex)\n',
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
