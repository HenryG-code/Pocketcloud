# Build plan

Everything here is **planned**. A slice becomes fact only when its tests pass and its row in the
progress table is ticked. [Requirements](requirements.md) say what must be true,
[architecture](architecture.md) says how, the [API contract](../packages/contracts/openapi.yaml) says
what the browser and API agree on, and this document says in what order.

## How to use this plan

Build one slice at a time, each on its own short-lived branch and pull request. Do not start a slice
until the one before it is merged.

Starting prompt for a working session:

```text
Read docs/build-plan.md, docs/requirements.md, docs/architecture.md and packages/contracts/openapi.yaml.
We are on slice <N>: <name>. Build only that slice.
Explain each decision and why, before writing the code for it.
Write the tests listed for the slice, and run them.
Update docs/architecture.md, the progress table in docs/build-plan.md and CHANGELOG.md in the same change.
Write or update an ADR for any decision the slice says to record.
Stop when the slice's "done when" list is met, and tell me what to try by hand.
```

## Where the project stands

The storage engine trials are recorded in [benchmarks](benchmarks/) and summarised in the
[feasibility gates](feasibility.md). No application code exists yet. The gates still open fall into
three groups:

| Open gate | Closed by |
|---|---|
| Free-space measurement and physical floor | Slice 0 |
| Persistence of PostgreSQL and recovery configuration | Slice 1 |
| Multipart, signing, checksums, range requests and headers on the pinned version | Slice 3 |
| Oversized upload stopped before exhausting the disk | Slice 4 |
| Quota rejection | Slice 4 |
| Combined PostgreSQL and object restore, at 1 GiB then 10 GB | Slice 9 |
| Another machine's browser reaches the service | Slice 10 |

## Decisions to make before slice 4

| Decision | Recommendation | Record in |
|---|---|---|
| Upload path | The streaming gateway. The contract is already drafted for it, storage stays internal, and the API enforces byte limits itself | Accept [ADR 0004](adr/0004-upload-path.md) |
| Password hashing and sessions | Argon2id, and opaque server-side sessions in PostgreSQL | New ADR |
| Part size | One fixed size chosen from the trials, stated in the contract | New ADR |

Use the latest supported patch of Go, PostgreSQL and the pinned storage release, and record the exact
versions in the README.

## Slice 0: finish the gates that need no application

**Build:** measure host free space with the storage volume at its planned size, choose the physical
free-space floor from the recorded overhead figures, and choose where the independent backup copy lives.

**Done when:** the two open capacity gates are ticked with a result file, and the floor is a named
configuration value in `.env.example`.

## Slice 1: API skeleton and CI

**Goal:** an empty API that builds, tests and runs the same way locally and in CI.

**Build:**

- `apps/api` as one Go module, laid out to match the building blocks in the architecture: `identity`,
  `uploads`, `files`, `capacity`, a metadata repository and an object-store adapter. Business packages
  import neither HTTP nor S3 code.
- chi router, configuration from the environment with validation at start, graceful shutdown on
  termination signals.
- A pgx pool with bounded size and timeouts. Versioned SQL migrations run by one explicit command with a
  dedicated role. sqlc for queries.
- `/health/live` and `/health/ready`, structured JSON logs with a request ID, RFC 9457 problem details.
- `infra/compose/` with PostgreSQL and the storage engine, both with explicitly persisted volumes and
  internal-only ports. `make dev-local`, `make stop-dev` and `make test` implemented.
- `.github/workflows/ci.yml`: gofmt check, `go vet`, staticcheck, govulncheck, `go test -race`, contract
  lint, secret scan. Read-only token, actions pinned to commit SHAs.

**Tests:** readiness against real PostgreSQL. Delete and recreate the containers and confirm the data
volumes survive.

**Done when:** a fresh clone runs with `make dev-local`, CI is green on a pull request, and the
persistence gate is ticked.

## Slice 2: identity

**Goal:** one owner can sign in and out.

**Build:** `POST /v1/session` and `DELETE /v1/session` as in the contract. The owner account is created
by a command-line command, never through the API. HttpOnly, Secure, SameSite cookies. CSRF protection on
every mutation, with the token header added to the contract, which does not describe it yet. A limit on sign-in attempts. Every later handler receives the authenticated owner from
middleware.

**Tests:** wrong password, expired session, a mutation without a CSRF token, and repeated failed
sign-ins.

**Done when:** an unauthenticated request to any `/v1` path except sign-in is refused.

## Slice 3: object-store adapter

**Goal:** one small interface between the application and storage, proven against the real engine.

**Build:** an interface for create multipart, put part, complete, abort, get with a byte range, head and
delete, implemented with the AWS SDK for Go v2 pointed at the local endpoint. Random object keys only.
Deadlines on every call.

**Tests:** integration tests against the pinned storage container for multipart with several parts, a
retried part, a repeated completion, range reads, an empty object and an abort. These carry the lessons
of the trials into permanent tests.

**Done when:** the multipart and range-request gate is ticked, with any behaviour that differs from S3
written into [ADR 0003](adr/0003-seaweedfs-candidate.md).

## Slice 4: upload sessions and parts

**Goal:** bytes arrive bounded, counted and resumable.

**Build:**

- `POST /v1/uploads`: validate the name and declared size, check the physical floor, and reserve quota in
  one transaction so concurrent uploads cannot both take the last space. State `INITIATED`.
