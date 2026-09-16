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

Band page → gear icon → **daw sync** shows a one-line installer. They paste it
into Terminal:

```
curl -fsSL https://jerboa.dad/install.sh | sh
```

It detects Apple Silicon vs Intel, installs into `~/.jerboa` (no sudo), and opens
the browser to approve the connection — they sign in, pick the band, click
authorize. On a Mac it also adds:

- **Jerboa Sync** in `~/Applications`, linked on the Desktop. Drop projects or
  folders onto it to sync them; double-click to re-sync everything it remembers.
- **Sync to Jerboa** in Finder's right-click menu under Quick Actions.

That single paste is the only Terminal step. Why it avoids Gatekeeper: `curl`
doesn't set the quarantine flag browsers add to downloads, and the app and
Quick Action are generated on the Mac itself, so nothing needs signing or
notarization. Re-running the installer updates the tool without re-pairing.

First runs trigger two one-time macOS prompts worth warning people about:
Jerboa Sync asking to control Terminal (that window shows upload progress), and
Terminal asking for Documents or Desktop access if projects live there.

Windows and Linux Reaper users can still download a binary with the token baked
in from the same panel; those keep working.

### Connections and tokens

Pairing (installer or Reaper script) and the settings-page download all issue
the user's single token for that band, **replacing any previous one**. So a
second computer paired on the same account disconnects the first. Use a
separate account per person, or re-run the installer on whichever machine
stopped working.

Credentials live in the user config dir (`~/Library/Application Support/jerboa-sync/config.json`
on a Mac), owner-readable only.

## Using it

Everything a person picks — dropped on the app, chosen in Finder, passed on the
command line, or picked from the double-click menu — is **remembered**. A
remembered folder includes projects saved into it later.

```
jerboa-sync                        # menu: Enter syncs everything remembered
jerboa-sync "My Song.band" ~/Band  # sync these (a folder means every .band in it)
jerboa-sync --dry-run              # show what would move, change nothing
jerboa-sync --pull                 # download a project from the server
jerboa-sync --exclude 'Take 3*'    # skip files, repeatable
jerboa-sync forget PATH            # stop syncing something
jerboa-sync status                 # check the connection (non-zero exit if broken)
```

With nothing remembered, a plain run looks in the current folder,
`~/Music/GarageBand`, and iCloud Drive. At end of input every prompt takes its
safe default, so a plain `jerboa-sync` from cron or launchd syncs everything
remembered and never replaces another copy's backup.

Always skipped: `*.nosync` (undo history, frozen-track renders), `Output`,
`.DS_Store`. To skip more permanently, put one pattern per line in
`.jerboaignore` **next to** the `.band` (not inside it). Patterns are
case-insensitive globs; one without a `/` matches any file or folder name,
one with a `/` matches a path from the package root. `--dry-run` groups files
by track, and each group's name works as a pattern.

### Same name, different project

The server identifies projects by name. Each push records which computer and
folder it came from; if a later push of the same name comes from somewhere
else, it stops and shows both locations before replacing anything. Only typing
`yes` replaces the backup. Answering anything else, or nothing, leaves it alone,
and a declined drop isn't remembered. Moving a project to a new folder also
triggers the question once, since the tool can't tell a move from a
different project with the same name.

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
