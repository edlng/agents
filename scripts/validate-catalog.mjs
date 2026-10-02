import path from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  loadCatalog,
  loadModelPolicy,
  validateCatalog,
} from './catalog-lib.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const EXPECTED = {
  agents: 8,
  skills: 13,
  universal: 5,
  claude: 8,
  codex: 8,
};

try {
  const [catalog, policy] = await Promise.all([
    loadCatalog(root),
    loadModelPolicy(root),
  ]);
  const errors = validateCatalog(catalog, policy);

  for (const key of ['agents', 'skills']) {
    if (catalog[key].length !== EXPECTED[key]) {
      errors.push(
        `expected ${EXPECTED[key]} ${key}, found ${catalog[key].length}`,
      );
    }
  }
  for (const platform of ['universal', 'claude', 'codex']) {
    const count = catalog.skillVariants[platform].length;
    if (count !== EXPECTED[platform]) {
      errors.push(
        `expected ${EXPECTED[platform]} ${platform} skills, found ${count}`,
      );
    }
  }

  if (errors.length > 0) {
    for (const error of errors) {
      console.error(`ERROR ${error}`);
    }
    process.exitCode = 1;
  } else {
    console.log('Catalog valid: 8 agents, 13 skills (5 universal, 8 Claude, 8 Codex)');
  }
} catch (error) {
  console.error(`ERROR ${error.message}`);
  process.exitCode = 1;
}
