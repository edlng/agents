import path from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  loadCatalog,
  loadModelPolicy,
  validateCatalog,
} from './catalog-lib.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const EXPECTED = {
  agents: 17,
  skills: 41,
  universal: 18,
  claude: 23,
  codex: 23,
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
    console.log('Catalog valid: 17 agents, 41 skills (18 universal, 23 Claude, 23 Codex)');
  }
} catch (error) {
  console.error(`ERROR ${error.message}`);
  process.exitCode = 1;
}
