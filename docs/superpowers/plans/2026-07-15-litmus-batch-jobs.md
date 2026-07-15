# Litmus Batch Jobs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run independent Litmus batch cases with a bounded worker pool while preserving practical batch-budget controls and ordered results.

**Architecture:** Extend the batch command with a validated `--jobs N` option that defaults to three. The scheduler owns all budget reservations: it reserves a case's maximum allocation before starting a goroutine, subtracts that reservation when the result arrives, and credits the actual provider cost. Workers run `Runner.Probe` with the allocation selected by the scheduler; results are stored by manifest index and persisted only after all started work finishes.

**Tech Stack:** Go standard library (`context`, `sync`); existing Litmus CLI, runner, results, and Makefile.

---

## File Structure

- Modify: `litmus/cmd/litmus-eval/main.go`
  - Parse `--jobs`, implement bounded dispatch and reservation accounting.
- Modify: `litmus/cmd/litmus-eval/main_test.go`
  - Cover parser defaults and validation, concurrency, budget reservation, and stable result order.
- Modify: `README.md`
  - Document the batch worker default, override, and cost semantics.
- Modify: `docs/superpowers/specs/2026-07-15-litmus-eval-design.md`
  - Keep the implementation design accurate about bounded concurrent batches.

### Task 1: Specify Batch Job Parsing

**Files:**
- Modify: `litmus/cmd/litmus-eval/main_test.go`
- Modify: `litmus/cmd/litmus-eval/main.go`

- [x] **Step 1: Write failing parser tests**

```go
func TestParseBatchJobs(t *testing.T) {
	command, err := parseArgs([]string{
		"batch", "full", "--budget", "1.50", "--jobs", "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Jobs != 2 {
		t.Fatalf("jobs = %d, want 2", command.Jobs)
	}
}

func TestParseBatchDefaultsToThreeJobs(t *testing.T) {
	command, err := parseArgs([]string{"batch", "core", "--budget", "0.80"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Jobs != 3 {
		t.Fatalf("jobs = %d, want 3", command.Jobs)
	}
}

func TestParseBatchRejectsInvalidJobs(t *testing.T) {
	_, err := parseArgs([]string{"batch", "core", "--budget", "0.80", "--jobs", "0"})
	if err == nil || !strings.Contains(err.Error(), "jobs") {
		t.Fatalf("parseArgs() error = %v, want jobs validation error", err)
	}
}
```

- [x] **Step 2: Run the parser tests to verify they fail**

Run:

```sh
GOPROXY=direct go test ./litmus/cmd/litmus-eval -run 'TestParseBatch(Jobs|DefaultsToThreeJobs|RejectsInvalidJobs)$'
```

Expected: FAIL because `command` has no `Jobs` field and the batch parser accepts no `--jobs` option.

- [x] **Step 3: Implement minimal job parsing**

```go
const defaultBatchJobs = 3

type command struct {
	// Existing fields.
	Jobs int
}

func parseJobs(value string) (int, error) {
	jobs, err := strconv.Atoi(value)
	if err != nil || jobs <= 0 {
		return 0, fmt.Errorf("jobs must be a positive integer")
	}
	return jobs, nil
}
```

Accept `--include-replay-only` and `--jobs N` in either order, reject duplicate or unknown batch flags, and set `Jobs: defaultBatchJobs` when omitted.

- [x] **Step 4: Run parser tests to verify they pass**

Run:

```sh
GOPROXY=direct go test ./litmus/cmd/litmus-eval -run 'TestParseBatch(Jobs|DefaultsToThreeJobs|RejectsInvalidJobs)$'
```

Expected: PASS.

### Task 2: Add Reservation-Based Bounded Dispatch

**Files:**
- Modify: `litmus/cmd/litmus-eval/main_test.go`
- Modify: `litmus/cmd/litmus-eval/main.go`

- [x] **Step 1: Write the failing bounded-concurrency test**

Add an executor test double whose `Execute` method increments an `active` counter under a mutex, records the observed peak, waits on a channel, then returns a passing `$0.04` response. Execute a four-case batch with `--jobs 2`; wait until the peak is two, release the executor, and assert the persisted run has four results in manifest order.

- [x] **Step 2: Run the bounded-concurrency test to verify it fails**

Run:

```sh
GOPROXY=direct go test ./litmus/cmd/litmus-eval -run TestRunBatchLimitsConcurrentProbes$
```

Expected: FAIL because the serial batch never reaches two concurrent executor calls.

