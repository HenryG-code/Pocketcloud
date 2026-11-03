# 0002: PostgreSQL for metadata, object storage for bytes

- **Status:** Accepted
- **Date:** 2026-11-04

## Problem

Files need both transactional rules (ownership, quota, state) and large-byte storage. Where should each live?

## Options

1. **All in PostgreSQL:** one transaction covers everything, but large binaries bloat the database and its backups.
2. **Plain filesystem plus PostgreSQL:** simple to inspect, but multipart, integrity and portability become custom work.
3. **S3-compatible object storage behind an adapter, plus PostgreSQL:** purpose-built for each job, and teaches an industry-standard API.

## Decision

Option 3. Business code depends on a small adapter interface. Object keys are random and never derived from filenames.

## Consequences

- No transaction covers both stores, which forces [ADR 0005](0005-state-machine-and-reconciliation.md).
- Backups must capture both stores consistently.
- Renames change metadata only.
- S3 compatibility is not universal, so behaviour is tested against the exact backend rather than assumed.
