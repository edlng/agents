/**
 * GLIDE Performance Skill — Benchmark Runner
 *
 * Runs the same ride-sharing workload against two Valkey layer implementations
 * and compares latency. Measures the real-world impact of following the
 * skill's recommendations.
 *
 * Usage:
 *   node app-benchmark.js                        # compare both
 *   node app-benchmark.js --only baseline         # single impl
 *   node app-benchmark.js --only skill            # single impl
 *   node app-benchmark.js --iterations 500        # more samples
 *
 * Prerequisites:
 *   docker run -d --name valkey -p 6379:6379 valkey/valkey:latest
 *   npm install @valkey/valkey-glide
 */

let ITERATIONS = 50;
let runOnly = null;

for (let i = 2; i < process.argv.length; i++) {
  if (process.argv[i] === "--iterations") ITERATIONS = parseInt(process.argv[++i], 10);
  if (process.argv[i] === "--only") runOnly = process.argv[++i];
}

// ---------------------------------------------------------------------------
// Timing
// ---------------------------------------------------------------------------

function now() {
  return Number(process.hrtime.bigint()) / 1e6;
}

function stats(samples) {
  const sorted = [...samples].sort((a, b) => a - b);
  const sum = sorted.reduce((a, b) => a + b, 0);
  return {
    mean: (sum / sorted.length).toFixed(2),
    p50: sorted[Math.floor(sorted.length * 0.5)].toFixed(2),
    p99: sorted[Math.floor(sorted.length * 0.99)].toFixed(2),
  };
}

const WARMUP_RATIO = 0.1; // 10% of iterations discarded as warmup

async function measure(fn, n) {
  const count = n || ITERATIONS;
  const warmup = Math.max(1, Math.floor(count * WARMUP_RATIO));
  const samples = [];

  // Warmup — discard these to avoid JIT / connection cold-start skew
  for (let i = 0; i < warmup; i++) {
    await fn(i);
  }

  for (let i = 0; i < count; i++) {
    const t0 = now();
    await fn(i);
    samples.push(now() - t0);
  }
  return stats(samples);
}


// ---------------------------------------------------------------------------
// Workload
// ---------------------------------------------------------------------------

