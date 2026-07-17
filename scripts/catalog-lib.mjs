import { createHash } from 'node:crypto';
import {
  lstat,
  readdir,
  readFile,
  realpath,
  stat,
} from 'node:fs/promises';
import path from 'node:path';

import TOML from '@iarna/toml';
import { parse as parseYaml } from 'yaml';

const EFFORT_ORDER = new Map([
  ['low', 0],
  ['medium', 1],
  ['high', 2],
  ['xhigh', 3],
  ['max', 4],
]);

const MODEL_ID = /\b(?:(?:global\.anthropic\.)?claude-(?:haiku|sonnet|opus)-[\w.-]+|openai\.gpt-[\w.-]+)\b/i;
const SKILL_NAME = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function compareStrings(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function toPosix(value) {
  return value.split(path.sep).join('/');
}

function isPathWithin(targetPath, ownerPath) {
  const relative = path.relative(ownerPath, targetPath);
  return relative === ''
    || (relative !== '..'
      && !relative.startsWith(`..${path.sep}`)
      && !path.isAbsolute(relative));
}

async function resolveContainedPath(filePath, ownerDirectory) {
  const [resolvedPath, resolvedOwner] = await Promise.all([
    realpath(filePath),
    realpath(ownerDirectory),
  ]);
  if (!isPathWithin(resolvedPath, resolvedOwner)) {
    throw new Error(`${filePath}: resolved path escapes owning directory`);
  }
  return resolvedPath;
}

async function readContainedFile(filePath, ownerDirectory, encoding) {
  const resolvedPath = await resolveContainedPath(filePath, ownerDirectory);
  return readFile(resolvedPath, encoding);
}

async function readOptional(filePath, ownerDirectory, encoding) {
  try {
    await lstat(filePath);
  } catch (error) {
    if (error.code === 'ENOENT') {
      return null;
    }
    throw error;
  }
  return readContainedFile(filePath, ownerDirectory, encoding);
}

async function directDirectories(directory, containmentRoot) {
  try {
    await lstat(directory);
    await resolveContainedPath(directory, containmentRoot);
    const entries = await readdir(directory, { withFileTypes: true });
    const names = [];
    for (const entry of entries.sort((left, right) => compareStrings(left.name, right.name))) {
      if (entry.isDirectory()) {
        names.push(entry.name);
      } else if (entry.isSymbolicLink()) {
        const candidate = path.join(directory, entry.name);
        const resolvedPath = await resolveContainedPath(candidate, directory);
        if ((await stat(resolvedPath)).isDirectory()) {
          names.push(entry.name);
        }
      }
    }
    return names;
  } catch (error) {
    if (error.code === 'ENOENT') {
      return [];
    }
    throw error;
  }
}

async function filesUnder(
  directory,
  root = directory,
  containmentRoot = path.dirname(root),
  ancestors = new Set(),
) {
  let entries;
  try {
    await lstat(directory);
    await resolveContainedPath(root, containmentRoot);
    const resolvedDirectory = await resolveContainedPath(directory, root);
    if (ancestors.has(resolvedDirectory)) {
      throw new Error(`${directory}: symlink cycle in auxiliary files`);
    }
    ancestors = new Set(ancestors).add(resolvedDirectory);
    entries = await readdir(directory, { withFileTypes: true });
  } catch (error) {
    if (error.code === 'ENOENT') {
      return [];
    }
    throw error;
  }

  const files = [];
  for (const entry of entries.sort((left, right) => compareStrings(left.name, right.name))) {
    const filePath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await filesUnder(filePath, root, containmentRoot, ancestors));
    } else if (entry.isFile()) {
      files.push({
        name: toPosix(path.relative(root, filePath)),
        path: filePath,
        contents: await readContainedFile(filePath, root),
        mode: (await stat(filePath)).mode & 0o777,
      });
    } else if (entry.isSymbolicLink()) {
      const resolvedPath = await resolveContainedPath(filePath, root);
      const target = await stat(resolvedPath);
      if (target.isDirectory()) {
        files.push(...await filesUnder(filePath, root, containmentRoot, ancestors));
      } else if (target.isFile()) {
        files.push({
          name: toPosix(path.relative(root, filePath)),
          path: filePath,
          contents: await readFile(resolvedPath),
          mode: target.mode & 0o777,
        });
      } else {
        throw new Error(`${filePath}: unsupported symlink target`);
      }
    }
  }
  return files;
}

