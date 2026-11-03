# 0001: One API and one worker, not microservices

- **Status:** Accepted
- **Date:** 2026-11-04

## Problem

The system has several concerns: identity, uploads, files, capacity and cleanup. How many separately deployed services should it have?

## Options

1. **One service per concern:** clear ownership, but network failures and distributed transactions from day one.
2. **One program for everything:** simplest, but long verification work competes with interactive requests.
3. **One API plus one worker, with strict internal modules:** one codebase and database, with background work separated.

## Decision

Option 3. One person owns everything, and no concern needs independent scaling yet. Long-running verification
and cleanup are the only work with a different resource profile, so they alone run separately.

## Consequences

- Upload and quota changes can share one database transaction, which is far simpler to get right.
- Module boundaries depend on discipline, since nothing physical enforces them.
- API and worker share a failure domain on one host, which is acceptable here.
- Revisit if a module needs independent scaling or separate ownership.
