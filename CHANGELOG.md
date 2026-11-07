# Changelog

Notable changes are recorded here, following [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Project structure and planning documentation.
- MIT licence.
- Storage feasibility prototype and first SeaweedFS trial results.
- Oversized upload test in Go.
- Interrupted upload and crash recovery test in Go.
- Backup, restore and deliberate damage test in Go.
- Batch backup and restore test, run at 1 GiB and 10 GiB.
- Proposed decision record for public access through an edge proxy.

### Changed

- Storage prototype container runs with no capabilities and a read-only root filesystem.

### Fixed

- Backup archives no longer pass through Docker's logs.
- Prototype scripts clear the folder Docker creates when a mounted file is missing.
- Corrected an earlier finding that the storage container runs as root.