function parseFrontmatter(contents, sourcePath) {
  const lines = contents.replace(/^\uFEFF/, '').split(/\r?\n/);
  if (lines[0] !== '---') {
    throw new Error(`${sourcePath}: missing YAML frontmatter`);
  }

  const closingIndex = lines.indexOf('---', 1);
  if (closingIndex === -1) {
    throw new Error(`${sourcePath}: unterminated YAML frontmatter`);
  }

  let frontmatter;
  try {
    frontmatter = parseYaml(lines.slice(1, closingIndex).join('\n'));
  } catch (error) {
    throw new Error(`${sourcePath}: invalid YAML frontmatter: ${error.message}`, { cause: error });
  }

  if (frontmatter === null) {
    frontmatter = {};
  }
  if (typeof frontmatter !== 'object' || Array.isArray(frontmatter)) {
    throw new Error(`${sourcePath}: YAML frontmatter must be a mapping`);
  }

  return {
    frontmatter,
    body: lines.slice(closingIndex + 1).join('\n').replace(/^\n/, ''),
  };
}

export function parseClaudeAgent(contents, sourcePath) {
  const { frontmatter, body } = parseFrontmatter(contents, sourcePath);
  return {
    ...frontmatter,
    instructions: body,
    contents,
    sourcePath,
  };
}

export function parseCodexAgent(contents, sourcePath) {
  try {
    return {
      ...TOML.parse(contents),
      contents,
      sourcePath,
    };
  } catch (error) {
    throw new Error(`${sourcePath}: invalid TOML: ${error.message}`, { cause: error });
  }
}

function parseSkill(contents, sourcePath) {
  const { frontmatter, body } = parseFrontmatter(contents, sourcePath);
  return { metadata: frontmatter, instructions: body };
}

export async function loadModelPolicy(root) {
  const platformsRoot = path.join(root, 'platforms');
  const policyPath = path.join(platformsRoot, 'model-policy.json');
  try {
    await resolveContainedPath(platformsRoot, root);
    return JSON.parse(await readContainedFile(
      policyPath,
      platformsRoot,
      'utf8',
    ));
  } catch (error) {
    throw new Error(`${policyPath}: invalid model policy: ${error.message}`, { cause: error });
  }
}

async function loadAgents(root) {
  const agentsRoot = path.join(root, 'agents');
  const agents = [];

  for (const directoryName of await directDirectories(agentsRoot, root)) {
    const directory = path.join(agentsRoot, directoryName);
    const manifestPath = path.join(directory, 'manifest.json');
    const manifestContents = await readOptional(manifestPath, directory, 'utf8');
    if (manifestContents === null) {
      continue;
    }

    let manifest;
    try {
      manifest = JSON.parse(manifestContents);
    } catch (error) {
      throw new Error(`${manifestPath}: invalid agent manifest: ${error.message}`, { cause: error });
    }

    const claudePath = path.join(directory, 'claude.md');
    const codexPath = path.join(directory, 'codex.toml');
    const claudeContents = await readOptional(claudePath, directory, 'utf8');
    const codexContents = await readOptional(codexPath, directory, 'utf8');

    agents.push({
      manifest,
      claude: claudeContents === null
        ? null
        : parseClaudeAgent(claudeContents, claudePath),
      codex: codexContents === null
        ? null
        : parseCodexAgent(codexContents, codexPath),
      paths: {
        directory,
        manifest: manifestPath,
        claude: claudePath,
        codex: codexPath,
      },
      directoryName,
    });
  }

  return agents;
}

