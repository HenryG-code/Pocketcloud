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

## Evidence so far

A [resume trial](../benchmarks/2026-11-04-seaweedfs-resume-trial.md) found that storage returns 404 for a repeated completion and can list a retried part twice. This supports keeping the authoritative record in PostgreSQL.

## Consequences

- A crash can waste space temporarily, but cannot show a file whose bytes are missing.
- Each crash point needs a defined recovery and a test. See section 6.4 of the [architecture](../architecture.md).
- Cleanup must recheck references and honour a grace period, since a wrong deletion is the worst failure here.
