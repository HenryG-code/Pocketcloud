# SeaweedFS 64 MiB trial

**Date:** 2026-11-04. **Status:** small trial only. The 1 GiB and 10 GB gates are still open.

## Setup

- SeaweedFS 4.52, pinned by digest, as one `weed server` container in Docker Desktop on WSL2
- 256 MiB volume limit, 16 volume maximum, no preallocation, 2 GiB memory limit
- Repeat with `prototypes/storage-feasibility/run.sh 64` at the commit containing this file

## Allocation

| Point | Allocated on `/data` |
|---|---|
| After startup | 184 KiB |
| After uploading 64 MiB (65,536 KiB) | 65,808 KiB |
| After restart | 65,840 KiB |
| Straight after delete | 65,856 KiB |
| After manual `volume.vacuum` | 292 KiB |

The upload took 3.4 seconds on the same machine, so it says little about real transfer speed. The file
was spread across at least 6 volume files.

## Findings

- With these settings, nothing large is preallocated at startup.
- Overhead on one 64 MiB file was small. Many small files may behave differently, and this was not tested.
- **Deleting does not free disk space until vacuum runs.** Physical accounting must count deleted bytes until then.
- Data survived a restart and a recreated container, with matching SHA-256.
- Anonymous reads and writes and a wrong secret were refused. Internal ports were not published.
- The container runs as root (uid 0), which conflicts with the planned non-root standard and is unresolved.

## Not tested

Interrupted or multipart uploads, oversize rejection, quota rejection, automatic vacuum timing, many small
files, access from another device, backup and restore, and multi-service deployment.