async function loadSkillVariants(root, platform) {
  const skillsRoot = path.join(root, 'skills');
  const platformRoot = path.join(root, 'skills', platform);
  const variants = [];

  for (const name of await directDirectories(platformRoot, skillsRoot)) {
    const directory = path.join(platformRoot, name);
    const skillPath = path.join(directory, 'SKILL.md');
    const contents = await readOptional(skillPath, directory, 'utf8');
    if (contents === null) {
      continue;
    }

    const parsed = parseSkill(contents, skillPath);
    variants.push({
      name,
      platform,
      ...parsed,
      path: skillPath,
      directory,
      relativePath: `skills/${platform}/${name}/SKILL.md`,
      contents,
      files: await filesUnder(directory, directory, platformRoot),
    });
  }

  return variants;
}

export async function loadCatalog(root) {
  const sharedRoot = path.join(root, 'skills', '_shared');
  const [agents, universal, claude, codex, sharedFiles] = await Promise.all([
    loadAgents(root),
    loadSkillVariants(root, 'universal'),
    loadSkillVariants(root, 'claude'),
    loadSkillVariants(root, 'codex'),
    filesUnder(sharedRoot, sharedRoot, path.join(root, 'skills')),
  ]);

  const skillsByName = new Map();
  for (const [platform, variants] of Object.entries({ universal, claude, codex })) {
    for (const variant of variants) {
      if (!skillsByName.has(variant.name)) {
        skillsByName.set(variant.name, {
          name: variant.name,
          variants: {},
          paths: {},
        });
      }
      const skill = skillsByName.get(variant.name);
      skill.variants[platform] = variant;
      skill.paths[platform] = variant.path;
    }
  }

  return {
    root,
    agents,
    skills: [...skillsByName.values()].sort((left, right) => compareStrings(left.name, right.name)),
    skillVariants: { universal, claude, codex },
    sharedFiles,
  };
}

function addMetadataErrors(errors, agent, native, platformLabel) {
  const name = agent.manifest?.name ?? agent.directoryName;
  if (native.name !== agent.manifest?.name) {
    errors.push(`${name}: ${platformLabel} name does not match manifest`);
  }
  if (native.description !== agent.manifest?.description) {
    errors.push(`${name}: ${platformLabel} description does not match manifest`);
  }
}

function addPolicyErrors(errors, agent, native, platform, profileName, expected) {
  const name = agent.manifest?.name ?? agent.directoryName;
  const label = platform === 'claude' ? 'Claude' : 'Codex';
  const effortField = platform === 'claude' ? 'effort' : 'model_reasoning_effort';
  const actualEffort = native[effortField];

  if (native.model !== expected.model) {
    errors.push(
      `${name}: ${label} model "${native.model}" does not match profile "${profileName}" model "${expected.model}"`,
    );
  }
  if (actualEffort !== expected.effort) {
    errors.push(
      `${name}: ${label} effort "${actualEffort}" does not match profile "${profileName}" effort "${expected.effort}"`,
    );
  }
}

function addProviderPolicyCompletenessErrors(
  errors,
  profileName,
  provider,
  providerPolicy,
) {
  const label = provider === 'claude' ? 'Claude' : 'Codex';
  if (
    providerPolicy === null
    || typeof providerPolicy !== 'object'
    || Array.isArray(providerPolicy)
  ) {
    errors.push(`model profile "${profileName}": missing ${label} policy`);
    return false;
  }

  let complete = true;
  for (const field of ['model', 'effort']) {
    if (
      typeof providerPolicy[field] !== 'string'
      || providerPolicy[field].trim().length === 0
    ) {
      errors.push(
        `model profile "${profileName}": ${label} policy requires a non-empty ${field}`,
      );
      complete = false;
    }
  }
  return complete;
}

