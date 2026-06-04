/**
 * Valkey GLIDE — Implementation with GLIDE Performance Optimization Skill
 *
 * THIS FILE WAS GENERATED STRAIGHT FROM THE AUTO MODEL IN THE KIRO IDE. IT HAS
 * NOT BEEN MODIFIED FOR CORRECTNESS OR PERFORMANCE.
 * 
 * A ride-sharing / delivery platform Valkey layer that exercises every major
 * pattern the GLIDE Performance Optimization skill covers.
 */

const { GlideClient, Batch } = require("@valkey/valkey-glide");

// ============================================================================
// Module-level client instances (reuse pattern)
// ============================================================================

let client = null;
let blockingClient = null;

// ============================================================================
// Lifecycle
// ============================================================================

/**
 * Called once before any benchmark runs.
 * Set up clients, connections, or any shared state.
 */
async function initialize() {
  // Create main client with production-ready configuration
  client = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
    requestTimeout: 500,
    clientName: "valkey-skill-main",
    connectionBackoff: {
      numberOfRetries: 10,
      factor: 500,
      exponentBase: 2,
    },
  });

  // Clean slate for benchmarks
  await client.flushall();

  // Dedicated client for blocking operations
  blockingClient = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
    requestTimeout: 30000,
    clientName: "valkey-skill-blocking",
    connectionBackoff: {
      numberOfRetries: 10,
      factor: 500,
      exponentBase: 2,
    },
  });
}

/**
 * Called once after all benchmarks complete.
 * Close connections and clean up.
 */
async function shutdown() {
  if (client) {
    await client.close();
  }
  if (blockingClient) {
    await blockingClient.close();
  }
}

// ============================================================================
// 1. Driver Profiles — Hash vs JSON, single-field updates, bulk reads
// ============================================================================

/**
 * Store a driver profile with fields: name, phone, vehicleType, licensePlate,
 * rating, totalTrips, currentZone.
 */
async function setDriverProfile(driverId, profile) {
  await client.hset(`driver:${driverId}`, profile);
}

/**
 * Fetch a full driver profile (all fields).
 */
async function getDriverProfile(driverId) {
  const result = await client.hgetall(`driver:${driverId}`);
  // Convert array of {field, value} to object
  if (!result || result.length === 0) return null;
  const obj = {};
  for (const item of result) {
    obj[item.field] = item.value;
  }
  return obj;
}

/**
 * Update a single field on a driver profile (e.g. currentZone after relocation).
 */
async function updateDriverField(driverId, field, value) {
  await client.hset(`driver:${driverId}`, { [field]: value });
}

/**
 * Fetch name and rating for multiple drivers (e.g. showing nearby drivers to rider).
 */
async function getMultipleDriverSummaries(driverIds) {
  const batch = new Batch(false);
  
  for (const driverId of driverIds) {
    batch.hmget(`driver:${driverId}`, ["name", "rating"]);
  }
  
  const results = await client.exec(batch, true);
  return results.map(([name, rating]) => ({ name, rating }));
}

// ============================================================================
// 2. Trip Management — Bulk reads/writes, batching, pipeline, chunking
// ============================================================================

/**
 * Fetch trip data for multiple trip IDs.
 */
async function getTrips(tripIds) {
  return await client.mget(tripIds.map(id => `trip:${id}`));
}

/**
 * Write trip data for multiple trips in one operation.
 */
async function setTrips(trips) {
  const keyValuePairs = {};
  for (const trip of trips) {
    keyValuePairs[`trip:${trip.tripId}`] = trip.data;
  }
  await client.mset(keyValuePairs);
}

/**
 * Archive a large batch of completed trips. Input may contain 500+ items.
 * Must handle large batches appropriately.
 */
async function archiveTripsBatch(trips) {
  const batchSize = 100;
  
  for (let i = 0; i < trips.length; i += batchSize) {
    const batch = new Batch(false);
    const chunk = trips.slice(i, i + batchSize);
    
    for (const trip of chunk) {
      batch.set(`archive:trip:${trip.tripId}`, trip.data);
    }
    
    await client.exec(batch, true);
  }
}

// ============================================================================
// 3. Rider Sessions — TTL handling, SET with EX, JSON vs Hash
// ============================================================================

/**
 * Save a rider session with a 1-hour TTL.
 */
async function saveRiderSession(sessionId, sessionData) {
  const batch = new Batch(false);
  
  // Use Hash for structured data with TTL
  batch.hset(`session:${sessionId}`, sessionData);
  // Add jitter to TTL (3600 ± 10%)
  const ttl = 3600 + Math.floor(Math.random() * 720 - 360);
  batch.expire(`session:${sessionId}`, ttl);
  
  await client.exec(batch, true);
}

