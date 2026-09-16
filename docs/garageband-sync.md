# GarageBand project sync / backup

Mirrors a GarageBand `.band` package to a Backblaze B2 bucket, file by file.
Built for the case where zipping a project and uploading it through the browser
stopped being practical.

Unlike the Reaper sync, this does **not** understand the session. GarageBand's
`ProjectData` is a closed binary format, so we can't tell which audio is used,
muted, or where it sits on the timeline. This is a file mirror, not a session
import — the payoff is that nothing needs zipping, only changed files move, and
the copy in B2 doubles as an off-site backup.

## Server setup (one-time)

1. **Create a B2 bucket**, private, and turn on **object versioning**.
2. **Add a lifecycle rule** on that bucket: keep prior versions for **30 days**.
   This is what makes the mirror a snapshot rather than a plain mirror — saving
   over a good take on Tuesday is recoverable until 30 days later. Set it in the
   B2 console; the server does not manage bucket policy.
3. **Create an application key** scoped to that bucket.
4. Set these on the server (they are separate from `JERBOA_S3_*`, which is the
   Hetzner bucket used for band files — the mirror lives at a different provider
   on purpose, so one provider's bad day can't take both):

   ```
   JERBOA_MIRROR_S3_ENDPOINT=s3.us-west-004.backblazeb2.com
   JERBOA_MIRROR_S3_BUCKET=your-bucket
   JERBOA_MIRROR_S3_REGION=us-west-004
   JERBOA_MIRROR_S3_ACCESS_KEY=<application key id>
   JERBOA_MIRROR_S3_SECRET_KEY=<application key>
   ```

   The endpoint and region must match the bucket's — B2 shows both on the bucket
   page. Without these, the mirror endpoints return 503 and the rest of Jerboa
   is unaffected.

Objects are keyed `mirror/<band id>/<project>.band/<path inside the package>`.
The key is deterministic: a changed file overwrites the same key, and B2 keeps
the previous version.

## Getting the tool to a bandmate

Band page → files tab → "sync it instead" → the Mac download. macOS quarantines
anything downloaded from a browser, so in Terminal:

```
chmod +x ~/Downloads/jerboa-sync-<band>
xattr -d com.apple.quarantine ~/Downloads/jerboa-sync-<band>
```

The download has the server URL, band and an API token baked into it, so it
needs no configuration — and it is worth treating like a password.

> **Untested:** the config is appended to the end of the binary, and Go
> ad-hoc-signs darwin/arm64 builds. Appended bytes sit outside any mapped
> segment so this is expected to run fine, but it has not been verified on a
> real Mac. If it dies with "killed: 9", the fix is to ship the config as a
> sidecar file instead.

## Using it

```
jerboa-sync                     # next to a .band, or pick from ~/Music/GarageBand
jerboa-sync "My Song.band"      # explicit
jerboa-sync --dry-run           # list every file with sizes, change nothing
jerboa-sync --pull              # download a project from the server
jerboa-sync --exclude 'Take 3*' # skip files, repeatable
```

Always skipped: `*.nosync` (undo history, frozen-track renders), `Output`,
`.DS_Store`. To skip more permanently, put one pattern per line in
`.jerboaignore` **next to** the `.band` (not inside it). Patterns are
case-insensitive globs; one without a `/` matches any file or folder name,
one with a `/` matches a path from the package root.

`--dry-run` is the tool for deciding what to exclude: it prints every file with
its size and totals up what the current excludes are already saving.

## What makes it survivable on a slow link

A first sync of a large project is hours of upload, unattended. So:

- **Every file is committed separately.** Re-running after any failure skips
  everything already uploaded — a drop costs one file, not the run.
- **Hashes are cached** by size and mtime in `~/Library/Caches/jerboa-sync`, so
  later runs don't re-read every byte of the project.
- **Stalled transfers are killed and retried.** A Wi-Fi drop usually leaves a
  half-open connection that would otherwise hang forever; no bytes for 3
  minutes means the transfer is treated as dead.
- **Retries back off** 5s → 15s → 45s → 2m → 5m, enough to ride out a router
  reboot.
- **The Mac is kept awake** (`caffeinate`) for the duration.
- **Uploads go straight to B2** with a presigned URL — bytes never transit
  jerboa.dad. The server then confirms the object landed at the expected size
  before recording it, so a truncated upload can't leave a row claiming a
  backup exists that isn't there.
- **Progress shows throughput and ETA** on a terminal, and is suppressed when
  output is redirected, so an overnight log stays readable.

## Restoring

`jerboa-sync --pull` recreates the package. Files that differ locally are moved
into a timestamped `.jerboa-backup-<date>` folder next to the project rather
than overwritten, and files that exist only locally are left alone.

To recover a version older than the current one, use the B2 console — per-file
version history lives there, not in Jerboa.
