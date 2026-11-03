# Feasibility gates

**Start here.** Nothing is tested yet. Use disposable data only, in `prototypes/storage-feasibility/`.
Do not ingest personal files until every gate below has passed. Record results and link them here.

## Capacity (measure before committing to the cap)

- [ ] Host free space measured, along with WSL, Docker and object-store allocation
- [ ] Pinned object-store release, with volume size and preallocation reviewed
- [ ] Allocated bytes measured after startup, upload, deletion and maintenance
- [ ] Physical free-space floor chosen from those measurements

## Storage engine

- [ ] Volumes, filer metadata, PostgreSQL data and recovery configuration are explicitly persisted
- [ ] Credentials configured, internal ports restricted, unauthenticated reads and writes fail
- [ ] Multipart, signing, checksums, range requests and required headers tested against this exact version
- [ ] Oversized upload rejected before exhausting the disk, or the streaming gateway is selected

## Networking

- [ ] Other machine's browser reaches the API and storage, without disabling firewalls
- [ ] Signed URLs use a hostname the client can reach, with CORS and resumed uploads verified from that client

## Durability

- [ ] A disposable 1 GiB test survives restart, interrupted upload, integrity verification, quota rejection, backup and isolated restore
- [ ] A representative 10 GB test then does the same within measured capacity
- [ ] Application containers can be deleted and restarted with data intact

## Decision

If the engine's overhead is unsuitable, keep the adapter and evaluate another local backend before
building further. Update [ADR 0003](adr/0003-seaweedfs-candidate.md) and [ADR 0004](adr/0004-upload-path.md) with the results.