/**
 * Read a rider session.
 */
async function getRiderSession(sessionId) {
  const result = await client.hgetall(`session:${sessionId}`);
  if (!result || result.length === 0) return null;
  const obj = {};
  for (const item of result) {
    obj[item.field] = item.value;
  }
  return obj;
}

/**
 * Update the rider's location in their session and refresh the TTL.
 */
async function updateRiderLocation(sessionId, location) {
  const batch = new Batch(false);
  
  batch.hset(`session:${sessionId}`, { currentLocation: location });
  const ttl = 3600 + Math.floor(Math.random() * 720 - 360);
  batch.expire(`session:${sessionId}`, ttl);
  
  await client.exec(batch, true);
}

// ============================================================================
// 4. Surge Pricing & Analytics — Counters, atomic increments, sorted sets
// ============================================================================

/**
 * Record a ride request in a zone: increment the request counter and
 * store the timestamp of the latest request.
 */
async function recordRideRequest(zoneId, timestamp) {
  const batch = new Batch(false);
  
  batch.hincrBy(`zone:${zoneId}`, "requestCount", 1);
  batch.hset(`zone:${zoneId}`, { lastRequest: timestamp.toString() });
  
  await client.exec(batch, true);
}

/**
 * Get ride request stats for multiple zones.
 */
async function getZoneStats(zoneIds) {
  const batch = new Batch(false);
  
  for (const zoneId of zoneIds) {
    batch.hmget(`zone:${zoneId}`, ["requestCount", "lastRequest"]);
  }
  
  const results = await client.exec(batch, true);
  return results.map(([requestCount, lastRequest]) => ({ requestCount, lastRequest }));
}

/**
 * Submit driver earnings to a leaderboard (sorted set).
 */
async function submitEarnings(leaderboardKey, entries) {
  const batchSize = 100;
  
  for (let i = 0; i < entries.length; i += batchSize) {
    const batch = new Batch(false);
    const chunk = entries.slice(i, i + batchSize);
    
    for (const entry of chunk) {
      batch.zadd(leaderboardKey, [{ element: entry.driverId, score: entry.earnings }]);
    }
    
    await client.exec(batch, true);
  }
}

/**
 * Get the top N earners from a leaderboard.
 */
async function getTopEarners(leaderboardKey, topN) {
  const results = await client.zrangeWithScores(leaderboardKey, { start: 0, end: topN - 1 }, { reverse: true });
  return results.map(item => ({ driverId: item.element, earnings: item.score }));
}

// ============================================================================
// 5. Dispatch Queue — BLPOP, dedicated client, list ops
// ============================================================================

/**
 * Push multiple ride requests onto the dispatch queue.
 */
async function enqueueRideRequests(queueKey, rideRequests) {
  await client.rpush(queueKey, rideRequests);
}

/**
 * Pop the next ride request from the dispatch queue using a blocking pop.
 */
async function dequeueRideRequest(queueKey, timeoutSeconds) {
  const result = await blockingClient.blpop([queueKey], timeoutSeconds);
  return result ? result[1] : null;
}

// ============================================================================
// 6. Vehicle Availability — Atomic operations, transactions
// ============================================================================

/**
 * Atomically claim a vehicle from a zone's available pool.
 */
async function claimVehicle(zoneId, count) {
  const key = `zone:${zoneId}:available`;
  
  // Get current availability
  const availableStr = await client.get(key);
  const available = parseInt(availableStr || "0", 10);
  
  if (available < count) {
    return false;
  }
  
  // Decrement if sufficient
  await client.decrBy(key, count);
  return true;
}

/**
 * Set initial vehicle availability for multiple zones.
 */
async function setZoneAvailability(zones) {
  const batchSize = 100;
  
  for (let i = 0; i < zones.length; i += batchSize) {
    const batch = new Batch(false);
    const chunk = zones.slice(i, i + batchSize);
    
    for (const zone of chunk) {
      batch.set(`zone:${zone.zoneId}:available`, zone.available.toString());
    }
    
    await client.exec(batch, true);
  }
}

/**
 * Read vehicle availability for multiple zones.
 */
async function getZoneAvailability(zoneIds) {
  const batch = new Batch(false);
  
  for (const zoneId of zoneIds) {
    batch.get(`zone:${zoneId}:available`);
  }
  
  const results = await client.exec(batch, true);
  return zoneIds.map((zoneId, index) => ({ zoneId, available: results[index] }));
}

