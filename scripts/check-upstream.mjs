import { execFile } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const execFileAsync = promisify(execFile);
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// Maps compare-API files to the ported skills whose upstream directory they sit in.
export function changedSkills(source, files) {
  const changes = new Map();
  for (const file of files) {
    for (const [skill, upstreamPath] of Object.entries(source.skills)) {
      if (file.filename.startsWith(`${upstreamPath}/`)) {
        if (!changes.has(skill)) changes.set(skill, []);
        changes.get(skill).push(file);
      }
    }
  }
  return changes;
}

async function compare(source) {
  const { stdout } = await execFileAsync(
    'gh',
    ['api', `repos/${source.repo}/compare/${source.ref}...${source.branch}`],
    { maxBuffer: 64 * 1024 * 1024 },
  );
  return JSON.parse(stdout);
}

async function main() {
  const showDiff = process.argv.includes('--diff');
  const lock = JSON.parse(await readFile(path.join(root, 'upstream.json'), 'utf8'));
  let outdated = false;

  for (const source of lock.sources) {
    const result = await compare(source);
    const head = result.commits.at(-1)?.sha ?? source.ref;
    const changes = changedSkills(source, result.files ?? []);

    if (changes.size === 0) {
      console.log(`${source.name}: up to date at ${source.ref.slice(0, 7)}`);
      continue;
    }

    outdated = true;
    console.log(
      `${source.name}: ${changes.size} skill(s) changed upstream since ${source.ref.slice(0, 7)} (head ${head})`,
    );
    console.log(`  https://github.com/${source.repo}/compare/${source.ref}...${head}`);
    for (const [skill, files] of changes) {
      console.log(`  ${skill}`);
      for (const file of files) {
        console.log(`    ${file.status} ${file.filename}`);
        if (showDiff) console.log(file.patch ?? '    (patch too large; open the compare URL)');
      }
    }
  }

  if (outdated) {
    console.log(
      '\nMerge the upstream changes into skills/universal/<skill>/ by hand, keeping local adaptations, then set "ref" in upstream.json to the head SHA.',
    );
    process.exitCode = 1;
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(`check-upstream failed: ${error.message}`);
    process.exitCode = 2;
  });
}
