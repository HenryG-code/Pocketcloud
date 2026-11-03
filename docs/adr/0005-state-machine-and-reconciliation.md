# 0005: Explicit upload states with a reconciler

- **Status:** Accepted
- **Date:** 2026-11-04

## Problem

PostgreSQL and object storage cannot commit atomically together. A crash between them can leave the two disagreeing.

## Options

1. **Hope crashes are rare:** leaves orphaned bytes and missing files with no route to repair.
2. **Distributed transaction across both:** not supported by S3-compatible storage.
3. **Explicit states, idempotent transitions and a background reconciler:** the disagreement is expected and repairable.

## Decision

Option 3. Write bytes first and publish in PostgreSQL last. Every transition is a conditional update from an
expected state. A worker verifies objects and repairs or reclaims anything left in between.

## Consequences

- A crash can waste space temporarily, but cannot show a file whose bytes are missing.
- Each crash point needs a defined recovery and a test. See section 6.4 of the [architecture](../architecture.md).
- Cleanup must recheck references and honour a grace period, since a wrong deletion is the worst failure here.
