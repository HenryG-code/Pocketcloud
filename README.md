# PocketCloud

A self-hosted cloud drive: upload, download and manage your files on hardware you own, with
resumable transfers and verified backups.

> **Status: not started.** This repository holds the plan and structure. Nothing below is implemented,
> and no claim here has been tested.

## Stack

Go, chi, pgx and sqlc, React with Vite and TypeScript, PostgreSQL, and a self-hosted S3-compatible
object store behind a replaceable adapter. SeaweedFS is the candidate, pending
[its feasibility gate](docs/feasibility.md).

## Build order

1. [Feasibility prototype](docs/feasibility.md): storage, networking, integrity and restore, using disposable data
2. First usable release: one owner, upload, list, download, delete, quotas, interrupted-transfer recovery and verified export
3. Later: multiple users, share links, trash, version history, previews, Kubernetes

## Quick start

Not available yet. Copy `.env.example` to `.env` and generate your own secrets when setup exists.
Commands are listed by `make help`.

## Documentation

Start with the [documentation index](docs/README.md).

- [Requirements](docs/requirements.md)
- [Architecture](docs/architecture.md): arc42 structure with C4 diagrams
- [API contract](packages/contracts/openapi.yaml): OpenAPI draft
- [Threat model](docs/threat-model.md)
- [Feasibility gates](docs/feasibility.md)
- [Decision records](docs/adr/)

## Limitations

PocketCloud uses disks you already own, so it adds no capacity and does not replace an off-site backup.
Two copies on one disk do not protect against that disk failing.

## Licence

Not yet chosen.
