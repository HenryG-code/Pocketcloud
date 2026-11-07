# SeaweedFS oversized upload trial

**Date:** 2026-11-04. **Status:** partial evidence for [ADR 0004](../adr/0004-upload-path.md). Not a final decision.

**Question:** can a signed upload stop a client from sending more bytes than it was granted?

## Setup

- SeaweedFS 4.52, pinned by digest, in Docker Desktop on WSL2
- AWS SDK for Go v2, and Go 1.26.10
- Each case grants 1 MiB. Oversized cases send 2 MiB, from the same machine
- Repeat with `prototypes/storage-feasibility/oversize.sh`

## Results

| Case | Status | Stored | Result |
|---|---|---|---|
| Sized URL, exactly 1 MiB (control) | 200 | 1024 KiB | Within limit |
| Sized URL, 2 MiB with honest length | 403 | Nothing | Refused |
| Sized URL, 2 MiB with no declared length | 403 | Nothing | Refused |
| Unsized URL, 2 MiB | 200 | 2048 KiB | **Not stopped** |
| Sized part URL, 2 MiB in one part | 403 | Nothing | Refused |
| POST policy, exactly 1 MiB (control) | 204 | 1024 KiB | Within limit |
| POST policy limit 1 MiB, 2 MiB sent | 400 | Nothing | Refused |

## Findings

- Signing `Content-Length` limits the upload, because a different length breaks the signature.
- A URL signed without a size has no limit, so the API must always sign a size.
- A sized part URL refuses a larger part, so total size can be bounded by the size and number of parts the API signs.
- The POST policy's `content-length-range` was enforced in this test.

## Disk use during refusal

A 1 MiB grant was sent a 256 MiB upload, with and without a declared length. Both were refused with 403 in about half a second, and disk use grew by 0 KiB.

## Not tested

- Sending more bytes than the declared `Content-Length` over a raw connection
- Part retries, concurrent sessions and many parts adding up across a session
- Browser access, CORS, and hostnames reachable from another device
- Any SeaweedFS version other than 4.52, so this must rerun after upgrades