function extractReferences(contents) {
  const references = new Set();
  const markdownLink = /!?\[[^\]]*]\(\s*(?:<([^>]+)>|([^\s)]+))/g;
  const inlineCode = /`([^`\n]+)`/g;
  const plainPath = /(?:^|[\s("'`])((?:\.\.?\/|_shared\/|references\/)[^\s)"'`<>]+)/gm;
  const supportedPath = /^(?:references\/|_shared\/|\.\.\/_shared\/|\.\/|\.\.\/)/;

  for (const match of contents.matchAll(markdownLink)) {
    const value = (match[1] ?? match[2] ?? '').trim();
    if (value) {
      references.add(value.replace(/[.,;:]+$/, ''));
    }
  }
  for (const expression of [inlineCode, plainPath]) {
    for (const match of contents.matchAll(expression)) {
      const value = (match[1] ?? match[2] ?? '').trim();
      if (value && supportedPath.test(value)) {
        references.add(value.replace(/[.,;:]+$/, ''));
      }
    }
  }
  return [...references].sort(compareStrings);
}

function isAbsoluteReference(reference) {
  return path.posix.isAbsolute(reference)
    || path.win32.isAbsolute(reference)
    || /^file:/i.test(reference)
    || /^[a-z]:/i.test(reference);
}

function isExternalReference(reference) {
  return /^[a-z][a-z\d+.-]*:/i.test(reference) && !/^file:/i.test(reference);
}

function cleanReference(reference) {
  return reference.split('#', 1)[0].split('?', 1)[0];
}

function referenceEscapes(reference) {
  const cleaned = cleanReference(reference).replaceAll('\\', '/');
  if (cleaned.startsWith('../_shared/')) {
    const sharedPath = path.posix.normalize(cleaned.slice('../_shared/'.length));
    return sharedPath === '..' || sharedPath.startsWith('../');
  }
  if (cleaned.startsWith('_shared/')) {
    const sharedPath = path.posix.normalize(cleaned.slice('_shared/'.length));
    return sharedPath === '..' || sharedPath.startsWith('../');
  }

  const normalized = path.posix.normalize(cleaned);
  return normalized === '..' || normalized.startsWith('../');
}

function addSkillErrors(errors, variant) {
  const metadata = variant.metadata;
  const source = variant.relativePath;

  if (typeof metadata.name !== 'string' || metadata.name.length === 0) {
    errors.push(`${source}: skill frontmatter requires a non-empty name`);
  } else if (metadata.name !== variant.name) {
    errors.push(
      `${source}: frontmatter name "${metadata.name}" does not match directory "${variant.name}"`,
    );
  }
  if (typeof metadata.description !== 'string' || metadata.description.trim().length === 0) {
    errors.push(`${source}: skill frontmatter requires a non-empty description`);
  }
  if (variant.name.length > 64 || !SKILL_NAME.test(variant.name)) {
    errors.push(`${source}: skill name "${variant.name}" is invalid`);
  }

  for (const reference of extractReferences(variant.contents)) {
    if (isAbsoluteReference(reference)) {
      errors.push(`${source}: unsafe absolute reference "${reference}"`);
    } else if (isExternalReference(reference)) {
      continue;
    } else if (referenceEscapes(reference)) {
      errors.push(
        `${source}: reference "${reference}" escapes the skill or shared directory`,
      );
    }
  }
}