async function runWorkload(impl, label) {
  console.log(`\n${"─".repeat(72)}`);
  console.log(`  ${label}`);
  console.log(`  Iterations per operation: ${ITERATIONS}`);
  console.log(`${"─".repeat(72)}`);

  await impl.initialize();
  const results = {};

  // -- 1. Driver Profiles --
  const driverProfile = {
    name: "Alex Rivera", phone: "555-0100", vehicleType: "sedan",
    licensePlate: "ABC-1234", rating: "4.92", totalTrips: "1847",
    currentZone: "downtown",
  };
  for (let i = 0; i < 50; i++) {
    await impl.setDriverProfile(`driver-${i}`, { ...driverProfile, name: `Driver ${i}` });
  }

  results["setDriverProfile"] = await measure((i) =>
    impl.setDriverProfile(`driver-${i % 50}`, { ...driverProfile, name: `Driver ${i}` })
  );
  results["getDriverProfile"] = await measure((i) =>
    impl.getDriverProfile(`driver-${i % 50}`)
  );
  results["updateDriverField"] = await measure((i) =>
    impl.updateDriverField(`driver-${i % 50}`, "currentZone", `zone-${i % 8}`)
  );

  const driverBatch = Array.from({ length: 20 }, (_, i) => `driver-${i}`);
  results["getMultipleDriverSummaries (20)"] = await measure(() =>
    impl.getMultipleDriverSummaries(driverBatch)
  );

  // -- 2. Trips --
  const tripIds = Array.from({ length: 20 }, (_, i) => `trip-${i}`);
  const tripPayload = tripIds.map((id) => ({
    tripId: id, data: `{"from":"A","to":"B","fare":12.50}`,
  }));
  await impl.setTrips(tripPayload);

  results["getTrips (20)"] = await measure(() => impl.getTrips(tripIds));
  results["setTrips (20)"] = await measure(() => impl.setTrips(tripPayload));

  const archivePayload = Array.from({ length: 500 }, (_, i) => ({
    tripId: `archive-${i}`, data: `{"status":"completed"}`,
  }));
  results["archiveTripsBatch (500)"] = await measure(
    () => impl.archiveTripsBatch(archivePayload),
    Math.min(ITERATIONS, 50)
  );

  // -- 3. Rider Sessions --
  results["saveRiderSession"] = await measure((i) =>
    impl.saveRiderSession(`rsess-${i}`, {
      riderId: `rider-${i}`, location: "40.7128,-74.0060",
      paymentMethod: "card_visa", activeTrip: "",
    })
  );
  results["getRiderSession"] = await measure((i) =>
    impl.getRiderSession(`rsess-${i % 50}`)
  );
  results["updateRiderLocation"] = await measure((i) =>
    impl.updateRiderLocation(`rsess-${i % 50}`, `${40.71 + Math.random() * 0.01},${-74.00 + Math.random() * 0.01}`)
  );

  // -- 4. Surge Pricing & Analytics --
  results["recordRideRequest"] = await measure((i) =>
    impl.recordRideRequest(`zone-${i % 12}`, Math.floor(Date.now() / 1000))
  );

  const zoneIds = Array.from({ length: 12 }, (_, i) => `zone-${i}`);
  results["getZoneStats (12)"] = await measure(() => impl.getZoneStats(zoneIds));

  const earningsEntries = Array.from({ length: 20 }, (_, i) => ({
    driverId: `driver-${i}`, earnings: Math.floor(Math.random() * 500) + 50,
  }));
  results["submitEarnings (20)"] = await measure(() =>
    impl.submitEarnings("lb:earnings:daily", earningsEntries)
  );
  results["getTopEarners (10)"] = await measure(() =>
    impl.getTopEarners("lb:earnings:daily", 10)
  );


  // -- 5. Dispatch Queue --
  results["enqueueRideRequests (10)"] = await measure(() =>
    impl.enqueueRideRequests("dispatch:queue", Array.from({ length: 10 }, (_, i) => `req-${i}`))
  );
  // dequeueRideRequest skipped — blocking call not suitable for tight benchmark loop

  // -- 6. Vehicle Availability --
  const availZones = Array.from({ length: 30 }, (_, i) => ({
    zoneId: `avail-zone-${i}`, available: 50,
  }));
  await impl.setZoneAvailability(availZones);

  results["setZoneAvailability (30)"] = await measure(() =>
    impl.setZoneAvailability(availZones)
  );

  const availZoneIds = availZones.map((z) => z.zoneId);
  results["getZoneAvailability (30)"] = await measure(() =>
    impl.getZoneAvailability(availZoneIds)
  );
  results["claimVehicle"] = await measure((i) =>
    impl.claimVehicle(`avail-zone-${i % 30}`, 1)
  );

  // -- 7. Feature Flags --
  const flagNames = Array.from({ length: 10 }, (_, i) => `flag-${i}`);
  for (const f of flagNames) {
    await impl.setFeatureFlag(f, "true", 3600);
  }

  results["setFeatureFlag"] = await measure((i) =>
    impl.setFeatureFlag(`flag-${i % 10}`, i % 2 === 0 ? "true" : "false", 3600)
  );
  results["resolveFeatureFlags (10)"] = await measure(() =>
    impl.resolveFeatureFlags(flagNames)
  );

  // -- 8. ETA Cache --
  for (let i = 0; i < 30; i++) {
    await impl.updateEtaField(`eta-trip-${i}`, "pickupEta", "5");
    await impl.updateEtaField(`eta-trip-${i}`, "dropoffEta", "18");
    await impl.updateEtaField(`eta-trip-${i}`, "distanceKm", "7.2");
    await impl.updateEtaField(`eta-trip-${i}`, "lastUpdated", String(Date.now()));
  }

  results["updateEtaField"] = await measure((i) =>
    impl.updateEtaField(`eta-trip-${i % 30}`, "pickupEta", String(3 + (i % 10)))
  );
  results["getEta"] = await measure((i) =>
    impl.getEta(`eta-trip-${i % 30}`)
  );
  results["clearEtaField"] = await measure((i) =>
    impl.clearEtaField(`eta-trip-${i % 30}`, "dropoffEta")
  );

  // -- 9. Rate Limiting --
  results["incrementRateLimit"] = await measure((i) =>
    impl.incrementRateLimit(`rider-${i % 50}`, 60)
  );

  // -- 10. Driver Zones --
  const zoneDrivers = Array.from({ length: 10 }, (_, i) => `driver-${i}`);
  results["addDriversToZone (10)"] = await measure((i) =>
    impl.addDriversToZone(`zone-${i % 8}`, zoneDrivers)
  );
  results["checkDriversInZone (10)"] = await measure((i) =>
    impl.checkDriversInZone(`zone-${i % 8}`, zoneDrivers)
  );

  // -- 11. Operations Dashboard --
  const { GlideClient } = require("@valkey/valkey-glide");
  const seedClient = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
  });
  await seedClient.set("ops:activeDrivers", "342");
  await seedClient.set("ops:activeTrips", "128");
  await seedClient.set("ops:totalRidesToday", "4521");
  await seedClient.set("ops:avgWaitTime", "4.2");
  await seedClient.lpush("ops:recentCompletions", [
    "trip-9901", "trip-9902", "trip-9903", "trip-9904", "trip-9905",
  ]);
  seedClient.close();

  results["getOperationsDashboard"] = await measure(() =>
    impl.getOperationsDashboard()
  );

  await impl.shutdown();
  return results;
}


