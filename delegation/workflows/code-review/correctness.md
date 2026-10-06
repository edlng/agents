## Lens: correctness

Look for logic errors, off-by-one errors, unhandled edge cases (empty, zero,
negative, nil, case and Unicode handling), and broken invariants. Name the
concrete input that triggers each bug and what the code returns for it.
Security defects belong to the security lens; skip them here.

When the change has shared mutable state, threads, timers, futures, or
callbacks, work through each of these before submitting:
- Check-then-act: every decision made under a lock or after a check, and
  whether another thread (including close or shutdown) can change the state
  before the action that depends on it. Name the interleaving.
- Completion: every future, promise, or in-flight marker, and whether every
  exit path completes or clears it, including non-RuntimeException errors
  (Java Error, Python BaseException) and cancellation. A marker left set
  makes later callers hang or fail forever.
- Freshness: every validity or expiry check, and whether time can pass
  between the check and the use, so a caller receives a stale value.
- Scheduling: whether a refresh, retry, or timer can be cancelled, replaced
  by a shorter delay, or skipped so that no future refresh is scheduled.
- Callbacks: where user callbacks run (a shared or single-thread executor
  blocks other work), whether they run before shared state is consistent,
  and whether they can re-enter the object.
- Consistency: whether concurrent callers waiting on the same operation see
  the same result and the same error type as the caller that started it.
