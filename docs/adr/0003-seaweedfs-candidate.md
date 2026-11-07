# 0003: SeaweedFS as the candidate storage engine

- **Status:** Proposed. Depends on the [feasibility gates](../feasibility.md)
- **Date:** 2026-11-04

## Problem

The object store must be free, self-hosted and S3-compatible, and fit a small disk budget on one machine.

## Options

1. **SeaweedFS:** open source under Apache-2.0 with an S3 API. Its default volume allocation needs tuning for a small disk.
2. **Another S3-compatible engine:** evaluate if SeaweedFS fails its gates.
3. **Write a storage engine:** rejected, because blob durability is not what this project sets out to teach.

## Decision

Evaluate SeaweedFS first with disposable data. Accept only if a pinned release passes allocation, private access,
multipart, restart and restore tests within the measured budget.

## Evidence so far

A [64 MiB trial](../benchmarks/2026-11-04-seaweedfs-64mib-trial.md) found low overhead and private access working, but deleted space is only freed after vacuum. A [1 GiB trial](../benchmarks/2026-11-04-seaweedfs-1gib-trial.md) confirmed this, with about 0.03% overhead for one large file. An [oversize trial](../benchmarks/2026-11-04-seaweedfs-oversize-trial.md) found size limits enforced on signed uploads. A [resume trial](../benchmarks/2026-11-04-seaweedfs-resume-trial.md) found resume and crash recovery working at 64 MiB, with quirks the API must handle. A [restore trial](../benchmarks/2026-11-04-seaweedfs-restore-trial.md) brought back all 1,005 files exactly from an archive after the original was destroyed, and detected deliberate damage in one trial. A [10 GiB trial](../benchmarks/2026-11-04-seaweedfs-10gib-trial.md) repeated that restore with 2,165 files, running as non-root, with no wrong bytes. An earlier claim that the image runs as root was a measurement mistake. The decision stays open until browser access is tested and restore includes PostgreSQL.

## Consequences

- The adapter stays replaceable until this record is accepted.
- Failing the gates is a cheap, early change of engine rather than a late migration.
- Measured results will be linked here when the status changes.
