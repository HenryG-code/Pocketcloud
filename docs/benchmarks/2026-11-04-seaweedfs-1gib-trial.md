# SeaweedFS 1 GiB trial

**Date:** 2026-11-04. **Status:** allocation, private access and persistence measured. The 1 GiB gate is
only partly met, and the 10 GB gate is untested.

## Setup

- SeaweedFS 4.52, pinned by digest, as one `weed server` container in Docker Desktop on WSL2
- 256 MiB volume limit, 16 volume maximum, no preallocation, 2 GiB memory limit
- One 1024 MiB random file, sent as a single HTTP PUT from the same machine
- Repeat with `prototypes/storage-feasibility/run.sh`

## Allocation

| Point | Allocated on `/data` | Above the file's 1,048,576 KiB |
|---|---|---|
| After startup | 184 KiB | |
| After upload | 1,048,884 KiB | 308 KiB |
| After restart | 1,048,916 KiB | 340 KiB |
| Straight after delete | 1,048,932 KiB | 356 KiB |
| After manual `volume.vacuum` | 300 KiB | |

The upload took 29.9 seconds, which reflects this one local path only and is not a network measurement.

## Findings

- Overhead was about 0.03% for one large file. Many small files could behave differently.
- The file was split across several volume files, the largest about 193 MB, all under the 256 MiB limit.
- Deleted bytes stayed allocated until vacuum, confirming the 64 MiB result at a larger size.
- SHA-256 matched after upload, restart and a recreated container.
- All access control checks passed. **Corrected:** this was a measurement mistake. The uid 0 came from `docker exec`, while the server process ran as uid 1000. Prototypes now set this explicitly, and it was [confirmed from the host](2026-11-04-seaweedfs-10gib-trial.md).

## Not tested

Multipart and interrupted uploads, oversize and quota rejection, automatic vacuum timing, many small files,
backup and restore, access from another device, and whether a hard crash loses recent writes.
