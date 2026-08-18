import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import {
  chmod,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import test from 'node:test';

const execFileAsync = promisify(execFile);
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const JOURNAL_ENTRY = path.join(ROOT, 'scripts', 'codex-journal-entry.sh');
const STOP_HOOKS = [
  'claude-journal-stop.sh',
  'codex-journal-stop.sh',
  'kiro-ide-journal-stop.sh',
  'kiro-journal-stop.sh',
];

const FAKE_CODEX = [
  '#!/usr/bin/env bash',
  'set -euo pipefail',
  'output=""',
  'while (($#)); do',
  '  if [[ "$1" == "--output-last-message" ]]; then',
  '    output="$2"',
  '    shift 2',
  '  else',
  '    shift',
  '  fi',
  'done',
  'input="$(cat)"',
  'printf "%s" "$input" > "$JOURNAL_PROMPT_CAPTURE"',
  'printf "%s" "journal-summary" > "$output"',
].join('\n');

const AGENT_DISPLAY = {
  claude: 'Claude',
  codex: 'Codex',
  kiro: 'Kiro',
};

async function currentEntryPath() {
  const { stdout } = await execFileAsync('date', ['+%Y/%m/%Y-%m-%d']);
  const parts = stdout.trim().split('/');
  parts[parts.length - 1] += '.md';
  return parts;
}

for (const agent of Object.keys(AGENT_DISPLAY)) {
  test(`journal prompt preserves technical context for ${agent}`, async (t) => {
    const fixture = await mkdtemp(path.join(tmpdir(), 'journal-hook-'));
    t.after(() => rm(fixture, { recursive: true, force: true }));

    const fakeCodex = path.join(fixture, 'codex');
    const promptCapture = path.join(fixture, 'prompt.txt');
    await writeFile(fakeCodex, FAKE_CODEX);
    await chmod(fakeCodex, 0o755);

    await execFileAsync(
      'bash',
      [
        JOURNAL_ENTRY,
        '--agent',
        agent,
        [
          'User asked: compare the request serializer and middleware.',
          'Assistant responded (summary): inspected PhotonRequest, ran 166 tests,',
          'and confirmed the header normalization behavior.',
        ].join('\n'),
      ],
      {
        cwd: ROOT,
        env: {
          ...process.env,
          CODEX_BIN: fakeCodex,
          JOURNAL_PROMPT_CAPTURE: promptCapture,
          VAULT_ROOT: path.join(fixture, 'vault'),
        },
      },
    );

    const prompt = await readFile(promptCapture, 'utf8');
    assert.match(prompt, /technical work journaler/);
    assert.match(prompt, /40-80 words/);
    assert.match(prompt, /concrete technical details/);
    assert.match(prompt, /166 tests/);
    assert.match(prompt, new RegExp(`Using ${AGENT_DISPLAY[agent]}`));

    const entryPath = path.join(
      fixture,
      'vault',
      'journals',
      'entries',
      ...await currentEntryPath(),
    );
    assert.match(await readFile(entryPath, 'utf8'), /journal-summary/);
  });
}

test('all stop hooks pass a larger technical context window', async () => {
  for (const hook of STOP_HOOKS) {
    const source = await readFile(path.join(ROOT, 'scripts', hook), 'utf8');
    assert.match(source, /PROMPT_TRUNCATED="\$\{PROMPT:0:1000\}"/, hook);
    assert.match(source, /RESPONSE_TRUNCATED="\$\{RESPONSE:0:1800\}"/, hook);
  }
});
