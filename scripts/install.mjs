import { randomUUID } from 'node:crypto';
import { execFile } from 'node:child_process';
import * as fs from 'node:fs/promises';
import { homedir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import {
  buildInstallSet,
  loadCatalog,
  loadModelPolicy,
  sha256,
  validateCatalog,
} from './catalog-lib.mjs';

const execFileAsync = promisify(execFile);
const STATE_FILE = '.agents-catalog-install.json';
const STATE_SCHEMA_VERSION = 1;
const REVISION_PATTERN = /^[0-9a-f]{40}(?:-dirty)?$/;
const HASH_PATTERN = /^[0-9a-f]{64}$/;

export const USAGE = 'Usage: node scripts/install.mjs <claude|codex> '
  + '[--scope <user|project>] [--target <path>] [--dry-run] [--force] '
  + '[--migrate-legacy]';

function usageError() {
  return new Error(USAGE);
}

export function parseArgs(args) {
  if (args[0] !== 'claude' && args[0] !== 'codex') {
    throw usageError();
  }
  const options = {
    platform: args[0],
    scope: 'user',
    target: undefined,
    dryRun: false,
    force: false,
    migrateLegacy: false,
  };
  const seen = new Set();

  for (let index = 1; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === 'claude' || argument === 'codex') {
      if (options.platform !== undefined) {
        throw usageError();
      }
      options.platform = argument;
      continue;
    }

    if (argument === '--scope' || argument === '--target') {
      if (seen.has(argument)) {
        throw usageError();
      }
      const value = args[index + 1];
      if (value === undefined || value.length === 0 || value.startsWith('--')) {
        throw usageError();
      }
      if (argument === '--scope' && value !== 'user' && value !== 'project') {
        throw usageError();
      }
      seen.add(argument);
      options[argument === '--scope' ? 'scope' : 'target'] = value;
      index += 1;
      continue;
    }

    if (
      argument === '--dry-run'
      || argument === '--force'
      || argument === '--migrate-legacy'
    ) {
      if (seen.has(argument)) {
        throw usageError();
      }
      if (argument === '--migrate-legacy' && options.platform !== 'claude') {
        throw usageError();
      }
      seen.add(argument);
      if (argument === '--dry-run') options.dryRun = true;
      if (argument === '--force') options.force = true;
      if (argument === '--migrate-legacy') options.migrateLegacy = true;
      continue;
    }

    throw usageError();
  }

  return options;
}

async function gitIsClean(root, args) {
  try {
    await execFileAsync('git', ['-C', root, ...args]);
    return true;
  } catch (error) {
    if (error.code === 1) {
      return false;
    }
    throw error;
  }
}

export async function getCatalogRevision(catalogRoot) {
  const { stdout } = await execFileAsync(
    'git',
    ['-C', catalogRoot, 'rev-parse', '--verify', 'HEAD'],
  );
  const revision = stdout.trim();
  if (!/^[0-9a-f]{40}$/.test(revision)) {
    throw new Error(`unable to determine full catalog revision for ${catalogRoot}`);
  }

  const [worktreeClean, indexClean] = await Promise.all([
    gitIsClean(catalogRoot, ['diff', '--quiet', '--ignore-submodules', '--']),
    gitIsClean(catalogRoot, ['diff', '--cached', '--quiet', '--ignore-submodules', '--']),
  ]);
  return worktreeClean && indexClean ? revision : `${revision}-dirty`;
}

function toPosix(value) {
  return value.split(path.sep).join('/');
}