- `PUT /v1/uploads/{uploadId}/parts/{partNumber}`: stream the body to storage through a hard byte limit,
  without buffering it in memory. Record the part's size and newest tag in PostgreSQL. A resent part
  replaces the earlier record. The running total may never exceed the declared size.
- `GET /v1/uploads/{uploadId}` returns the recorded parts. `DELETE` aborts and releases the reservation.
- Limits on concurrent sessions and outstanding reservations. Sessions expire.

**Tests:** two uploads racing for the last quota, a part larger than allowed, a total larger than
declared, an upload cut mid-part and resumed, another owner's session, and free disk below the floor.

**Done when:** acceptance tests 2 and 5 in the requirements pass, and the oversize and quota gates are
ticked.

## Slice 5: completion and verification

**Goal:** a file becomes available only after its bytes are proven.

**Build:**

- `POST /v1/uploads/{uploadId}/complete`: check our own state first, complete the multipart upload
  naming exactly the recorded parts, and move to `VERIFYING`. Repeating the call returns the same result.
- `workers/cleanup` gains a verification job: claim a session with a time-limited claim, stream the
  object through SHA-256, compare the size and the hash supplied by the client, then in one transaction
  create the file version, consume the reservation and set `AVAILABLE`. A mismatch sets `FAILED`.
- Every transition is a conditional update from the expected state.

**Tests:** repeated completion, a corrupted part, a worker killed during verification, and an empty file.

**Done when:** acceptance tests 3 and 4 pass.

## Slice 6: files

**Goal:** list, download and delete.

**Build:** `GET /v1/files` with pagination, `GET /v1/files/{fileId}`, `GET /v1/files/{fileId}/content`
with range support and attachment disposition, `DELETE /v1/files/{fileId}` and `GET /v1/usage`. Delete
removes the metadata and queues a cleanup task. It never deletes the object inline.

**Tests:** range requests, Unicode filenames, another owner's file returning not found, and usage
totals after uploads and deletes.

**Done when:** journeys 1, 3 and 4 work through the API, and acceptance test 1 passes.

## Slice 7: reconciler and cleanup

**Goal:** every crash point in the architecture's table has code that repairs it.

**Build:** worker jobs that expire abandoned sessions and abort their multipart uploads, continue
sessions whose storage completion succeeded before the state was saved, release stale reservations, and
delete objects named by cleanup tasks after a grace period and a fresh reference check.

**Tests:** one test per row of the crash table in section 6.4 of the architecture, plus cleanup racing
an active upload.

**Done when:** acceptance test 6 passes, and each crash-table row links to its test.

## Slice 8: web interface

**Goal:** the five journeys in a browser.

**Build:** `apps/web` with React, Vite and strict TypeScript. Sign in, file list, an upload queue that
hashes and slices files without loading them whole, resume after a refresh by reselecting the file,
download, delete with confirmation, and a usage bar. Design the empty, expired-session, quota-reached,
checksum-mismatch and storage-unreachable states together.

**Tests:** Playwright for the five journeys, including a cut connection and a refresh mid-upload.

**Done when:** all five journeys in the requirements pass in Playwright in CI.

## Slice 9: backup, restore and export

**Goal:** prove the data can be recovered.

**Build:** `make backup-lab` stops writes, dumps PostgreSQL, copies the storage volume and configuration,
and writes a manifest of every file's size and hash. `make restore-test` restores into an isolated
deployment and compares every file with the manifest. A verified export of the whole file tree. A
runbook for restore.

**Done when:** the combined restore passes at 1 GiB and then at the planned quota size with results in
`docs/benchmarks/`, acceptance tests 7 and 8 pass, and the durability gates are ticked.

## Slice 10: second device and first release

**Build:** reach the service from another machine on the local network over TLS, without disabling a
firewall, and complete an upload, a resume and a download from it. README quick start with a short
recording, updated architecture document, changelog and tag `v0.1.0`.

**Done when:** the networking gates are ticked, every acceptance test in the requirements passes, and
the pre-publish checklist is complete. Only then may real files be stored.

## Later scope, one piece at a time

| Slice | Adds | Evidence required |
|---|---|---|
| 11 | Folders | No cycles, and ownership checked on parents |
| 12 | Trash and version history | Physical-space accounting includes every retained byte |
| 13 | Multiple users | Cross-user access tests on every endpoint |
| 14 | Expiring share links | Revocation behaviour tested and stated in the interface |
| 15 | Login-protected internet access | The requirements in [ADR 0006](adr/0006-public-access-through-edge-proxy.md) |
| 16 | Kubernetes on kind | Stateless API replicas with one migration job |

## Progress

| Slice | Status | Evidence |
|---|---|---|
| Storage engine trials | Done | [Benchmarks](benchmarks/) |
| 0 Remaining capacity gates | Not started | |
| 1 Skeleton and CI | Not started | |
| 2 Identity | Not started | |
| 3 Object-store adapter | Not started | |
| 4 Upload sessions and parts | Not started | |
| 5 Completion and verification | Not started | |
| 6 Files | Not started | |
| 7 Reconciler and cleanup | Not started | |
| 8 Web interface | Not started | |
| 9 Backup, restore and export | Not started | |
| 10 Second device and release | Not started | |
