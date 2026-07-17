# GLIDE Performance Skill — Benchmark

Measures whether code generated with the GLIDE Performance skill is faster than code generated without it, using a ride-sharing platform backend as the workload. For this benchmark, we will be using the Node implementation.

## How It Works

1. `valkey-template.js` defines a ride-sharing Valkey layer as empty stubs — driver profiles, trip management, rider sessions, surge pricing, dispatch queues, vehicle availability, feature flags, ETA caching, rate limiting, driver zones, and an operations dashboard.
2. You ask your AI to implement the template **twice**:
   - **Without** the skill loaded → save as `valkey-baseline.js`
   - **With** the skill loaded → save as `valkey-skill.js`
3. `app-benchmark.js` runs the same workload against both and compares latency.

The delta between the two is the measurable impact of the skill.

## Directory Structure

```
benchmarks/
├── README.md                    # This file
├── app-benchmark.js             # Benchmark runner
├── valkey-template.js           # Stubs — the contract both implementations must satisfy
├── valkey-baseline.js           # (AI-generated without skill — you create this)
├── valkey-skill.js              # (AI-generated with skill — you create this)
├── sample-valkey-baseline.js    # Reference baseline implementation
├── sample-valkey-skill.js       # Reference skill-guided implementation
└── sample-results.txt           # Expected benchmark output
```

## Setup

```bash
# Start Valkey
valkey-server # OR
docker run -d --name valkey -p 6379:6379 valkey/valkey:latest

# Install GLIDE
npm install @valkey/valkey-glide
```

## Generating Implementations

### Step 1: Baseline implementation (skill NOT loaded)

Make sure the GLIDE Performance skill is **not** active, then prompt your AI:

```
Implement every function in valkey-template.js using @valkey/valkey-glide.
The Valkey server is at localhost:6379. Save the result as valkey-baseline.js.
Do not change the function signatures or exports.
```

### Step 2: Skill-guided implementation (skill loaded)

Load the GLIDE Performance skill, then prompt your AI:

```
Implement every function in valkey-template.js using @valkey/valkey-glide and the GLIDE performance skill.
Do not read valkey-baseline.js.
The Valkey server is at localhost:6379. Save the result as valkey-skill.js.
Do not change the function signatures or exports.
```

## Running

```bash
# Compare both implementations
node app-benchmark.js

# Run only one
node app-benchmark.js --only baseline
node app-benchmark.js --only skill

# More iterations for stable numbers
node app-benchmark.js --iterations 500
```

