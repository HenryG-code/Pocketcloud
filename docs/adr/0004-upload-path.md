# 0004: How file bytes travel during upload

- **Status:** Proposed. Depends on the [feasibility gates](../feasibility.md)
- **Date:** 2026-11-04

## Problem

Uploads must be bounded during transfer. Checking size afterwards is too late, because the disk may already be full.

## Options

| | Signed URLs (browser to storage) | Streaming gateway (browser to API to storage) |
|---|---|---|
| Throughput | API carries no file traffic | All bytes pass through the API |
| Byte limits | Only if the backend can enforce them | The API enforces them directly |
| Network exposure | Storage must be reachable from the browser | Storage stays internal |
| Complexity | Hostnames, CORS and signature details | Streaming code and part tracking |
| Credential risk | Signed URLs are bearer credentials | No extra credential to leak |

## Decision

Test whether the chosen backend enforces request, part and session limits on signed uploads. Use signed URLs
only if it does. Otherwise, the first release uses the gateway. The [API contract](../../packages/contracts/openapi.yaml)
is drafted for the gateway as the safer default.

## Consequences

- Both options share the same session, part and completion states, so switching later is contained.
- With one user, the gateway's throughput cost is unlikely to matter, though this is unmeasured.
