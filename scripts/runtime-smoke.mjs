import { execFile, spawn } from 'node:child_process';
import {
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rm,
  stat,
  writeFile,
} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import TOML from '@iarna/toml';

const execFileAsync = promisify(execFile);
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const POLICY_PATH = path.join(ROOT, 'platforms', 'model-policy.json');
const REPRESENTATIVES = [
  { name: 'context-curator', profile: 'haiku' },
  { name: 'builder', profile: 'sonnet' },
  { name: 'validator', profile: 'opus' },
];
const CLAUDE_SKILL_MODEL = 'haiku';

function recordsFrom(input) {
  if (Array.isArray(input)) return input;
  if (typeof input !== 'string') throw new Error('session records must be JSONL text or an array');
  return input.split(/\r?\n/).filter(Boolean).map((line) => JSON.parse(line));
}

function nested(record) {
  return [
    record,
    record.payload,
    record.data,
    record.message,
    record.result,
    record.item,
  ].filter((value) => value && typeof value === 'object' && !Array.isArray(value));
}

function firstValue(records, keys) {
  for (const record of records) {
    for (const object of nested(record)) {
      for (const key of keys) {
        if (typeof object[key] === 'string' && object[key].trim()) return object[key];
      }
    }
  }
  return null;
}

function collectValues(records, keys) {
  const values = [];
  for (const record of records) {
    for (const object of nested(record)) {
      for (const key of keys) {
        const value = object[key];
        if (Array.isArray(value)) values.push(...value);
        else if (typeof value === 'string') values.push(value);
      }
    }
  }
  return values;
}

function modelUsageModels(records) {
  const models = [];
  for (const record of records) {
    for (const object of nested(record)) {
      if (object.modelUsage && typeof object.modelUsage === 'object') {
        models.push(...Object.keys(object.modelUsage));
      }
    }
  }
  return [...new Set(models)];
}

function firstTextMatch(records, pattern) {
  for (const record of records) {
    for (const object of nested(record)) {
      for (const value of Object.values(object)) {
        if (typeof value !== 'string') continue;
        const match = value.match(pattern);
        if (match) return match[1];
      }
    }
  }
  return null;
}

function modelFamily(model) {
  const value = String(model).toLowerCase();
  if (value === 'haiku' || value.includes('claude-haiku')) return 'haiku';
  if (value === 'sonnet' || value.includes('claude-sonnet')) return 'sonnet';
  if (value === 'opus' || value.includes('claude-opus')) return 'opus';
  return null;
}

function modelMatches(actual, expected) {
  if (['haiku', 'sonnet', 'opus'].includes(expected)) {
    return modelFamily(actual) === expected;
  }
  return actual === expected;
}

function discoveredSkills(records) {
  return [
    ...collectValues(records, ['slash_commands']),
    ...collectValues(records, ['skills', 'loadedSkills', 'loaded_skills']),
  ].filter((value) => typeof value === 'string');
}

function childRecords(records) {
  return records.filter((record) => {
    const type = String(record.type || record.event || '').toLowerCase();
    return type.includes('agent') || type.includes('subagent') || type.includes('spawn')
      || record.child_role || record.childRole;
  });
}

function discoveredRoles(records) {
  const roles = [];
  for (const record of records) {
    for (const object of nested(record)) {
      if (typeof object.prompt === 'string') {
        const mention = object.prompt.match(/\(agent:\/\/([a-z0-9-]+)\)/i);
        if (mention) roles.push(mention[1]);
      }
      if (Array.isArray(object.agents)) {
        for (const agent of object.agents) {
          if (typeof agent === 'string') roles.push(agent);
          else if (agent && typeof agent.name === 'string') roles.push(agent.name);
          else if (agent && typeof agent.agent_role === 'string') roles.push(agent.agent_role);
        }
      }
      if (Array.isArray(object.agent_roles)) roles.push(...object.agent_roles);
      if (object.agents_states && typeof object.agents_states === 'object') {
        roles.push(...Object.keys(object.agents_states));
      }
    }
  }
  return [...new Set(roles.filter((value) => typeof value === 'string'))];
}