// ---------------------------------------------------------------------------
// Reporting
// ---------------------------------------------------------------------------

function printComparison(baselineResults, skillResults) {
  console.log(`\n${"=".repeat(76)}`);
  console.log("  RESULTS COMPARISON");
  console.log(`${"=".repeat(76)}\n`);

  const col1 = 36, col2 = 16, col3 = 18;
  const header = "Operation".padEnd(col1) + "Baseline (mean)".padEnd(col2) + "Skill (mean)".padEnd(col3) + "Speedup";
  console.log(`  ${header}`);
  console.log(`  ${"─".repeat(header.length)}`);

  for (const op of Object.keys(baselineResults)) {
    const bMean = parseFloat(baselineResults[op].mean);
    const sMean = parseFloat(skillResults[op].mean);
    const speedup = sMean > 0 ? (bMean / sMean).toFixed(1) : "∞";
    console.log(
      `  ${op.padEnd(col1)}${(bMean.toFixed(2) + "ms").padEnd(col2)}${(sMean.toFixed(2) + "ms").padEnd(col3)}${speedup}x`
    );
  }

  const baselineTotal = Object.values(baselineResults).reduce((s, r) => s + parseFloat(r.mean), 0);
  const skillTotal = Object.values(skillResults).reduce((s, r) => s + parseFloat(r.mean), 0);
  const totalSpeedup = skillTotal > 0 ? (baselineTotal / skillTotal).toFixed(1) : "∞";
  console.log(`  ${"─".repeat(header.length)}`);
  console.log(
    `  ${"TOTAL (sum of means)".padEnd(col1)}${(baselineTotal.toFixed(2) + "ms").padEnd(col2)}${(skillTotal.toFixed(2) + "ms").padEnd(col3)}${totalSpeedup}x`
  );
  console.log("");
}

function printSingle(label, results) {
  console.log(`\n${"=".repeat(72)}`);
  console.log(`  RESULTS: ${label}`);
  console.log(`${"=".repeat(72)}\n`);
  console.log(`  ${"Operation".padEnd(36)}${"Mean".padEnd(12)}${"P50".padEnd(12)}P99`);
  console.log(`  ${"─".repeat(68)}`);
  for (const [op, s] of Object.entries(results)) {
    console.log(`  ${op.padEnd(36)}${(s.mean + "ms").padEnd(12)}${(s.p50 + "ms").padEnd(12)}${s.p99}ms`);
  }
  console.log("");
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  console.log("GLIDE Performance Skill — Ride-Sharing Benchmark");
  console.log(`Date: ${new Date().toISOString()}`);
  console.log(`Iterations per operation: ${ITERATIONS}\n`);

  let baselineResults = null;
  let skillResults = null;

  // Build the list of implementations to run
  const impls = [];

  if (runOnly !== "skill") {
    try {
      impls.push({ key: "baseline", impl: require("./valkey-baseline"), label: "valkey-baseline (without skill)" });
    } catch (err) {
      if (err.code === "MODULE_NOT_FOUND") {
        console.log("\n  valkey-baseline.js not found — skipping.");
        console.log("  Generate it: ask your AI to implement valkey-template.js WITHOUT the skill.\n");
      } else {
        throw err;
      }
    }
  }

  if (runOnly !== "baseline") {
    try {
      impls.push({ key: "skill", impl: require("./valkey-skill"), label: "valkey-skill (with skill)" });
    } catch (err) {
      if (err.code === "MODULE_NOT_FOUND") {
        console.log("\n  valkey-skill.js not found — skipping.");
        console.log("  Generate it: ask your AI to implement valkey-template.js WITH the skill loaded.\n");
      } else {
        throw err;
      }
    }
  }

  // Randomize execution order to avoid warm-up bias
  if (impls.length > 1 && Math.random() < 0.5) {
    impls.reverse();
  }
  if (impls.length > 1) {
    console.log(`  Run order: ${impls.map(i => i.key).join(" → ")}\n`);
  }

  for (const { key, impl, label } of impls) {
    const results = await runWorkload(impl, label);
    if (key === "baseline") baselineResults = results;
    else skillResults = results;
  }

  if (baselineResults && skillResults) {
    printComparison(baselineResults, skillResults);
  } else if (baselineResults) {
    printSingle("valkey-baseline", baselineResults);
  } else if (skillResults) {
    printSingle("valkey-skill", skillResults);
  } else {
    console.log("\nNo implementations found. See README.md for setup instructions.");
  }
}

main().catch((err) => {
  console.error("Benchmark failed:", err.message);
  process.exit(1);
});
