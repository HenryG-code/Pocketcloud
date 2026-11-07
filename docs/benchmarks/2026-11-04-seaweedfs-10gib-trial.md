# SeaweedFS 10 GiB backup and restore trial

**Date:** 2026-11-04. **Status:** the object store restores correctly at 10 GiB. PostgreSQL, the application's quota and an independent disk are untested.

## Setup

- SeaweedFS 4.52, pinned by digest, in Docker Desktop on WSL2, on one desktop's disk
- The container runs as uid 1000 with no capabilities, no new privileges, a read-only root filesystem and a 2 GiB memory limit
- 2,165 generated files: one 5,120 MiB file, 160 files of 32 MiB, 2,000 small files, and edge cases
- A source manifest of hashes, recorded independently of storage
- Backup with storage stopped, using `tar` on the data volume with Docker logging turned off
- Repeat with `prototypes/storage-feasibility/batch.sh 10240`, or `batch.sh 1024` for the shorter run

## Results

| Check | 1 GiB | 10 GiB |
|---|---|---|
| Checks passed | 5 of 5 | 5 of 5 |
| Files with wrong bytes before backup | 0 | 0 |
| Original volume destroyed before restoring | Yes | Yes |
| Archive hash unchanged before restoring | Yes | Yes |
| File names match the manifest exactly | Yes | Yes |
| Files with wrong bytes after restore | 0 | 0 |
| Storage overhead | 0.12% | 0.03% (2,805 KiB on 10,552,363 KiB) |
| Backup, including its hash | 3 seconds | 116 seconds |
| Restore and start | 12 seconds | 69 seconds |

At 10 GiB, storing took 58 seconds and verifying every byte took 13 seconds. Allocated space was 10,567,304 KiB straight
after deleting everything, and 15,928 KiB after vacuum.

## The failure that came first

The first 10 GiB attempt used far more disk than it should have, about 64 GiB, and kept the disk 100% busy until Docker was closed.
The likely cause was the backup design: `tar` sent the archive over the container's standard output, and Docker's default
`json-file` logging stored a copy of that output.

Turning logging off for the backup container (`--log-driver none`) removed the growth. At 1 GiB, backup fell from 35 to 3 seconds,
and restore and start from 54 to 12 seconds. Docker's disk file did not grow at either 1 GiB or 10 GiB with the fix. We did not run a separate
experiment isolating the log file, so the cause is inferred from this before and after.

Docker's disk file does not shrink by itself after growing, so the earlier attempt's growth stays until it is compacted.

## Findings

- A full backup needs room for a second complete copy while it runs. Plan the physical budget around that.
- Overhead stayed low with this file mix, at a size close to the planned 10 GiB quota.
- Deleted space came back only after vacuum, confirming the smaller trials at this size.
- The hardened container settings did not break any check.
- Anything sent through a container's output can also land in its logs. Backups and other bulk data must not take that route with logging on.

## Not tested

- Backing up PostgreSQL and objects together, which is what a real restore depends on
- Backing up while storage is running
- An independent physical disk. This archive was on the same disk, so it would not survive that disk failing
- Interrupted uploads at this size, which have only run at 64 MiB
- Quota rejection, which needs the application
- Access from a browser or another device
