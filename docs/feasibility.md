# Feasibility gates

**Start here.** Nothing is tested yet. Use disposable data only, in `prototypes/storage-feasibility/`.
Do not ingest personal files until every gate below has passed. Record results and link them here.

## Capacity (measure before committing to the cap)

- [ ] Host free space measured, along with WSL, Docker and object-store allocation
- [x] Pinned object-store release, with volume size and preallocation reviewed ([trial](benchmarks/2026-11-04-seaweedfs-64mib-trial.md))
- [x] Allocated bytes measured after startup, upload, deletion and maintenance, at 1 GiB ([trial](benchmarks/2026-11-04-seaweedfs-1gib-trial.md))
- [ ] Physical free-space floor chosen from those measurements

## Storage engine

- [ ] Volumes, filer metadata, PostgreSQL data and recovery configuration are explicitly persisted
- [x] Credentials configured, internal ports restricted, unauthenticated reads and writes fail ([trial](benchmarks/2026-11-04-seaweedfs-64mib-trial.md))
- [ ] Container runs as non-root, or the exception is justified (the trial image runs as root)
- [ ] Multipart, signing, checksums, range requests and required headers tested against this exact version
- [ ] Oversized upload rejected before exhausting the disk, or the streaming gateway is selected. [Partial evidence](benchmarks/2026-11-04-seaweedfs-oversize-trial.md): small oversized uploads are refused, with temporary disk use unmeasured

## Networking

- [ ] Other machine's browser reaches the API and storage, without disabling firewalls
- [ ] Signed URLs use a hostname the client can reach, with CORS and resumed uploads verified from that client

## Durability

- [ ] A disposable 1 GiB test survives restart, interrupted upload, integrity verification, quota rejection, backup and isolated restore
- [ ] A representative 10 GB test then does the same within measured capacity
- [x] Storage container can be deleted and recreated with data intact ([trial](benchmarks/2026-11-04-seaweedfs-1gib-trial.md)). Repeat once application containers exist

## Decision

If the engine's overhead is unsuitable, keep the adapter and evaluate another local backend before
building further. Update [ADR 0003](adr/0003-seaweedfs-candidate.md) and [ADR 0004](adr/0004-upload-path.md) with the results.
