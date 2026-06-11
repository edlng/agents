# Shared: Async Dispatch Protocols

> Shared reference for agents that delegate work asynchronously (assign/fire-and-forget) and receive results via inbox delivery.

## Idle-Based Delivery

Async worker results are delivered to the supervisor terminal **only when it becomes idle** (finishes its turn). This creates a critical anti-pattern:

**DO NOT** keep the terminal busy while waiting for results:
- No `sleep` or `wait` loops
- No placeholder `echo` commands
- No polling scripts

**DO** finish the turn after dispatching work. State what was dispatched and what you expect. Results arrive as your next input automatically.

## Dispatch Pattern

1. Get your terminal/session identifier.
2. Dispatch all async tasks in sequence (they run in parallel once dispatched).
3. Include the callback ID in every task message so workers know where to send results.
4. End your turn - do not run further commands.

## Callback Instructions

When assigning async work, always include explicit callback instructions in the task message:

```
[task description]. When complete, send results to terminal [ID] using send_message.
```

Do not rely on workers guessing where to send results. Be explicit.

## Receiving Results

When results arrive (as new messages on your next turn):
- Verify all expected results have arrived.
- If some are missing, wait another turn - do not re-dispatch (duplicates work).
- Once all results are in, synthesize and continue.

## Mixed Sync/Async

When combining blocking (handoff) and non-blocking (assign) in one workflow:
1. Dispatch all async tasks first (they start running).
2. Then run the blocking handoff (uses the wait time productively).
3. After handoff returns, finish the turn to receive async results.

## When NOT to Use Async

Use synchronous/blocking dispatch when:
- The next step depends on the result (sequential dependency).
- You need to iterate on the result before proceeding.
- The task is short enough that async overhead is wasteful.