export function validateCatalog(catalog, policy) {
  const errors = [];
  const minimumEffort = policy?.minimumClaudeEffort;
  const minimumOrder = EFFORT_ORDER.get(minimumEffort);
  const completeProfiles = new Map();

  for (const profileName of Object.keys(policy?.profiles ?? {}).sort(compareStrings)) {
    const profile = policy.profiles[profileName];
    completeProfiles.set(profileName, {
      claude: addProviderPolicyCompletenessErrors(
        errors,
        profileName,
        'claude',
        profile?.claude,
      ),
      codex: addProviderPolicyCompletenessErrors(
        errors,
        profileName,
        'codex',
        profile?.codex,
      ),
    });
  }

  for (const agent of catalog.agents) {
    const name = agent.manifest?.name ?? agent.directoryName;
    if (agent.manifest?.name !== agent.directoryName) {
      errors.push(
        `${name}: manifest name "${agent.manifest?.name}" does not match directory "${agent.directoryName}"`,
      );
    }
    if (!agent.claude) {
      errors.push(`${name}: missing Claude agent variant`);
    }
    if (!agent.codex) {
      errors.push(`${name}: missing Codex agent variant`);
    }

    if (agent.claude) {
      addMetadataErrors(errors, agent, agent.claude, 'Claude');
      const actualOrder = EFFORT_ORDER.get(agent.claude.effort);
      if (actualOrder === undefined) {
        errors.push(`${name}: Claude effort "${agent.claude.effort}" is not recognized`);
      } else if (minimumOrder !== undefined && actualOrder < minimumOrder) {
        errors.push(
          `${name}: Claude effort "${agent.claude.effort}" is below required "${minimumEffort}"`,
        );
      }
    }
    if (agent.codex) {
      addMetadataErrors(errors, agent, agent.codex, 'Codex');
      if (!EFFORT_ORDER.has(agent.codex.model_reasoning_effort)) {
        errors.push(
          `${name}: Codex effort "${agent.codex.model_reasoning_effort}" is not recognized`,
        );
      }
    }

    const profileName = agent.manifest?.profile;
    const profile = policy?.profiles?.[profileName];
    if (!profile) {
      errors.push(`${name}: unknown model profile "${profileName}"`);
      continue;
    }
    if (agent.claude && completeProfiles.get(profileName)?.claude) {
      addPolicyErrors(errors, agent, agent.claude, 'claude', profileName, profile.claude);
    }
    if (agent.codex && completeProfiles.get(profileName)?.codex) {
      addPolicyErrors(errors, agent, agent.codex, 'codex', profileName, profile.codex);
    }
  }

  for (const platform of ['universal', 'claude', 'codex']) {
    for (const variant of catalog.skillVariants[platform]) {
      addSkillErrors(errors, variant);
      if (platform === 'universal') {
        const match = variant.contents.match(MODEL_ID);
        if (match) {
          errors.push(
            `universal skill "${variant.name}" contains concrete provider model ID "${match[0]}"`,
          );
        }
      }
    }
  }

  const universalNames = new Set(
    catalog.skillVariants.universal.map((variant) => variant.name),
  );
  for (const platform of ['claude', 'codex']) {
    for (const variant of catalog.skillVariants[platform]) {
      if (universalNames.has(variant.name)) {
        errors.push(`skill "${variant.name}" exists in universal and ${platform}`);
      }
    }
  }

  return [...new Set(errors)].sort(compareStrings);
}

export function buildInstallSet(catalog, platform) {
  if (platform !== 'claude' && platform !== 'codex') {
    throw new Error(`unsupported platform "${platform}"`);
  }

  const platformSkills = new Map();
  for (const variant of [
    ...catalog.skillVariants.universal,
    ...catalog.skillVariants[platform],
  ]) {
    if (platformSkills.has(variant.name)) {
      throw new Error(`skill "${variant.name}" exists in universal and ${platform}`);
    }
    platformSkills.set(variant.name, variant);
  }

  return {
    platform,
    agents: catalog.agents
      .filter((agent) => agent[platform])
      .map((agent) => ({
        name: agent.manifest.name,
        path: agent.paths[platform],
        contents: agent[platform].contents,
        agent: agent[platform],
      }))
      .sort((left, right) => compareStrings(left.name, right.name)),
    skills: [...platformSkills.values()].sort(
      (left, right) => compareStrings(left.name, right.name),
    ),
    sharedFiles: catalog.sharedFiles,
  };
}

export function sha256(contents) {
  return createHash('sha256').update(contents).digest('hex');
}
