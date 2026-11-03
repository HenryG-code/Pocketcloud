# Requirements

## User and problem

One person wants a private place for their own files that they control, including moving files
between their machines safely. Other users come later.

## Essential journeys (first release)

1. Sign in, upload a file, and see it listed
2. Lose the connection mid-transfer, resume, and end with the correct bytes
3. Download a file and verify it matches the source
4. Delete a file explicitly
5. Be refused clearly when quota or physical space would be exceeded

Failure and empty states: empty drive, expired session, quota reached, checksum mismatch, storage unreachable.

## Scope stages

| Stage | Includes |
|---|---|
| Feasibility prototype | Disposable storage, network, integrity and restore tests, including a representative 10 GB batch |
| First usable release | One owner, authenticated upload, list and download, persistent data, explicit delete, quotas, interrupted-transfer recovery, verified export, independent backup method |
| Later scope | Folders, multiple users, expiring shares, trash, version history, previews, Kubernetes |

**Excluded for now:** automatic trash purge, version retention, public access, and any automatic deletion of user data to maintain a limit.

## Invariants

- A file is never marked available until its integrity check passes.
- A user can never read or change another user's files or upload sessions.
- Concurrent uploads cannot jointly exceed quota.
- Uploads cannot fill the disk: physical bytes are bounded during transfer, not only after it.
- Cleanup never deletes a referenced object or an active upload.

## Non-functional requirements

- Stream content and never buffer a whole file in API or browser memory.
- Storage endpoints stay private and authenticated from the start.
- LAN-only for the first deployment, with TLS outside localhost.
- Downtime is accepted. Saved data must survive restarts.

## Acceptance tests

- [ ] Another user cannot read, overwrite, move, or complete uploads belonging to someone else
- [ ] Concurrent uploads cannot exceed quota, and rejected or expired sessions release reservations
- [ ] Interrupted multipart uploads resume, and repeated completion creates no duplicate version
- [ ] Failed checksum validation never publishes the file
- [ ] An upload exceeding its declared size is stopped before exhausting the host
- [ ] Cleanup cannot delete a retained version or a legitimately active upload
- [ ] A database backup plus an object backup restores a file tree with matching checksums
- [ ] Validation includes large files, Unicode filenames, nested folders and empty files

Later scope: renames preserve contents and history, and expired or revoked shares issue no new downloads.
