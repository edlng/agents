import { lstat, mkdir, readFile, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { loadCatalog } from './catalog-lib.mjs';

// Installs catalog agents and universal skills into a Kiro home (~/.kiro by default).
// It never deletes files and never writes through a symlink, so linked skills such as
// unslop keep pointing at their source. Agent configs keep keys that exist only in the
// installed copy (for example policy-injected toolsSettings), while catalog keys win.

const REPOSITORY_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function isPlainObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

export function mergeKiroConfig(catalogConfig, installedConfig) {
  if (!isPlainObject(installedConfig)) return structuredClone(catalogConfig);
  const merged = structuredClone(installedConfig);
  for (const [key, value] of Object.entries(catalogConfig)) {
    merged[key] = isPlainObject(value) && isPlainObject(merged[key])
      ? mergeKiroConfig(value, merged[key])
      : structuredClone(value);
  }
  return merged;
}

async function readOptional(filePath) {
  try {
    return await readFile(filePath);
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw error;
  }
}

async function isSymlink(filePath) {
  try {
    return (await lstat(filePath)).isSymbolicLink();
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
}

export async function planKiroInstall(kiroRoot, repositoryRoot = REPOSITORY_ROOT) {
  const catalog = await loadCatalog(repositoryRoot);
  const entries = [];

  for (const agent of catalog.agents) {
    if (!agent.manifest.platforms.includes('kiro')) continue;
    const name = agent.manifest.name;
    const source = agent.paths.directory;
    const config = JSON.parse(await readFile(path.join(source, 'kiro.json'), 'utf8'));
    config.prompt = `file://./${name}-prompt.md`;
    const configPath = path.join(kiroRoot, 'agents', `${name}.json`);
    const installed = await readOptional(configPath);
    const merged = mergeKiroConfig(config, installed && JSON.parse(installed.toString('utf8')));
    entries.push(
      { path: path.join(kiroRoot, 'agents', `${name}-prompt.md`), contents: await readFile(path.join(source, 'kiro-prompt.md')) },
      { path: configPath, contents: Buffer.from(`${JSON.stringify(merged, null, 2)}\n`) },
    );
  }

  for (const skill of catalog.skillVariants.universal) {
    for (const file of skill.files) {
      if (file.name.startsWith('agents/')) continue; // Codex-only invocation policy
      entries.push({ path: path.join(kiroRoot, 'skills', skill.name, file.name), contents: file.contents, mode: file.mode });
    }
  }
  for (const file of catalog.sharedFiles) {
    entries.push({ path: path.join(kiroRoot, 'skills', '_shared', file.name), contents: file.contents, mode: file.mode });
  }

  for (const entry of entries) {
    if (await isSymlink(entry.path)) {
      entry.action = 'skip-symlink';
      continue;
    }
    const existing = await readOptional(entry.path);
    entry.action = existing === null ? 'create' : existing.equals(entry.contents) ? 'unchanged' : 'replace';
  }
  return entries;
}

export async function installKiro({ kiroRoot, dryRun }) {
  const entries = await planKiroInstall(kiroRoot);
  for (const entry of entries) {
    if (!dryRun && (entry.action === 'create' || entry.action === 'replace')) {
      await mkdir(path.dirname(entry.path), { recursive: true });
      await writeFile(entry.path, entry.contents, entry.mode ? { mode: entry.mode } : undefined);
    }
  }
  return entries;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const dryRun = args.includes('--dry-run');
  const rootIndex = args.indexOf('--root');
  const kiroRoot = rootIndex === -1 ? path.join(os.homedir(), '.kiro') : path.resolve(args[rootIndex + 1]);
  const entries = await installKiro({ kiroRoot, dryRun });
  const prefix = dryRun ? 'DRY-RUN' : 'INSTALL';
  for (const entry of entries) {
    if (entry.action !== 'unchanged') console.log(`${prefix} ${entry.action} ${path.relative(kiroRoot, entry.path)}`);
  }
  const counts = entries.reduce((total, { action }) => ({ ...total, [action]: (total[action] ?? 0) + 1 }), {});
  console.log(`${prefix} summary ${JSON.stringify(counts)}`);
}
