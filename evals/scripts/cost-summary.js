#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');

const METRICS_FILE = path.join(__dirname, '../metrics/token_usage.jsonl');

if (!fs.existsSync(METRICS_FILE)) {
  console.log('\nNo eval metrics found. Run `make eval` first.\n');
  process.exit(0);
}

const content = fs.readFileSync(METRICS_FILE, 'utf8').trim();
if (!content) {
  console.log('\nMetrics file is empty.\n');
  process.exit(0);
}

const entries = content.split('\n').filter(Boolean).map(line => JSON.parse(line));

// Group by agent
const byAgent = {};
for (const entry of entries) {
  if (!byAgent[entry.agent]) byAgent[entry.agent] = [];
  byAgent[entry.agent].push(entry);
}

const now = new Date().toISOString().slice(0, 10);
console.log(`\n=== Eval Cost Summary (${now}) ===\n`);

const COL = { agent: 25, runs: 4, avgIn: 7, avgOut: 8, dur: 5, cost: 12 };
const header = `${'Agent'.padEnd(COL.agent)} | ${'Runs'.padStart(COL.runs)} | ${'Avg In'.padStart(COL.avgIn)} | ${'Avg Out'.padStart(COL.avgOut)} | ${'Avg s'.padStart(COL.dur)} | ${'Total Cost'.padStart(COL.cost)}`;
const divider = '-'.repeat(header.length);

console.log(header);
console.log(divider);

let grandTotal = 0;
let grandRuns = 0;

for (const [agent, runs] of Object.entries(byAgent).sort()) {
  const avgIn = Math.round(runs.reduce((s, r) => s + r.input_tokens, 0) / runs.length);
  const avgOut = Math.round(runs.reduce((s, r) => s + r.output_tokens, 0) / runs.length);
  const avgDur = Math.round(runs.reduce((s, r) => s + (r.duration_s || 0), 0) / runs.length);
  const totalCost = runs.reduce((s, r) => s + r.total_cost_usd, 0);
  grandTotal += totalCost;
  grandRuns += runs.length;
  console.log(
    `${agent.padEnd(COL.agent)} | ${String(runs.length).padStart(COL.runs)} | ${String(avgIn).padStart(COL.avgIn)} | ${String(avgOut).padStart(COL.avgOut)} | ${String(avgDur).padStart(COL.dur)} | ${'$' + totalCost.toFixed(4).padStart(COL.cost - 1)}`
  );
}

console.log(divider);
console.log(
  `${'TOTAL'.padEnd(COL.agent)} | ${String(grandRuns).padStart(COL.runs)} | ${''.padStart(COL.avgIn)} | ${''.padStart(COL.avgOut)} | ${''.padStart(COL.dur)} | ${'$' + grandTotal.toFixed(4).padStart(COL.cost - 1)}`
);
console.log('');
