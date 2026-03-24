# Jerboa — Full Project Context

## Project Overview

**Jerboa** (jerboa.dad, "all ears.") is a collaborative band platform for uploading, reviewing, and commenting on audio recordings with timestamp-based comments.

**Tech stack:**
- Backend: Go (chi router, pgx, go-oidc, nhooyr/websocket)
- Frontend: Svelte 5 + TypeScript + Tailwind CSS v4 + wavesurfer.js
- Database: PostgreSQL 15 (running on Hetzner VPS + local dev)
- Auth: OIDC/SSO + magic link email login + invite token auth
- Audio processing: FFmpeg for probing + waveform peak generation
- YouTube import: yt-dlp on server, extracts audio from YouTube URLs
- Deployment: bare binary + systemd + Caddy (already on VPS), no Docker in prod
- Fonts: Chakra Petch (headings/display), Rajdhani (body)

**Mascot:** Jerbert — the jerboa in the logo. He is beloved.

**Design:** Vectorheart aesthetic (The Designers Republic / Wipeout era) — dark default, teal (#2ec4b6) accent, geometric sans-serifs, wide tracking, angular shapes, no rounded corners. LED spectrum analyzer on login page. All small text must be semibold minimum.

**Key features:**
- Bands with members/invites
- Track upload with background FFmpeg processing
- Tags on tracks (freeform)
- Song grouping (tracks belong to songs)
- Editable track metadata (title, description, notes/lyrics/tabs)
- YouTube URL import via yt-dlp
- Timestamp-based comments with waveform markers
- WebSocket real-time updates
- Dark/light theme
- Overdub system with voting and bouncing
- Perform mode (full-screen swipeable lyrics with ChordPro chord notation)
- Freestyle perform mode (pick songs on the fly, save as set)
- Sets/setlists with timestamp regions into recordings
- Direct browser recording via MediaRecorder API
- Feedback/bug reporting with screenshot capture
- Admin panel
- Car mode (continuous playback)
- PWA with service worker

---

## About Stefan (the creator)

Stefan is a developer who is strong on backend/systems but newer to frontend frameworks. Deep familiarity with Go, comfortable with general programming concepts. Learning Svelte/TypeScript — knows enough to be dangerous but needs guidance on fundamentals. Hates React, vibes with Svelte's approach. Wants to learn and build understanding, not just be handed solutions.

Stefan runs "The Whether Network" (whether.network) as his umbrella brand across all projects. Jerboa is one product under it.

---

## Product Philosophy

Jerboa's guiding philosophy — each feature maps to a familiar workflow from a best-in-class tool:

- **Discuss** like Soundcloud (comments on tracks, timestamped feedback)
- **Record** like Ableton (overdubs, bouncing, latency compensation)
- **Listen** like Spotify (persistent player, setlists, car mode)
- **Chat** like Messages (band chat)
- **Review** like GitHub (approve takes, vote on overdubs)
- **Perform** like OnSong (full-screen swipeable lyrics view for live shows/rehearsals, tied to setlists, ChordPro-style inline chords with toggle show/hide per player)

Each verb maps to a real workflow users already understand from a best-in-class tool, but consolidated into one focused band collaboration space.

---

## Business Model Direction

**Licensing:** Plan is open-source (AGPL or BSL) with paid hosted tier. Self-host free, pay for cloud hosting.

**Pricing model (evolving):**
- Free tier with full band collaboration features and limited storage (~1-2 GB)
- Paid tier (~$8-12/mo per band) for more storage + subscriber/fan features
- Subscriber/fan tier: non-members get read-only access to a band's creative process (tracks, comments, recording journey)
- Platform takes 10-15% cut of subscriber fees paid to bands (like Patreon/Substack model)
- Subscribers are cheapest users (listen-only, no uploads/processing)
- Self-hosted: everything, unlimited, forever

**Key insight — process-as-content:** Bands don't create extra content for subscribers. Fans watch the creative process unfold organically — rough takes, comments, iteration. "Like watching a band's group chat but for music." This differentiates from Bandcamp (finished product → fan) by being (creative process → fan).

**Growth model:** Bands market Jerboa to their own fans ("follow our recording process on Jerboa"). Organic distribution — every band is a channel.

**Infra reality:** Storage is main cost. S3-compatible on Hetzner ~€0.005/GB/mo. Single Go binary + Postgres + Caddy scales to hundreds of bands on one VPS. No k8s needed.

---

## Feature Roadmap

- **Persistent player bar** — Spotify-style bottom bar that survives navigation. Audio element lives in Layout, playlist/queue system.
- **Band photo headers** — hero image on band page for personality.
- **Count-in for recording** — songs have a BPM field (manual or auto-detected), count-in clicks before recording starts. Especially useful for overdubs.
- **Live recording with song tagging** — record during perform mode and tap to tag when each song starts/ends. Creates a single recording with set_item timestamps auto-populated. Each song transition captures current recording position as start_ms/end_ms.
- **Web Audio DSP** — compressor/EQ on playback via Web Audio API AudioWorklet. Far future.

---

## Preferences & Feedback

- **Lean tooling:** Prefers minimal/lean over feature-rich but bloated. Svelte over React. Keep bundle sizes small.
- **UI weight:** All small text (text-sm, text-xs, custom small sizes) must be at least semibold (600) weight for HiDPI readability.
- **No rounded corners.** Angular, geometric shapes throughout.
- **Keep self-hosting easy** — single binary deployment.
- **Design features with the subscriber read-only view in mind** for future monetization.

---

## Recent Session Work (2026-03-22)

Built in this session: feedback/bug reporting system, take uploads on SongView, mobile BandView fixes, superadmin page, PWA service worker cache versioning, track downloads, browser recording (MediaRecorder API), overdub system (upload/vote/bounce/scrub), magic link + invite token auth, smart email routing (Gmail → OIDC, others → magic link), display name onboarding, admin user/invite deletion, perform mode with ChordPro lyrics, freestyle perform mode (pick songs on the fly, save as set).