// ============================================================================
// 7. Feature Flags — A/B testing ride algorithms, bulk flag resolution
// ============================================================================

/**
 * Set a feature flag with a TTL.
 */
async function setFeatureFlag(flagName, value, ttlSeconds) {
  await client.set(`flag:${flagName}`, value, { expiry: { type: "EX", count: ttlSeconds } });
}

/**
 * Resolve multiple feature flags at once.
 */
async function resolveFeatureFlags(flagNames) {
  const keys = flagNames.map(name => `flag:${name}`);
  const values = await client.mget(keys);
  
  const result = {};
  flagNames.forEach((name, index) => {
    result[name] = values[index];
  });
  
  return result;
}

// ============================================================================
// 8. ETA Cache — Multi-field Hash operations, partial updates
// ============================================================================

/**
 * Cache an ETA estimate for a trip.
 */
async function updateEtaField(tripId, field, value) {
  await client.hset(`eta:${tripId}`, { [field]: value });
}

/**
 * Get the full cached ETA for a trip.
 */
async function getEta(tripId) {
  const result = await client.hgetall(`eta:${tripId}`);
  if (!result || result.length === 0) return {};
  const obj = {};
  for (const item of result) {
    obj[item.field] = item.value;
  }
  return obj;
}

/**
 * Remove a cached ETA field.
 */
async function clearEtaField(tripId, field) {
  await client.hdel(`eta:${tripId}`, [field]);
}

// ============================================================================
// 9. Rate Limiting — Anti-abuse for ride requests
// ============================================================================

/**
 * Check and increment a rate limit counter for a rider.
 */
async function incrementRateLimit(riderId, windowSeconds) {
  const key = `ratelimit:${riderId}`;
  
  const batch = new Batch(false);
  batch.incr(key);
  batch.expire(key, windowSeconds);
  
  const results = await client.exec(batch, true);
  return results[0];
}

// ============================================================================
// 10. Driver Zones — Set operations, membership checks
// ============================================================================

/**
 * Register multiple drivers as available in a zone.
 */
async function addDriversToZone(zoneId, driverIds) {
  await client.sadd(`zone:${zoneId}:drivers`, driverIds);
}

/**
 * Check which of the given drivers are currently in a zone.
 */
async function checkDriversInZone(zoneId, driverIds) {
  const batch = new Batch(false);
  
  for (const driverId of driverIds) {
    batch.sismember(`zone:${zoneId}:drivers`, driverId);
  }
  
  const results = await client.exec(batch, true);
  const result = {};
  driverIds.forEach((driverId, index) => {
    result[driverId] = results[index] === true;
  });
  
  return result;
}

// ============================================================================
// 11. Operations Dashboard — Concurrent independent reads
// ============================================================================

/**
 * Fetch a real-time operations dashboard requiring multiple independent reads.
 */
async function getOperationsDashboard() {
  const [activeDrivers, activeTrips, totalRidesToday, avgWaitTime, recentCompletions] = await Promise.all([
    client.get("ops:activeDrivers"),
    client.get("ops:activeTrips"),
    client.get("ops:totalRidesToday"),
    client.get("ops:avgWaitTime"),
    client.lrange("ops:recentCompletions", 0, 4),
  ]);
  
  return {
    activeDrivers,
    activeTrips,
    totalRidesToday,
    avgWaitTime,
    recentCompletions,
  };
}

// ============================================================================
// Exports
// ============================================================================

module.exports = {
  initialize,
  shutdown,
  // Driver profiles
  setDriverProfile,
  getDriverProfile,
  updateDriverField,
  getMultipleDriverSummaries,
  // Trips
  getTrips,
  setTrips,
  archiveTripsBatch,
  // Rider sessions
  saveRiderSession,
  getRiderSession,
  updateRiderLocation,
  // Surge pricing & analytics
  recordRideRequest,
  getZoneStats,
  submitEarnings,
  getTopEarners,
  // Dispatch queue
  enqueueRideRequests,
  dequeueRideRequest,
  // Vehicle availability
  claimVehicle,
  setZoneAvailability,
  getZoneAvailability,
  // Feature flags
  setFeatureFlag,
  resolveFeatureFlags,
  // ETA cache
  updateEtaField,
  getEta,
  clearEtaField,
  // Rate limiting
  incrementRateLimit,
  // Driver zones
  addDriversToZone,
  checkDriversInZone,
  // Dashboard
  getOperationsDashboard,
};