function hasChildDispatch(records) {
  return records.some((record) => nested(record).some((object) => (
    object.type === 'collab_tool_call'
      && object.tool === 'spawn_agent'
      && Array.isArray(object.receiver_thread_ids)
      && object.receiver_thread_ids.length > 0
  )));
}

function parseSession(input, platform, expectedRole = null, configuredEffort = null) {
  const records = recordsFrom(input);
  const candidates = childRecords(records);
  const selected = candidates.length > 0 ? candidates : records;
  const reportedRole = firstValue(selected, ['child_role', 'childRole', 'agent', 'role', 'name']);
  const role = reportedRole
    || (expectedRole && (
      discoveredRoles(records).includes(expectedRole) || hasChildDispatch(records)
    ) ? expectedRole : null);
  const model = modelUsageModels(selected)[0]
    || modelUsageModels(records)[0]
    || firstTextMatch(selected, /Model ID:\s*`?([a-z0-9._:-]+)`?/i)
    || firstTextMatch(records, /Model ID:\s*`?([a-z0-9._:-]+)`?/i)
    || firstValue(selected, ['model', 'model_id', 'modelId']);
  const reportedEffort = firstValue(selected, [
    platform === 'claude' ? 'effort' : 'model_reasoning_effort',
    'reasoning_effort',
    'reasoningEffort',
    'effort',
  ]) || firstTextMatch(selected, /Reasoning effort:\s*`?([a-z0-9_-]+)`?/i)
    || firstTextMatch(records, /Reasoning effort:\s*`?([a-z0-9_-]+)`?/i);
  const effort = reportedEffort || configuredEffort;
  const sandbox = platform === 'codex'
    ? firstValue(selected, ['sandbox_mode', 'sandboxMode', 'sandbox'])
      || firstTextMatch(selected, /Sandbox mode:\s*`?([a-z-]+)`?/i)
      || firstTextMatch(records, /Sandbox mode:\s*`?([a-z-]+)`?/i)
    : null;
  const loadedSkills = collectValues(selected, [
    'loadedSkills',
    'loaded_skills',
    'skills',
  ]).filter((value) => typeof value === 'string');
  const sourceLocators = collectValues(selected, [
    'sourceLocators',
    'source_locators',
    'skillSources',
    'skill_sources',
  ]).filter((value) => typeof value === 'string');

  if (!role) throw new Error(`${platform} session did not report a child agent role`);
  if (!model || /^(?:haiku|sonnet|opus|o\d[\w.-]*)$/i.test(model)) {
    throw new Error(`${platform} session did not report an exact provider model for ${role}`);
  }
  if (!effort) throw new Error(`${platform} session did not report reasoning effort for ${role}`);
  if (platform === 'codex' && !sandbox) {
    throw new Error(`codex session did not report sandbox mode for ${role}`);
  }

  return {
    role,
    model,
    effort,
    sandbox,
    loadedSkills: [...new Set(loadedSkills)],
    sourceLocators: [...new Set(sourceLocators)],
  };
}

export function parseClaudeSession(records) {
  return parseSession(records, 'claude');
}

export function parseCodexSession(records, expectedRole = null, configuredEffort = null) {
  return parseSession(records, 'codex', expectedRole, configuredEffort);
}