function compareStrings(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function isPlainObject(value) {
  return value !== null
    && typeof value === 'object'
    && !Array.isArray(value);
}

function isSafeRelativePosixPath(value) {
  if (
    typeof value !== 'string'
    || value.length === 0
    || value.includes('\\')
    || path.posix.isAbsolute(value)
  ) {
    return false;
  }
  const normalized = path.posix.normalize(value);
  return normalized === value
    && normalized !== '.'
    && normalized !== '..'
    && !normalized.startsWith('../');
}

function validateState(state) {
  if (!isPlainObject(state)) {
    throw new Error('invalid install state: root must be an object');
  }
  if (state.schemaVersion !== STATE_SCHEMA_VERSION) {
    throw new Error(`unsupported install state schemaVersion "${state.schemaVersion}"`);
  }
  if (state.platform !== 'claude' && state.platform !== 'codex') {
    throw new Error('invalid install state: platform must be "claude" or "codex"');
  }
  if (typeof state.catalogRevision !== 'string' || !REVISION_PATTERN.test(state.catalogRevision)) {
    throw new Error('invalid install state: catalogRevision must be a full Git revision');
  }
  if (!isPlainObject(state.files)) {
    throw new Error('invalid install state: files must be an object');
  }

  for (const [destination, metadata] of Object.entries(state.files)) {
    if (!isSafeRelativePosixPath(destination)) {
      throw new Error(`invalid install state: unsafe destination "${destination}"`);
    }
    if (
      !isPlainObject(metadata)
      || typeof metadata.sha256 !== 'string'
      || !HASH_PATTERN.test(metadata.sha256)
      || !isSafeRelativePosixPath(metadata.source)
    ) {
      throw new Error(`invalid install state: bad metadata for "${destination}"`);
    }
  }
  return state;
}

function platformRoots(platform) {
  if (platform === 'claude') {
    return {
      agent: '.claude/agents',
      skill: '.claude/skills',
    };
  }
  return {
    agent: '.codex/agents',
    skill: '.agents/skills',
  };
}

async function findLegacyMigrations(root, platform, filePlan, enabled, fsOps) {
  if (!enabled) return [];
  if (platform !== 'claude') {
    throw new Error('--migrate-legacy is only supported for Claude installs');
  }

  let resolvedRoot = root;
  try {
    resolvedRoot = await fsOps.realpath(root);
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }

  const skillNames = new Set();
  for (const entry of filePlan) {
    const prefix = '.claude/skills/';
    if (entry.path.startsWith(prefix)) {
      skillNames.add(entry.path.slice(prefix.length).split('/', 1)[0]);
    }
  }

  const migrations = [];
  for (const name of [...skillNames].sort(compareStrings)) {
    const destinationRelative = `.claude/skills/${name}`;
    const destination = path.join(root, ...destinationRelative.split('/'));
    const status = await optionalLstat(destination, fsOps);
    if (status === null || !status.isSymbolicLink()) continue;

    let resolvedDestination;
    try {
      resolvedDestination = await fsOps.realpath(destination);
    } catch (error) {
      throw new Error(
        `${destinationRelative}: legacy migration target is unavailable: ${error.message}`,
        { cause: error },
      );
    }
    const targetStatus = await fsOps.stat(resolvedDestination);
    if (!targetStatus.isDirectory()) {
      throw new Error(
        `${destinationRelative}: legacy migration target must be a directory`,
      );
    }

    migrations.push({
      path: destinationRelative,
      target: toPosix(path.relative(resolvedRoot, resolvedDestination)) || '.',
    });
  }
  return migrations;
}

function sourceRelativePath(catalogRoot, sourcePath) {
  const relative = toPosix(path.relative(catalogRoot, sourcePath));
  if (!isSafeRelativePosixPath(relative)) {
    throw new Error(`catalog source escapes catalog root: ${sourcePath}`);
  }
  return relative;
}

function addPlanEntry(
  entries,
  catalogRoot,
  destination,
  sourcePath,
  contents,
  mode = 0o644,
) {
  if (!isSafeRelativePosixPath(destination)) {
    throw new Error(`unsafe install destination "${destination}"`);
  }
  if (entries.has(destination)) {
    throw new Error(`duplicate install destination "${destination}"`);
  }
  const bytes = Buffer.isBuffer(contents) ? contents : Buffer.from(contents);
  entries.set(destination, {
    path: destination,
    source: sourceRelativePath(catalogRoot, sourcePath),
    contents: bytes,
    sha256: sha256(bytes),
    mode,
  });
}

function createFilePlan(catalogRoot, installSet) {
  const roots = platformRoots(installSet.platform);
  const entries = new Map();
  const agentExtension = installSet.platform === 'claude' ? 'md' : 'toml';

  for (const agent of installSet.agents) {
    addPlanEntry(
      entries,
      catalogRoot,
      `${roots.agent}/${agent.name}.${agentExtension}`,
      agent.path,
      agent.contents,
    );
  }
  for (const skill of installSet.skills) {
    for (const file of skill.files) {
      addPlanEntry(
        entries,
        catalogRoot,
        `${roots.skill}/${skill.name}/${file.name}`,
        file.path,
        file.contents,
        file.mode,
      );
    }
  }
  for (const file of installSet.sharedFiles) {
    addPlanEntry(
      entries,
      catalogRoot,
      `${roots.skill}/_shared/${file.name}`,
      file.path,
      file.contents,
      file.mode,
    );
  }

  return [...entries.values()].sort((left, right) => compareStrings(left.path, right.path));
}

async function optionalLstat(filePath, fsOps) {
  try {
    return await fsOps.lstat(filePath);
  } catch (error) {
    if (error.code === 'ENOENT') {
      return null;
    }
    throw error;
  }
}

function displayPath(root, absolutePath) {
  const relative = path.relative(root, absolutePath);
  return relative === '' ? '.' : toPosix(relative);
}

async function inspectTargetRoot(root, fsOps) {
  const status = await optionalLstat(root, fsOps);
  if (status === null) {
    return;
  }
  if (status.isSymbolicLink()) {
    throw new Error(`target root is a symlink: ${root}`);
  }
  if (!status.isDirectory()) {
    throw new Error(`target root is not a directory: ${root}`);
  }
}

async function inspectDestination(root, relativePath, fsOps, migratableSymlinks = new Set()) {
  const parts = relativePath.split('/');
  let current = root;

  for (let index = 0; index < parts.length - 1; index += 1) {
    current = path.join(current, parts[index]);
    const status = await optionalLstat(current, fsOps);
    if (status === null) {
      return null;
    }
    if (status.isSymbolicLink()) {
      if (migratableSymlinks.has(toPosix(path.relative(root, current)))) {
        return null;
      }
      throw new Error(`parent path is a symlink: ${displayPath(root, current)}`);
    }
    if (!status.isDirectory()) {
      throw new Error(`parent path is not a directory: ${displayPath(root, current)}`);
    }
  }

  const destination = path.join(root, ...parts);
  const status = await optionalLstat(destination, fsOps);
  if (status?.isSymbolicLink()) {
    throw new Error(`destination is a symlink: ${relativePath}`);
  }
  if (status?.isDirectory()) {
    throw new Error(`destination is a directory: ${relativePath}`);
  }
  if (status !== null && !status.isFile()) {
    throw new Error(`destination is not a regular file: ${relativePath}`);
  }
  return status;
}

async function loadState(root, fsOps) {
  const status = await inspectDestination(root, STATE_FILE, fsOps);
  if (status === null) {
    return { state: null, bytes: null };
  }

  const bytes = await fsOps.readFile(path.join(root, STATE_FILE));
  let state;
  try {
    state = JSON.parse(bytes.toString('utf8'));
  } catch (error) {
    throw new Error(`invalid install state: ${error.message}`, { cause: error });
  }
  return { state: validateState(state), bytes };
}

function isCurrentPlatformPath(relativePath, platform) {
  if (platform === 'claude') {
    return relativePath.startsWith('.claude/');
  }
  return relativePath.startsWith('.codex/') || relativePath.startsWith('.agents/');
}

function serializeState(platform, revision, existingFiles, filePlan) {
  const files = {
    ...existingFiles,
  };
  for (const entry of filePlan) {
    files[entry.path] = {
      sha256: entry.sha256,
      source: entry.source,
    };
  }
  const sortedFiles = Object.fromEntries(
    Object.entries(files).sort(([left], [right]) => compareStrings(left, right)),
  );
  return Buffer.from(`${JSON.stringify({
    schemaVersion: STATE_SCHEMA_VERSION,
    platform,
    catalogRevision: revision,
    files: sortedFiles,
  }, null, 2)}\n`);
}

async function preflightFiles(
  root,
  filePlan,
  ownedFiles,
  force,
  fsOps,
  migratableSymlinks = new Set(),
) {
  const collisions = [];
  const planned = [];

  for (const entry of filePlan) {
    const status = await inspectDestination(root, entry.path, fsOps, migratableSymlinks);
    if (status === null) {
      planned.push({ ...entry, action: 'create' });
      continue;
    }

    const existing = await fsOps.readFile(path.join(root, ...entry.path.split('/')));
    if (existing.equals(entry.contents)) {
      planned.push({
        ...entry,
        action: Object.hasOwn(ownedFiles, entry.path) ? 'unchanged' : 'adopt',
      });
    } else if (Object.hasOwn(ownedFiles, entry.path) || force) {
      planned.push({ ...entry, action: 'replace' });
    } else {
      collisions.push(entry.path);
    }
  }

  if (collisions.length > 0) {
    throw new Error(
      `unowned destination differs:\n${collisions.map((file) => `  ${file}`).join('\n')}`,
    );
  }
  return planned;
}

async function prepareLegacyMigrations(root, migrations, fsOps) {
  const prepared = [];
  try {
    for (const migration of migrations) {
      const source = path.join(root, ...migration.path.split('/'));
      const backup = `${source}.agents-catalog-legacy-${randomUUID()}`;
      await fsOps.rename(source, backup);
      prepared.push({ source, backup });
      await fsOps.mkdir(source);
    }
    return prepared;
  } catch (error) {
    for (const { source, backup } of prepared.reverse()) {
      try {
        await fsOps.rm(source, { recursive: true, force: true });
        await fsOps.rename(backup, source);
      } catch {
        // Preserve the original error; the backup remains available for recovery.
      }
    }
    throw error;
  }
}

async function discardLegacyMigrationBackups(prepared, fsOps) {
  for (const { backup } of prepared) {
    await fsOps.unlink(backup);
  }
}

async function restoreLegacyMigrationBackups(prepared, fsOps) {
  for (const { source, backup } of [...prepared].reverse()) {
    await fsOps.rm(source, { recursive: true, force: true });
    await fsOps.rename(backup, source);
  }
}

async function atomicWrite(destination, contents, mode, fsOps) {
  const temporary = path.join(
    path.dirname(destination),
    `.${path.basename(destination)}.tmp-${process.pid}-${randomUUID()}`,
  );
  try {
    await fsOps.writeFile(temporary, contents, { flag: 'wx', mode });
    await fsOps.chmod(temporary, mode);
    await fsOps.rename(temporary, destination);
  } catch (error) {
    try {
      await fsOps.unlink(temporary);
    } catch (cleanupError) {
      if (cleanupError.code !== 'ENOENT') {
        error.cleanupError = cleanupError;
      }
    }
    throw error;
  }
}

function copyFailure(error, copied, failed, pending) {
  const wrapped = new Error(
    `install write failed at ${failed}: ${error.message}; `
      + `copied=[${copied.join(', ')}] pending=[${pending.join(', ')}]`,
    { cause: error },
  );
  wrapped.report = { copied, failed, pending };
  return wrapped;
}

function normalizeOptions(options) {
  if (!isPlainObject(options)) {
    throw new Error('install options are required');
  }
  const {
    platform,
    scope = 'user',
    target,
    catalogRoot,
    dryRun = false,
    force = false,
    migrateLegacy = false,
    home = homedir(),
    cwd = process.cwd(),
  } = options;

  if (platform !== 'claude' && platform !== 'codex') {
    throw new Error(`unsupported platform "${platform}"`);
  }
  if (scope !== 'user' && scope !== 'project') {
    throw new Error(`unsupported scope "${scope}"`);
  }
  if (typeof catalogRoot !== 'string' || catalogRoot.length === 0) {
    throw new Error('catalogRoot is required');
  }
  if (target !== undefined && (typeof target !== 'string' || target.length === 0)) {
    throw new Error('target must be a non-empty path');
  }
  if (typeof home !== 'string' || typeof cwd !== 'string') {
    throw new Error('home and cwd must be paths');
  }

  return {
    platform,
    scope,
    root: path.resolve(target ?? (scope === 'user' ? home : cwd)),
    catalogRoot: path.resolve(catalogRoot),
    dryRun: Boolean(dryRun),
    force: Boolean(force),
    migrateLegacy: Boolean(migrateLegacy),
  };
}

export async function install(options) {
  const normalized = normalizeOptions(options);
  const fsOps = {
    ...fs,
    ...(options.fsOps ?? {}),
  };

  const [catalog, policy, revision] = await Promise.all([
    loadCatalog(normalized.catalogRoot),
    loadModelPolicy(normalized.catalogRoot),
    getCatalogRevision(normalized.catalogRoot),
  ]);
  const validationErrors = validateCatalog(catalog, policy);
  if (validationErrors.length > 0) {
    throw new Error(`catalog validation failed:\n${validationErrors.join('\n')}`);
  }

  const filePlan = createFilePlan(
    normalized.catalogRoot,
    buildInstallSet(catalog, normalized.platform),
  );
  await inspectTargetRoot(normalized.root, fsOps);
  const existing = await loadState(normalized.root, fsOps);
  const existingFiles = existing.state?.files ?? {};
  const migrations = await findLegacyMigrations(
    normalized.root,
    normalized.platform,
    filePlan,
    normalized.migrateLegacy,
    fsOps,
  );
  const migratableSymlinks = new Set(migrations.map(({ path: file }) => file));
  const plannedFiles = await preflightFiles(
    normalized.root,
    filePlan,
    existingFiles,
    normalized.force,
    fsOps,
    migratableSymlinks,
  );
  const plannedPaths = new Set(filePlan.map((entry) => entry.path));
  const stale = Object.keys(existingFiles)
    .filter(
      (file) => isCurrentPlatformPath(file, normalized.platform) && !plannedPaths.has(file),
    )
    .sort();
  const stateBytes = serializeState(
    normalized.platform,
    revision,
    existingFiles,
    filePlan,
  );
  const stateAction = existing.bytes?.equals(stateBytes)
    ? 'unchanged'
    : existing.bytes === null ? 'create' : 'replace';
  const report = {
    root: normalized.root,
    platform: normalized.platform,
    scope: normalized.scope,
    dryRun: normalized.dryRun,
    force: normalized.force,
    migrateLegacy: normalized.migrateLegacy,
    catalogRevision: revision,
    migrations,
    files: plannedFiles.map(({
      path: file,
      source,
      sha256: hash,
      mode,
      action,
    }) => ({
      path: file,
      source,
      sha256: hash,
      mode,
      action,
    })),
    stale,
    stateAction,
  };

  if (normalized.dryRun) {
    return report;
  }

  const preparedMigrations = await prepareLegacyMigrations(
    normalized.root,
    migrations,
    fsOps,
  );
  const changed = plannedFiles.filter(
    ({ action }) => action === 'create' || action === 'replace',
  );
  const copied = [];
  let installError;
  try {
    for (let index = 0; index < changed.length; index += 1) {
      const entry = changed[index];
      try {
        const destination = path.join(normalized.root, ...entry.path.split('/'));
        await fsOps.mkdir(path.dirname(destination), { recursive: true });
        await atomicWrite(destination, entry.contents, entry.mode, fsOps);
        copied.push(entry.path);
      } catch (error) {
        throw copyFailure(
          error,
          copied,
          entry.path,
          [
            ...changed.slice(index + 1).map(({ path: file }) => file),
            ...(stateAction === 'unchanged' ? [] : [STATE_FILE]),
          ],
        );
      }
    }

    if (stateAction !== 'unchanged') {
      try {
        const statePath = path.join(normalized.root, STATE_FILE);
        await fsOps.mkdir(normalized.root, { recursive: true });
        await atomicWrite(statePath, stateBytes, 0o644, fsOps);
      } catch (error) {
        throw copyFailure(error, copied, STATE_FILE, []);
      }
    }
  } catch (error) {
    installError = error;
    throw error;
  } finally {
    try {
      if (installError) {
        await restoreLegacyMigrationBackups(preparedMigrations, fsOps);
      } else {
        await discardLegacyMigrationBackups(preparedMigrations, fsOps);
      }
    } catch (cleanupError) {
      if (installError) {
        installError.cleanupError = cleanupError;
      } else {
        throw cleanupError;
      }
    }
  }

  return report;
}

function printReport(report) {
  const prefix = report.dryRun ? 'DRY-RUN' : 'INSTALL';
  for (const migration of report.migrations) {
    console.log(`${prefix} migrate ${migration.path} from ${migration.target}`);
  }
  for (const file of report.files) {
    console.log(`${prefix} ${file.action} ${file.path}`);
  }
  for (const file of report.stale) {
    console.log(`${prefix} stale-retained ${file}`);
  }
  console.log(`${prefix} ${report.stateAction} ${STATE_FILE}`);
}

const isDirectExecution = process.argv[1] !== undefined
  && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);

if (isDirectExecution) {
  try {
    const options = parseArgs(process.argv.slice(2));
    const catalogRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
    printReport(await install({ ...options, catalogRoot }));
  } catch (error) {
    if (error.message === USAGE) {
      console.error(USAGE);
    } else {
      console.error(`ERROR ${error.message}`);
    }
    process.exitCode = 1;
  }
}
