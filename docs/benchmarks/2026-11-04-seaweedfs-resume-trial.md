# SeaweedFS interrupted upload trial

**Date:** 2026-11-04. **Status:** resume and crash recovery work at 64 MiB. Larger sizes, backup and restore are untested.

## Setup

- SeaweedFS 4.52, pinned by digest, in Docker Desktop on WSL2
- 64 MiB of random data in eight 8 MiB parts, each sent through a size-signed URL from the same machine
- Crashes use SIGKILL on the storage container
- Repeat with `prototypes/storage-feasibility/resume.sh`

## Results

| Scenario | Result |
|---|---|
| Connection cut halfway through part 4 | Part 4 not recorded, parts 1 to 3 intact |
| Storage killed with no warning | Parts 1 to 3 still recorded |
| Resume | Only parts 4 to 8 were needed |
| Completed file | Correct size, and SHA-256 matches the source |
| Killed again after completion | SHA-256 still matches |
| Complete with a wrong part tag | Refused, and nothing published |
| Retry a part with different bytes | The retry's bytes are stored |

## Allocation

| Point | Allocated on `/data` |
|---|---|
| 3 parts saved (24 MiB) | 24,844 KiB |
| After the cut part (4 MiB sent) | 28,944 KiB |
| After retrying part 2 (8 MiB resent) | 37,176 KiB |
| Finished 64 MiB file (65,536 KiB) | 78,156 KiB |
| 3 abandoned parts, vacuum without abort | 24,928 KiB |
| Vacuum straight after abort | 25,012 KiB |
| Vacuum about 15 seconds after abort | 416 KiB |

## Findings

- **Cut transfers use disk even though no part is recorded.** The 4 MiB sent stayed allocated.
- **Retries add disk use too.** The finished file used about 19% more than its size until cleaned up.
- **Vacuum alone does not free an abandoned upload.** It must be aborted first, and deletion then takes a short time.
- **A retried part is listed twice,** so `ListParts` cannot be the record of which parts belong to a file.
- **Repeating complete returns 404,** so storage does not make completion idempotent. The API must.
- The retry's bytes were stored in both completion methods tried, including naming the retry's tag.

## Not tested

- Power loss or an operating system crash. SIGKILL ends the process, but the kernel can still write out cached data
- Files larger than 64 MiB, and many concurrent uploads
- Retrying after an interrupted complete request
- Automatic cleanup timing, and how often vacuum would run by itself
- Browsers and other devices
- Backup and restore