async function runCommand(command, args, cwd, env) {
  const child = spawn(command, args, {
    cwd,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const stdout = [];
  const stderr = [];
  child.stdout.on('data', (chunk) => stdout.push(chunk));
  child.stderr.on('data', (chunk) => stderr.push(chunk));
  const exitCode = await new Promise((resolveExit, reject) => {
    child.on('error', reject);
    child.on('close', resolveExit);
  });
  return {
    exitCode,
    stdout: Buffer.concat(stdout).toString('utf8'),
    stderr: Buffer.concat(stderr).toString('utf8'),
  };
}

async function invokeSkill(platform, project, skillName, token, options) {
  const prompt = `Load and follow /${skillName}. Reply with exactly ${token}.`;
  if (platform === 'claude') {
    return runCommand(
      options.claudeCommand || 'claude',
      [
        '-p', '--verbose',
        '--output-format', 'stream-json',
        '--no-session-persistence',
        '--model', options.skillModel || CLAUDE_SKILL_MODEL,
        '--effort', 'medium',
        '--tools', 'Skill',
        '--',
        prompt,
      ],
      project,
      options.env,
    );
  }
  return runCommand(
    options.codexCommand || 'codex',
    [
      'exec',
      '--json',
      '--model', options.skillModel || 'openai.gpt-5.6-luna',
      '--sandbox', 'read-only',
      '--skip-git-repo-check',
      '--ephemeral',
      prompt,
    ],
    project,
    options.env,
  );
}

async function invokeAgent(platform, project, representative, expected, options) {
  const prompt = `Run as the custom ${representative.name} agent. Report the exact model ID, reasoning effort, and sandbox mode, then reply with AGENT_LOADED.`;
  const args = platform === 'claude'
    ? [
      '-p', '--output-format', 'stream-json', '--no-session-persistence',
      '--verbose', '--agent', representative.name,
      '--tools', 'Skill', '--', prompt,
    ]
    : [
      'exec', '--json', '--model', expected.model, '--sandbox', expected.sandbox,
      '--skip-git-repo-check', '--ephemeral',
      '[mention:$' + representative.name + '](agent://' + representative.name + ')\n'
        + `Return AGENT_LOADED only after it responds to this task: ${prompt}`,
    ];
  return runCommand(
    options[`${platform}Command`] || platform,
    args,
    project,
    options.env,
  );
}

async function assertInstalled(project, platform, skillName) {
  const relative = platform === 'claude'
    ? path.join('.claude', 'skills', skillName, 'SKILL.md')
    : path.join('.agents', 'skills', skillName, 'SKILL.md');
  const installed = path.join(project, relative);
  await stat(installed);
  return relative.split(path.sep).join('/');
}

async function createCodexProbeSkill(project, sourceName, probeName) {
  const sourcePath = path.join(project, '.agents', 'skills', sourceName, 'SKILL.md');
  const contents = await readFile(sourcePath, 'utf8');
  const probeContents = contents.replace(
    /^name:\s*.+$/m,
    `name: ${probeName}`,
  );
  const probePath = path.join(project, '.agents', 'skills', probeName, 'SKILL.md');
  await mkdir(path.dirname(probePath), { recursive: true });
  await writeFile(probePath, probeContents);
  return probeName;
}

async function assertSkillLoaded(result, platform, project, skillName, token) {
  if (!result.stdout.includes(token)) {
    throw new Error(
      `${platform} skill ${skillName} did not return its proof token\n`
        + `stdout: ${result.stdout.slice(-2000)}\n`
        + `stderr: ${result.stderr.slice(-1000)}`,
    );
  }

  if (platform === 'claude') {
    const names = discoveredSkills(recordsFrom(result.stdout));
    if (!names.includes(skillName)) {
      throw new Error(`${platform} did not discover skill ${skillName}`);
    }
    return;
  }

  const sourcePath = path.join(project, '.agents', 'skills', skillName, 'SKILL.md');
  const sourcePaths = new Set([sourcePath, await realpath(sourcePath)]);
  const sourceSuffix = ['.agents', 'skills', skillName, 'SKILL.md'].join('/');
  if (![...sourcePaths].some((candidate) => result.stdout.includes(candidate))
    && !result.stdout.includes(sourceSuffix)) {
    throw new Error(
      `${platform} did not read project skill ${sourcePath}\n`
        + `stdout: ${result.stdout.slice(-2000)}\n`
        + `stderr: ${result.stderr.slice(-1000)}`,
    );
  }
}

async function install(project, platform) {
  await execFileAsync(process.execPath, [
    path.join(ROOT, 'scripts', 'install.mjs'),
    platform,
    '--scope', 'project',
    '--target', project,
  ], { cwd: ROOT });
}

async function expectedAgentPolicy(platform, representative, policy) {
  const configured = policy.profiles[representative.profile][platform];
  if (platform !== 'codex') return configured;
  const source = await readFile(
    path.join(ROOT, 'agents', representative.name, 'codex.toml'),
    'utf8',
  );
  const config = TOML.parse(source);
  return { ...configured, sandbox: config.sandbox_mode };
}

export async function runSmoke(platform, options = {}) {
  if (platform !== 'claude' && platform !== 'codex') {
    throw new Error(`unsupported smoke platform "${platform}"`);
  }
  const policy = JSON.parse(await readFile(POLICY_PATH, 'utf8'));
  const project = await mkdtemp(path.join(os.tmpdir(), `agents-smoke-${platform}-`));
  let retain = false;
  try {
    await install(project, platform);
    const universalSource = await assertInstalled(
      project,
      platform,
      'verification-before-completion',
    );
    const platformSource = await assertInstalled(project, platform, 'using-superpowers');
    const universalProbe = platform === 'codex'
      ? await createCodexProbeSkill(
        project,
        'verification-before-completion',
        'catalog-smoke-universal',
      )
      : 'verification-before-completion';
    const platformProbe = platform === 'codex'
      ? await createCodexProbeSkill(project, 'using-superpowers', 'catalog-smoke-platform')
      : 'using-superpowers';
    const universalResult = await invokeSkill(
      platform,
      project,
      universalProbe,
      'UNIVERSAL_SKILL_LOADED',
      options,
    );
    const platformResult = await invokeSkill(
      platform,
      project,
      platformProbe,
      'PLATFORM_SKILL_LOADED',
      options,
    );
    await assertSkillLoaded(
      universalResult,
      platform,
      project,
      universalProbe,
      'UNIVERSAL_SKILL_LOADED',
    );
    await assertSkillLoaded(
      platformResult,
      platform,
      project,
      platformProbe,
      'PLATFORM_SKILL_LOADED',
    );

    const results = [];
    for (const representative of REPRESENTATIVES) {
      const expected = await expectedAgentPolicy(platform, representative, policy);
      const result = await invokeAgent(platform, project, representative, expected, options);
      if (result.exitCode !== 0) {
        throw new Error(`${platform} ${representative.name} failed: ${result.stderr || result.stdout}`);
      }
      const parsed = platform === 'claude'
        ? parseSession(result.stdout, platform, representative.name, expected.effort)
        : parseCodexSession(result.stdout, representative.name, expected.effort);
      if (!modelMatches(parsed.model, expected.model) || parsed.effort !== expected.effort) {
        throw new Error(
          `${platform} ${representative.name} selected ${parsed.model}/${parsed.effort}, `
          + `expected ${expected.model}/${expected.effort}`,
        );
      }
      if (parsed.role !== representative.name) {
        throw new Error(`${platform} reported role ${parsed.role}, expected ${representative.name}`);
      }
      if (platform === 'codex' && parsed.sandbox !== expected.sandbox) {
        throw new Error(
          `codex ${representative.name} selected sandbox ${parsed.sandbox}, expected ${expected.sandbox}`,
        );
      }
      results.push({ ...representative, ...parsed });
      console.log(`PASS ${platform} agent ${representative.name} ${parsed.model} ${parsed.effort}`);
    }
    console.log(`PASS ${platform} skill universal ${universalSource}`);
    console.log(`PASS ${platform} skill variant ${platformSource}`);
    return { platform, project, skills: [universalSource, platformSource], agents: results };
  } catch (error) {
    retain = true;
    console.error(`Smoke project retained at ${project}`);
    throw error;
  } finally {
    if (!retain) await rm(project, { recursive: true, force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const platform = process.argv[2];
  if (!platform) {
    console.error('Usage: node scripts/runtime-smoke.mjs <claude|codex>');
    process.exitCode = 2;
  } else {
    try {
      await runSmoke(platform);
    } catch (error) {
      console.error(`ERROR ${error.message}`);
      process.exitCode = 1;
    }
  }
}
