/**
 * Valkey GLIDE — Baseline Implementation
 * 
 * THIS FILE WAS GENERATED STRAIGHT FROM THE AUTO MODEL IN THE KIRO IDE. IT HAS
 * NOT BEEN MODIFIED FOR CORRECTNESS OR PERFORMANCE.
 *
 * A straightforward implementation using @valkey/valkey-glide.
 * This serves as a baseline for performance comparison.
 */

const { GlideClient } = require("@valkey/valkey-glide");

let client;

// ============================================================================
// Lifecycle
// ============================================================================

async function initialize() {
  client = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
  });

  // Clean slate for benchmarks
  await client.flushall();
}

async function shutdown() {
  if (client) {
    client.close();
  }
}

// ============================================================================
// 1. Driver Profiles — Hash vs JSON, single-field updates, bulk reads
// ============================================================================

async function setDriverProfile(driverId, profile) {
  const key = `driver:${driverId}`;
  await client.hset(key, profile);
}

async function getDriverProfile(driverId) {
  const key = `driver:${driverId}`;
  return await client.hgetall(key);
}

async function updateDriverField(driverId, field, value) {
  const key = `driver:${driverId}`;
  await client.hset(key, { [field]: value });
}

async function getMultipleDriverSummaries(driverIds) {
  const results = [];
  for (const driverId of driverIds) {
    const key = `driver:${driverId}`;
    const name = await client.hget(key, "name");
    const rating = await client.hget(key, "rating");
    results.push({ name, rating });
  }
  return results;
}

// ============================================================================
// 2. Trip Management — Bulk reads/writes, batching, pipeline, chunking
// ============================================================================

async function getTrips(tripIds) {
  const results = [];
  for (const tripId of tripIds) {
    const data = await client.get(tripId);
    results.push(data);
  }
  return results;
}

async function setTrips(trips) {
  for (const trip of trips) {
    await client.set(trip.tripId, trip.data);
  }
}

async function archiveTripsBatch(trips) {
  for (const trip of trips) {
    await client.set(trip.tripId, trip.data);
  }
}

// ============================================================================
// 3. Rider Sessions — TTL handling, SET with EX, JSON vs Hash
// ============================================================================

async function saveRiderSession(sessionId, sessionData) {
  const key = `session:${sessionId}`;
  const jsonData = JSON.stringify(sessionData);
  await client.set(key, jsonData);
  await client.expire(key, 3600);
}

async function getRiderSession(sessionId) {
  const key = `session:${sessionId}`;
  const data = await client.get(key);
  return data ? JSON.parse(data) : null;
}

async function updateRiderLocation(sessionId, location) {
  const key = `session:${sessionId}`;
  const data = await client.get(key);
  if (data) {
    const session = JSON.parse(data);
    session.currentLocation = location;
    await client.set(key, JSON.stringify(session));
    await client.expire(key, 3600);
  }
}

// ============================================================================
// 4. Surge Pricing & Analytics — Counters, atomic increments, sorted sets
// ============================================================================

async function recordRideRequest(zoneId, timestamp) {
  const countKey = `zone:${zoneId}:requests`;
  const timeKey = `zone:${zoneId}:lastRequest`;
  await client.incr(countKey);
  await client.set(timeKey, timestamp.toString());
}

async function getZoneStats(zoneIds) {
  const results = [];
  for (const zoneId of zoneIds) {
    const countKey = `zone:${zoneId}:requests`;
    const timeKey = `zone:${zoneId}:lastRequest`;
    const requestCount = await client.get(countKey);
    const lastRequest = await client.get(timeKey);
    results.push({ requestCount, lastRequest });
  }
  return results;
}

async function submitEarnings(leaderboardKey, entries) {
  for (const entry of entries) {
    await client.zadd(leaderboardKey, { [entry.driverId]: entry.earnings });
  }
}

async function getTopEarners(leaderboardKey, topN) {
  const results = await client.zrangeWithScores(leaderboardKey, { start: 0, end: topN - 1 }, { reverse: true });
  return results.map(item => ({
    driverId: item.element,
    earnings: item.score
  }));
}

// ============================================================================
// 5. Dispatch Queue — BLPOP, dedicated client, list ops
// ============================================================================

async function enqueueRideRequests(queueKey, rideRequests) {
  for (const request of rideRequests) {
    await client.rpush(queueKey, [request]);
  }
}