- [x] **Step 3: Implement the scheduler**

Replace the serial batch loop with a scheduler that:

```go
cases := make([]litmus.Case, len(manifest.Cases))
for index, item := range manifest.Cases {
	testCase, err := litmus.LoadCase(app.root, item.Agent, item.CaseID)
	if err != nil {
		return reportBatchLoadError(item, err)
	}
	if parsed.IncludeReplayOnly {
		testCase.Live = true
	}
	cases[index] = testCase
}

for next < len(manifest.Cases) || active > 0 {
	for active < parsed.Jobs && next < len(manifest.Cases) {
		testCase := cases[next]
		reserved := effectiveReservation(testCase.MaxBudgetUSD, parsed.BudgetUSD, spent, reservedTotal)
		if reserved == 0 {
			next = len(manifest.Cases)
			break
		}
		reservedTotal += reserved
		allocatedCase := testCase
		allocatedCase.MaxBudgetUSD = reserved
		startWorker(index, allocatedCase, reserved)
		next++
		active++
	}

	completed := <-done
	reservedTotal -= completed.reserved
	spent += completed.result.CostUSD
	results[completed.index] = completed.result
	active--
}
```

Call `Runner.Probe` with the allocated case and `runBudget == reserved`, `spent == 0`, so its provider target cannot exceed the scheduler reservation. Preserve existing provider-error handling and write one run after all active workers complete.

- [x] **Step 4: Run the bounded-concurrency test to verify it passes**

Run:

```sh
GOPROXY=direct go test ./litmus/cmd/litmus-eval -run TestRunBatchLimitsConcurrentProbes$
```

Expected: PASS with a peak of two active calls and results in manifest order.

### Task 3: Prove Capped Batches Do Not Overschedule

**Files:**
- Modify: `litmus/cmd/litmus-eval/main_test.go`
- Modify: `litmus/cmd/litmus-eval/main.go`

- [x] **Step 1: Write the failing reservation test**

Create three `$0.10` cases, execute the manifest with `--budget 0.10 --jobs 3`, and use an executor that returns a passing `$0.10` result. Assert that exactly one provider call and one result are persisted.

- [x] **Step 2: Run the reservation test to verify it fails**

Run:

```sh
GOPROXY=direct go test ./litmus/cmd/litmus-eval -run TestRunBatchReservesBudgetBeforeDispatch$
```

Expected: FAIL because all ready workers observe the same unreserved budget.

- [x] **Step 3: Implement scheduler reservation accounting**

Use a scheduler-local `reservedTotal` in all admission checks:

```go
remaining := parsed.BudgetUSD - spent - reservedTotal
reserved := minCents(testCase.MaxBudgetUSD, remaining)
if reserved <= 0 {
	break
}
```

On worker completion, remove its reservation before adding the actual recorded provider cost. Do not start a replacement after a case whose actual cost exhausts the batch budget.

- [x] **Step 4: Run the reservation test to verify it passes**

Run:

```sh
GOPROXY=direct go test ./litmus/cmd/litmus-eval -run TestRunBatchReservesBudgetBeforeDispatch$
```

Expected: PASS with one provider call.

### Task 4: Document and Verify

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-07-15-litmus-eval-design.md`

- [x] **Step 1: Update the batch examples and cost notes**

Document:

```sh
make litmus-batch MANIFEST=core BUDGET=0.80
make litmus-batch MANIFEST=core BUDGET=0.80 BATCH_ARGS='--jobs 1'
```

State that batches default to three concurrent probes, reservations constrain scheduling rather than provider billing, and Claude's per-call budget remains advisory.

- [x] **Step 2: Run formatting and the focused test suites**

Run:

```sh
gofmt -w litmus/cmd/litmus-eval/main.go litmus/cmd/litmus-eval/main_test.go
GOPROXY=direct go test ./litmus/cmd/litmus-eval ./litmus/internal/litmus
```

Expected: both packages PASS.

- [x] **Step 3: Inspect the final diff**

Run:

```sh
git diff --check
git diff -- litmus/cmd/litmus-eval/main.go litmus/cmd/litmus-eval/main_test.go README.md docs/superpowers/specs/2026-07-15-litmus-eval-design.md
```

Expected: no whitespace errors; only the intended scheduler, parser, tests, and documentation changes.

- [x] **Step 4: Do not commit**

The user requires explicit approval before every commit. Leave the verified changes uncommitted.
