<p align="center">
  <img src="jerboalogo.svg" width="140" alt="Jerbert the jerboa" />
</p>

<h1 align="center">Jerboa</h1>
<p align="center"><em>all ears.</em></p>

Jerboa is a self-hosted collaboration platform for bands: upload demos and rehearsal recordings, leave timestamp-anchored comments directly on the waveform, keep lyrics and tabs next to the tracks they belong to, and run your sets from your phone. Built because shuffling bounces over group chat and losing feedback in scroll-back is no way to make a record.

Actively used by four bands (and counting).

<!-- TODO: screenshot / demo GIF here -->

## Features

- **Bands and invites** — members, roles, invite links; each band's material stays its own
- **Track uploads with background processing** — FFmpeg probing and waveform peak generation server-side
- **Timestamp comments** — feedback pinned to the exact moment on the waveform, updated live over WebSockets
- **Songs and versions** — group takes and bounces under the song they belong to, with editable notes, lyrics, and tabs
- **Overdubs** — propose parts on top of existing tracks, vote, and bounce the winners
- **Perform mode** — full-screen swipeable lyrics with ChordPro chord notation; freestyle mode for picking songs on the fly and saving the result as a set
- **Sets and setlists** — with timestamp regions into full rehearsal recordings
- **YouTube import** — pull audio in from a URL via yt-dlp
- **In-browser recording** — capture ideas straight from the mic via MediaRecorder
- **Car mode, PWA, dark/light themes** — listen back on the drive home
- **REAPER integration** — Lua scripts in [`extras/`](extras/) for syncing bounces out of your DAW

## Stack

Go backend (chi, pgx, OIDC, WebSockets) · Svelte 5 + TypeScript frontend · PostgreSQL · FFmpeg for audio processing. Deployed as a single binary with the frontend embedded — systemd + Caddy in production, Docker Compose for development.

## Self-hosting

Requirements: Go 1.25+, Node 20+, PostgreSQL 15+, FFmpeg.

```sh
cp .env.example .env    # then edit: secret, database URL, OIDC or magic-link email
make migrate            # apply database migrations
make dev                # backend + frontend dev servers
```

For production, `make build` produces a single self-contained binary in `bin/`; point systemd (or anything) at it. `docker compose up -d` works too. Auth supports any OIDC provider (Authentik, Keycloak, Google, ...) plus magic-link email login. File storage is local-disk or any S3-compatible object store.

A hosted version lives at [jerboa.dad](https://jerboa.dad) if you'd rather not run your own.

## License

[AGPL-3.0](LICENSE). Run it, fork it, improve it — if you host a modified version for others, share your changes.

---

<p align="center">The jerboa in the logo is named Jerbert. He is beloved.</p>