async function dequeueRideRequest(queueKey, timeoutSeconds) {
  const result = await client.blpop([queueKey], timeoutSeconds);
  return result ? result.value : null;
}

// ============================================================================
// 6. Vehicle Availability — Atomic operations, transactions
// ============================================================================

async function claimVehicle(zoneId, count) {
  const key = `zone:${zoneId}:available`;
  const current = await client.get(key);
  const available = current ? parseInt(current, 10) : 0;
  
  if (available >= count) {
    await client.decrBy(key, count);
    return true;
  }
  return false;
}

async function setZoneAvailability(zones) {
  for (const zone of zones) {
    const key = `zone:${zone.zoneId}:available`;
    await client.set(key, zone.available.toString());
  }
}

async function getZoneAvailability(zoneIds) {
  const results = [];
  for (const zoneId of zoneIds) {
    const key = `zone:${zoneId}:available`;
    const available = await client.get(key);
    results.push({ zoneId, available });
  }
  return results;
}

// ============================================================================
// 7. Feature Flags — A/B testing ride algorithms, bulk flag resolution
// ============================================================================

async function setFeatureFlag(flagName, value, ttlSeconds) {
  const key = `flag:${flagName}`;
  await client.set(key, value);
  await client.expire(key, ttlSeconds);
}

async function resolveFeatureFlags(flagNames) {
  const results = {};
  for (const flagName of flagNames) {
    const key = `flag:${flagName}`;
    results[flagName] = await client.get(key);
  }
  return results;
}

// ============================================================================
// 8. ETA Cache — Multi-field Hash operations, partial updates
// ============================================================================

async function updateEtaField(tripId, field, value) {
  const key = `eta:${tripId}`;
  await client.hset(key, { [field]: value });
}

async function getEta(tripId) {
  const key = `eta:${tripId}`;
  return await client.hgetall(key);
}

async function clearEtaField(tripId, field) {
  const key = `eta:${tripId}`;
  await client.hdel(key, [field]);
}

// ============================================================================
// 9. Rate Limiting — Anti-abuse for ride requests
// ============================================================================

async function incrementRateLimit(riderId, windowSeconds) {
  const key = `ratelimit:${riderId}`;
  const count = await client.incr(key);
  if (count === 1) {
    await client.expire(key, windowSeconds);
  }
  return count;
}

// ============================================================================
// 10. Driver Zones — Set operations, membership checks
// ============================================================================

async function addDriversToZone(zoneId, driverIds) {
  const key = `zone:${zoneId}:drivers`;
  for (const driverId of driverIds) {
    await client.sadd(key, [driverId]);
  }
}

async function checkDriversInZone(zoneId, driverIds) {
  const key = `zone:${zoneId}:drivers`;
  const results = {};
  for (const driverId of driverIds) {
    const isMember = await client.sismember(key, driverId);
    results[driverId] = isMember;
  }
  return results;
}

// ============================================================================
// 11. Operations Dashboard — Concurrent independent reads
// ============================================================================

async function getOperationsDashboard() {
  const activeDrivers = await client.get("ops:activeDrivers");
  const activeTrips = await client.get("ops:activeTrips");
  const totalRidesToday = await client.get("ops:totalRidesToday");
  const avgWaitTime = await client.get("ops:avgWaitTime");
  const recentCompletions = await client.lrange("ops:recentCompletions", 0, 4);
  
  return {
    activeDrivers,
    activeTrips,
    totalRidesToday,
    avgWaitTime,
    recentCompletions
  };
}

// ============================================================================
// Exports
// ============================================================================

module.exports = {
  initialize,
  shutdown,
  setDriverProfile,
  getDriverProfile,
  updateDriverField,
  getMultipleDriverSummaries,
  getTrips,
  setTrips,
  archiveTripsBatch,
  saveRiderSession,
  getRiderSession,
  updateRiderLocation,
  recordRideRequest,
  getZoneStats,
  submitEarnings,
  getTopEarners,
  enqueueRideRequests,
  dequeueRideRequest,
  claimVehicle,
  setZoneAvailability,
  getZoneAvailability,
  setFeatureFlag,
  resolveFeatureFlags,
  updateEtaField,
  getEta,
  clearEtaField,
  incrementRateLimit,
  addDriversToZone,
  checkDriversInZone,
  getOperationsDashboard,
};
