# SeaweedFS backup and restore trial

**Date:** 2026-11-04. **Status:** the object store restores correctly at about 70 MiB. Backup with PostgreSQL, larger sizes and an independent disk are untested.

## Setup

- SeaweedFS 4.52, pinned by digest, in Docker Desktop on WSL2
- 1,005 generated files: one 64 MiB file, 1,000 files of 4 KiB, an empty file, a deeply nested file, and Unicode and symbol filenames
- A source manifest of SHA-256 hashes, recorded independently of storage
- Backup with storage stopped, using `tar` on the data volume
- Repeat with `prototypes/storage-feasibility/restore.sh`

## Results

| Check | Result |
|---|---|
| Original volume destroyed before restoring | Yes |
| Archive hash unchanged before restoring | Yes |
| Restored into a separate deployment | Yes |
| File names match the manifest exactly | Yes |
| Files with wrong bytes after restore | 0 of 1,005 |
| New write after restore | Accepted |
| Anonymous read after restore | Refused |
| Archive size | 70,141 KiB, in 2.1 seconds |
| Restore and start | 8.0 seconds |

## Deliberate damage

One byte was changed in the middle of the largest volume file, with storage stopped. Afterwards, 1,004 files
read back exactly, 1 read was refused, and no wrong bytes were returned. This is one trial of one byte, so it
shows that this damage was detected, not that all damage will be.

## Findings

- The volume archive alone restored every file and its name. Filer metadata lives in the same volume.
- **The archive does not contain the access credentials.** They are in a separate config file, which this test kept. A real backup must save that too, separately and protected.
- One 64 MiB file used about 0.4% overhead, while 1,000 files of 4 KiB used about 12.6%, so small files cost proportionally more.
- Damage made a read fail rather than return wrong bytes, so a damaged file needs restoring from backup. Replication and repair were not tested.

## Not tested

- Backing up PostgreSQL and objects together, which is what a real restore depends on
- Backing up while storage is running
- An independent physical disk. This archive was on the same disk, so it would not survive that disk failing
- 1 GiB or 10 GB restores, and damage to metadata rather than file data
