import assert from 'node:assert/strict';
import { access, readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { changedSkills } from '../scripts/check-upstream.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('changedSkills groups upstream files by the ported skill directory', () => {
  const source = {
    skills: {
      tdd: 'pstack/skills/tdd',
      'blast-radius': 'pstack/skills/blast-radius',
    },
  };
  const changes = changedSkills(source, [
    { filename: 'pstack/skills/tdd/SKILL.md', status: 'modified' },
    { filename: 'pstack/skills/tdd-extra/SKILL.md', status: 'added' },
    { filename: 'pstack/README.md', status: 'modified' },
  ]);

  assert.deepEqual([...changes.keys()], ['tdd']);
  assert.deepEqual(changes.get('tdd').map((file) => file.filename), ['pstack/skills/tdd/SKILL.md']);
});

test('every skill tracked in upstream.json exists as a universal skill', async () => {
  const lock = JSON.parse(await readFile(path.join(root, 'upstream.json'), 'utf8'));
  for (const source of lock.sources) {
    assert.match(source.ref, /^[0-9a-f]{40}$/, `${source.name} ref must be a full commit SHA`);
    for (const skill of Object.keys(source.skills)) {
      await access(path.join(root, 'skills', 'universal', skill, 'SKILL.md'));
    }
  }
});
