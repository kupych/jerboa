<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, apiPost, apiPatch, apiDelete, uploadFile } from "../lib/api";
  import { ws } from "../lib/ws";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, formatFileSize, formatRelativeTime } from "../lib/utils/format";
  import WaveformPlayer from "../lib/components/WaveformPlayer.svelte";
  import CommentList from "../lib/components/CommentList.svelte";
  import Recorder from "../lib/components/Recorder.svelte";
  import MultiTrackMixer, { type MixerTrack } from "../lib/components/MultiTrackMixer.svelte";
  import { layoutWidth } from "../lib/stores/layoutWidth";
  import { isDemo } from "../lib/stores/auth";

  let { slug, trackId }: { slug: string; trackId: string } = $props();

  interface Song {
    id: string;
    name: string;
    bpm: number;
  }

  interface SetSummary {
    id: string;
    name: string;
    set_type: string;
  }

  interface Track {
    id: string;
    band_id: string;
    title: string;
    description?: string;
    notes?: string;
    waveform_data?: number[];
    duration_ms: number;
    format: string;
    sample_rate: number;
    file_size: number;
    status: string;
    tags: string[];
    song_id?: string;
    song?: Song;
    source_url?: string;
    recorded_at?: string;
    set_id?: string;
    overdub_of?: string;
    offset_ms: number;
    bounced_to?: string;
    pre_bounce_id?: string;
    bounce_versions?: number;
    rpp_session_name?: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
  }

  interface Comment {
    id: string;
    body: string;
    timestamp_ms: number | null;
    created_at: string;
    user: { id: string; display_name: string; email: string; avatar_url?: string };
    replies?: Comment[];
  }

  interface Member {
    user: { id: string; display_name: string; email: string };
    role: string;
  }

  interface Personnel {
    user_id: string;
    role: string;
    user: { id: string; display_name: string; email: string; avatar_url?: string };
  }

  let track = $state<Track | null>(null);
  let comments = $state<Comment[]>([]);
  let loading = $state(true);
  let newComment = $state("");
  let commentTimestamp = $state<number | null>(null);
  let posting = $state(false);
  let newTag = $state("");
  let savingTags = $state(false);

  let songs = $state<Song[]>([]);
  let allSets = $state<SetSummary[]>([]);
  let members = $state<Member[]>([]);
  let personnel = $state<Personnel[]>([]);
  let addPersonnelId = $state("");
  let addPersonnelRole = $state("");

  // Overdubs
  type Overdub = {
    id: string;
    title: string;
    waveform_data?: number[];
    duration_ms: number;
    format: string;
    file_size: number;
    status: string;
    offset_ms: number;
    created_at: string;
    vote_count: number;
    user_voted: boolean;
    uploader?: { display_name: string; email: string };
  };

  let overdubs = $state<Overdub[]>([]);
  let showOverdubRecord = $state(false);
  let showRecordingControls = $state(false);
  let voting = $state(false);
  let bouncing = $state(false);
  let scrubbing = $state(false);
  let deletingOd = $state(false);
  let punchInMs = $state(0);
  let showOverdubUpload = $state(false);
  let overdubFile = $state<File | null>(null);
  let uploadOffsetMs = $state(0);
  let uploadingOverdub = $state(false);
  let showLinkTrack = $state(false);
  let linkableTracks = $state<Track[]>([]);
  let linkingTrackId = $state<string | null>(null);
  let linkOffsetMs = $state(0);
  let linkSearch = $state("");
  let playerRef: any;
  let mixerRef: any;
  let mixerPositionMs = $state(0);
  let mixerPlaying = $state(false);

  let mixerTracks = $derived<MixerTrack[]>(
    track
      ? [
          // Omit the parent if it has no audio (e.g. a Reaper session container track)
          ...(track.file_size > 0
            ? [{
                id: track.id,
                title: track.title,
                duration_ms: track.duration_ms,
                offset_ms: 0,
                isParent: true,
                streamUrl: `/api/bands/${slug}/tracks/${trackId}/stream`,
                gain: track.gain,
                loudness_lufs: track.loudness_lufs,
              }]
            : []),
          ...overdubs.map((od) => ({
            id: od.id,
            title: od.title,
            duration_ms: od.duration_ms,
            offset_ms: od.offset_ms,
            isParent: false,
            streamUrl: `/api/bands/${slug}/tracks/${od.id}/stream`,
            uploader: od.uploader,
            vote_count: od.vote_count,
            user_voted: od.user_voted,
            gain: od.gain,
            loudness_lufs: od.loudness_lufs,
          })),
        ]
      : []
  );
  let highlight = $state(false);
  let isAdmin = $derived(
    members.some((m) => m.user.id === $currentUser?.id && m.role === "admin")
  );
  let isRppSession = $derived(!!(track?.rpp_session_name));

  import { user as currentUser } from "../lib/stores/auth";

  // Latency calibration
  let calibrating = $state(false);
  let calibrationMsg = $state("");
  let calibratedLatency = $state<number | null>(
    (() => {
      try {
        const v = localStorage.getItem("overdub-latency-ms");
        return v ? parseInt(v) : null;
      } catch { return null; }
    })()
  );
  let activeOverdubMode = $derived(
    showOverdubRecord ? {
      code: "REC",
      title: "Record Armed",
      detail: `Parent rolls from ${punchInMs > 0 ? formatDuration(punchInMs) : "0:00"}${calibratedLatency != null ? ` with ${calibratedLatency}ms compensation` : ""}.`
    } :
    showOverdubUpload ? {
      code: "UPL",
      title: "Upload Mode",
      detail: "Import a pre-recorded overdub and place it against the parent track."
    } :
    showLinkTrack ? {
      code: "LNK",
      title: "Link Mode",
      detail: "Attach an existing track as a shared overdub reference instead of creating a duplicate file."
    } :
    null
  );

  async function calibrate() {
    calibrating = true;
    calibrationMsg = "listen for 4 clicks, then clap on beat 5...";
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const actx = new AudioContext();

      // Start recording
      const recorder = new MediaRecorder(stream);
      const chunks: Blob[] = [];
      recorder.ondataavailable = (e) => { if (e.data.size > 0) chunks.push(e.data); };

      const stopped = new Promise<void>((res) => { recorder.onstop = () => res(); });
      recorder.start();

      // Let recorder settle
      await new Promise((r) => setTimeout(r, 300));

      // Play 4 clicks at 120BPM (500ms apart)
      const interval = 0.5;
      const t0 = actx.currentTime;
      for (let i = 0; i < 4; i++) {
        const osc = actx.createOscillator();
        osc.frequency.value = 1000;
        const gain = actx.createGain();
        const t = t0 + i * interval;
        gain.gain.setValueAtTime(0.5, t);
        gain.gain.exponentialRampToValueAtTime(0.001, t + 0.05);
        osc.connect(gain);
        gain.connect(actx.destination);
        osc.start(t);
        osc.stop(t + 0.06);
      }

      // Expected clap: 300ms settle + 4 beats * 500ms = 2300ms from recording start
      const expectedClapMs = 300 + 4 * interval * 1000;

      // Record for 3.5 seconds total
      await new Promise((r) => setTimeout(r, 3200));
      recorder.stop();
      stream.getTracks().forEach((t) => t.stop());
      await stopped;

      // Decode and analyze
      const blob = new Blob(chunks, { type: recorder.mimeType });
      const arrayBuf = await blob.arrayBuffer();
      const audioBuf = await actx.decodeAudioData(arrayBuf);
      const data = audioBuf.getChannelData(0);
      const sr = audioBuf.sampleRate;

      // Search for clap transient in window around expected time
      const searchStartSample = Math.floor(((expectedClapMs - 400) / 1000) * sr);
      const searchEndSample = Math.floor(((expectedClapMs + 1000) / 1000) * sr);

      let maxVal = 0;
      for (let i = searchStartSample; i < searchEndSample && i < data.length; i++) {
        const abs = Math.abs(data[i]);
        if (abs > maxVal) maxVal = abs;
      }

      if (maxVal < 0.02) {
        calibrationMsg = "no clap detected — try again louder";
        return;
      }

      // Find onset: first sample above 20% of peak in search window
      const threshold = maxVal * 0.2;
      let clapSample = -1;
      for (let i = searchStartSample; i < searchEndSample && i < data.length; i++) {
        if (Math.abs(data[i]) > threshold) {
          clapSample = i;
          break;
        }
      }

      if (clapSample === -1) {
        calibrationMsg = "couldn't detect clap onset — try again";
        return;
      }

      const clapMs = (clapSample / sr) * 1000;
      const latency = Math.round(clapMs - expectedClapMs);

      calibratedLatency = latency;
      localStorage.setItem("overdub-latency-ms", String(latency));
      calibrationMsg = `latency: ${latency}ms — will auto-adjust future overdubs`;

      actx.close();
    } catch (e: any) {
      calibrationMsg = e.message || "calibration failed";
    } finally {
      calibrating = false;
    }
  }

  // Delete
  let confirmDelete = $state(false);
  let deleting = $state(false);

  async function deleteTrack() {
    deleting = true;
    try {
      await apiDelete(`/api/bands/${slug}/tracks/${trackId}`);
      navigate(`/band/${slug}`);
    } finally {
      deleting = false;
    }
  }

  // Editable meta
  let editing = $state(false);
  let editTitle = $state("");
  let editDesc = $state("");
  let editNotes = $state("");
  let editRecordedAt = $state("");
  let savingMeta = $state(false);
  let assigningSet = $state(false);

  // Song combobox
  let songInput = $state("");
  let songDropdownOpen = $state(false);
  let filteredSongs = $derived(
    songInput.trim()
      ? songs.filter((s) => s.name.toLowerCase().includes(songInput.trim().toLowerCase()))
      : songs
  );
  let assigningSong = $state(false);

  let streamVersion = $state(0);
  let streamUrl = $derived(`/api/bands/${slug}/tracks/${trackId}/stream${streamVersion ? `?v=${streamVersion}` : ""}`);
  let timedComments = $derived(
    comments
      .flatMap((c) => {
        const items = [];
        if (c.timestamp_ms != null) {
          items.push({
            id: c.id,
            timestamp_ms: c.timestamp_ms,
            body: c.body,
            user_name: c.user.display_name || c.user.email,
          });
        }
        return items;
      })
  );

  // Reload when trackId changes (navigation between tracks, e.g. "view original")
  $effect(() => {
    const _id = trackId;
    streamVersion = 0;
    track = null;
    overdubs = [];
    comments = [];
    loadData().then(() => {
      if (new URLSearchParams(window.location.search).has("highlight")) {
        highlight = true;
        window.history.replaceState(null, "", window.location.pathname);
        setTimeout(() => (highlight = false), 2000);
      }
    });
  });

  $effect(() => {
    if (track) {
      ws.subscribe(`track:${track.id}`);
      ws.subscribe(`band:${track.band_id}`);
      const offComment = ws.on("comment.new", () => loadComments());
      const offTrack = ws.on("track.ready", (payload: any) => {
        if (payload.track_id === trackId) {
          // Our track was updated (e.g. bounce replaced the file)
          streamVersion++;
          loadData();
          bouncing = false;
        }
      });
      return () => {
        ws.unsubscribe(`track:${track!.id}`);
        ws.unsubscribe(`band:${track!.band_id}`);
        offComment();
        offTrack();
      };
    }
  });

  async function loadData() {
    loading = true;
    try {
      const bandDetail = api<{ band: any; members: Member[] }>(`/api/bands/${slug}`);
      [track, songs, personnel, allSets] = await Promise.all([
        api<Track>(`/api/bands/${slug}/tracks/${trackId}`),
        api<Song[]>(`/api/bands/${slug}/songs`),
        api<Personnel[]>(`/api/bands/${slug}/tracks/${trackId}/personnel`),
        api<SetSummary[]>(`/api/bands/${slug}/sets`),
      ]);
      const bd = await bandDetail;
      members = bd.members;
      await Promise.all([loadComments(), loadOverdubs()]);
    } finally {
      loading = false;
    }
  }

  async function loadComments() {
    comments = await api<Comment[]>(`/api/tracks/${trackId}/comments`);
  }

  function handleTimestampClick(ms: number) {
    commentTimestamp = ms;
    document.getElementById("comment-input")?.focus();
  }

  function handleSeek(ms: number) {
    playerRef?.seekTo(ms);
  }

  function scrollToComment(commentId: string) {
    const el = document.getElementById(`comment-${commentId}`);
    if (el) el.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  async function submitComment() {
    if (!newComment.trim()) return;
    posting = true;
    try {
      await apiPost(`/api/tracks/${trackId}/comments`, {
        body: newComment.trim(),
        timestamp_ms: commentTimestamp !== null ? Math.round(commentTimestamp) : null,
      });
      newComment = "";
      commentTimestamp = null;
      await loadComments();
    } finally {
      posting = false;
    }
  }

  function clearTimestamp() {
    commentTimestamp = null;
  }

  async function loadOverdubs() {
    overdubs = await api<Overdub[]>(`/api/bands/${slug}/tracks/${trackId}/overdubs`);
  }

  async function openLinkTrack() {
    showLinkTrack = true;
    showOverdubUpload = false;
    showOverdubRecord = false;
    if (linkableTracks.length === 0) {
      linkableTracks = await api<Track[]>(`/api/bands/${slug}/tracks`);
    }
  }

  async function linkTrack() {
    if (!linkingTrackId) return;
    try {
      await api(`/api/bands/${slug}/tracks/${trackId}/overdubs/link`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ source_track_id: linkingTrackId, offset_ms: linkOffsetMs }),
      });
      showLinkTrack = false;
      linkingTrackId = null;
      linkOffsetMs = 0;
      linkSearch = "";
      await loadOverdubs();
    } catch (e: any) {
      alert(e?.message ?? "failed to link track");
    }
  }

  async function voteOverdub(overdubId: string | null) {
    voting = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/overdubs/vote`, {
        overdub_id: overdubId,
      });
      await loadOverdubs();
    } finally {
      voting = false;
    }
  }

  async function deleteOverdub(overdubId: string) {
    deletingOd = true;
    try {
      await apiDelete(`/api/bands/${slug}/tracks/${overdubId}`);

      await loadOverdubs();
    } finally {
      deletingOd = false;
    }
  }

  async function bounceOverdub(overdubId: string, toNewTrack = false) {
    bouncing = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/overdubs/bounce`, {
        overdub_id: overdubId,
        to_new_track: toNewTrack,
      });
    } finally {
      bouncing = false;
    }
  }

  async function adjustOffset(overdubId: string, offsetMs: number) {
    await apiPatch(`/api/bands/${slug}/tracks/${trackId}/overdubs/${overdubId}/offset`, {
      offset_ms: offsetMs,
    });
    const od = overdubs.find((o) => o.id === overdubId);
    if (od) od.offset_ms = offsetMs;
  }

  async function renameTrack(id: string, title: string) {
    await apiPatch(`/api/bands/${slug}/tracks/${id}`, { title });
    if (id === trackId && track) {
      track.title = title;
    } else {
      const od = overdubs.find((o) => o.id === id);
      if (od) od.title = title;
    }
  }

  async function bounceMix(overdubIds: string[], toNew: boolean) {
    bouncing = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/overdubs/bounce-mix`, {
        overdub_ids: overdubIds,
        to_new_track: toNew,
      });
    } finally {
      bouncing = false;
    }
  }

  async function saveGain(id: string, gain: number) {
    await apiPatch(`/api/bands/${slug}/tracks/${id}/gain`, { gain });
  }

  async function scrubOverdubs(keepId?: string) {
    scrubbing = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/overdubs/scrub`, {
        keep_id: keepId || null,
      });
      await loadOverdubs();
    } finally {
      scrubbing = false;
    }
  }

  let restoring = $state(false);
  let confirmRestore = $state(false);

  async function restoreOriginal() {
    restoring = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/restore`, {});
      confirmRestore = false;
      streamVersion++;
      await loadData();
    } finally {
      restoring = false;
    }
  }

  async function uploadOverdubFile() {
    if (!overdubFile || uploadingOverdub) return;
    uploadingOverdub = true;
    try {
      await uploadFile(
        `/api/bands/${slug}/tracks/${trackId}/overdubs`,
        overdubFile,
        { offset_ms: String(uploadOffsetMs) },
      );
      showOverdubUpload = false;
      overdubFile = null;
      uploadOffsetMs = 0;
      await loadOverdubs();
    } catch {
    } finally {
      uploadingOverdub = false;
    }
  }

  // @mention autocomplete
  let mentionQuery = $state("");
  let mentionOpen = $state(false);
  let mentionIndex = $state(0);
  let mentionMatches = $derived(
    mentionQuery
      ? members.filter((m) => {
          const name = (m.user.display_name || m.user.email).toLowerCase();
          return name.includes(mentionQuery.toLowerCase());
        }).slice(0, 5)
      : []
  );

  function handleCommentInput(e: Event) {
    const input = e.target as HTMLInputElement;
    const val = input.value;
    const cursor = input.selectionStart ?? val.length;
    const before = val.slice(0, cursor);
    const match = before.match(/@([\w.\-]*)$/);
    if (match) {
      mentionQuery = match[1];
      mentionOpen = true;
      mentionIndex = 0;
    } else {
      mentionOpen = false;
    }
  }

  function handleCommentKeydown(e: KeyboardEvent) {
    if (!mentionOpen || mentionMatches.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      mentionIndex = (mentionIndex + 1) % mentionMatches.length;
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      mentionIndex = (mentionIndex - 1 + mentionMatches.length) % mentionMatches.length;
    } else if (e.key === "Tab" || e.key === "Enter") {
      if (mentionOpen && mentionMatches.length > 0) {
        e.preventDefault();
        insertMention(mentionMatches[mentionIndex]);
      }
    } else if (e.key === "Escape") {
      mentionOpen = false;
    }
  }

  function insertMention(member: Member) {
    const input = document.getElementById("comment-input") as HTMLInputElement;
    const cursor = input.selectionStart ?? newComment.length;
    const before = newComment.slice(0, cursor);
    const after = newComment.slice(cursor);
    const name = member.user.display_name || member.user.email;
    const replaced = before.replace(/@[\w.\-]*$/, `@${name} `);
    newComment = replaced + after;
    mentionOpen = false;
    input.focus();
  }

  function startEditing() {
    if (!track) return;
    editTitle = track.title;
    editDesc = track.description || "";
    editNotes = track.notes || "";
    editRecordedAt = track.recorded_at ? track.recorded_at.slice(0, 10) : "";
    editing = true;
  }

  async function saveMeta() {
    if (!track || !editTitle.trim()) return;
    savingMeta = true;
    try {
      const updated = await apiPatch<Track>(`/api/bands/${slug}/tracks/${trackId}`, {
        title: editTitle.trim(),
        description: editDesc.trim(),
        notes: editNotes,
        recorded_at: editRecordedAt || "",
      });
      track.title = updated.title;
      track.description = updated.description;
      track.notes = updated.notes;
      track.recorded_at = updated.recorded_at;
      editing = false;
    } finally {
      savingMeta = false;
    }
  }

  async function assignSet(setId: string | null) {
    if (!track) return;
    assigningSet = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/set`, {
        set_id: setId || null,
      });
      track.set_id = setId || undefined;
    } finally {
      assigningSet = false;
    }
  }

  async function addTag() {
    const tag = newTag.trim().toLowerCase();
    if (!tag || !track || track.tags.includes(tag)) { newTag = ""; return; }
    savingTags = true;
    try {
      const updated = await apiPatch<Track>(`/api/bands/${slug}/tracks/${trackId}/tags`, {
        tags: [...track.tags, tag],
      });
      track.tags = updated.tags;
      newTag = "";
    } finally {
      savingTags = false;
    }
  }

  async function removeTag(tag: string) {
    if (!track) return;
    savingTags = true;
    try {
      const updated = await apiPatch<Track>(`/api/bands/${slug}/tracks/${trackId}/tags`, {
        tags: track.tags.filter((t) => t !== tag),
      });
      track.tags = updated.tags;
    } finally {
      savingTags = false;
    }
  }

  async function pickSong(song: Song) {
    if (!track) return;
    assigningSong = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/song`, { song_id: song.id });
      track.song_id = song.id;
      track.song = song;
      songInput = "";
      songDropdownOpen = false;
    } finally {
      assigningSong = false;
    }
  }

  async function createAndAssignSong() {
    if (!track || !songInput.trim()) return;
    assigningSong = true;
    try {
      const song = await apiPost<Song>(`/api/bands/${slug}/songs`, { name: songInput.trim() });
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/song`, { song_id: song.id });
      songs = [...songs, song];
      track.song_id = song.id;
      track.song = song;
      songInput = "";
      songDropdownOpen = false;
    } finally {
      assigningSong = false;
    }
  }

  async function unassignSong() {
    if (!track) return;
    assigningSong = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/song`, { song_id: null });
      track.song_id = undefined;
      track.song = undefined;
    } finally {
      assigningSong = false;
    }
  }

  async function addPersonnel() {
    if (!addPersonnelId || !track) return;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/personnel`, {
        user_id: addPersonnelId,
        role: addPersonnelRole.trim(),
      });
      personnel = await api<Personnel[]>(`/api/bands/${slug}/tracks/${trackId}/personnel`);
      addPersonnelId = "";
      addPersonnelRole = "";
    } catch {}
  }

  async function removePersonnel(userId: string) {
    if (!track) return;
    await apiDelete(`/api/bands/${slug}/tracks/${trackId}/personnel/${userId}`);
    personnel = personnel.filter((p) => p.user_id !== userId);
  }

  onMount(() => layoutWidth.set('workspace'));
  onDestroy(() => layoutWidth.set('index'));
</script>

{#if loading}
  <div class="flex items-center justify-center gap-2 py-20 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
{:else if track}
  <div>
    <!-- Breadcrumb -->
    <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5 mb-6">
      <button onclick={() => navigate(`/band/${slug}`)} class="hover:text-accent/60 transition-colors py-1">{slug.toUpperCase()}</button>
      <span class="text-text-muted/20">&rsaquo;</span>
      <span class="text-text-muted/50">{track.title.toUpperCase()}</span>
    </div>

    {#if editing}
      <!-- Full-width edit form -->
      <div class="bg-bg-surface border border-border p-6 space-y-5">
        <div>
          <span class="label text-text-muted block mb-2">title</span>
          <input bind:value={editTitle} class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors" />
        </div>
        <div>
          <span class="label text-text-muted block mb-2">description</span>
          <input bind:value={editDesc} placeholder="Short description..." class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors" />
        </div>
        <div>
          <span class="label text-text-muted block mb-2">notes / lyrics / tabs</span>
          <textarea bind:value={editNotes} rows="8" placeholder="Paste lyrics, chord charts, tabs, session notes..." class="w-full bg-bg-primary border border-border px-4 py-3 text-sm font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y"></textarea>
        </div>
        <div>
          <span class="label text-text-muted block mb-2">recording date</span>
          <input bind:value={editRecordedAt} type="date" class="bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors" />
        </div>
        <div class="flex gap-4">
          <button onclick={saveMeta} disabled={savingMeta || !editTitle.trim()} class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors">{savingMeta ? "..." : "save"}</button>
          <button onclick={() => (editing = false)} class="px-6 py-3 label text-text-muted hover:text-text-secondary transition-colors">cancel</button>
        </div>
      </div>
    {:else}
      <!-- Desktop 2-col layout: main + rail -->
      <div class="md:grid md:grid-cols-[minmax(0,1fr)_220px] lg:grid-cols-[minmax(0,1fr)_232px] md:gap-6 lg:gap-8 md:items-start">

        <!-- MAIN COLUMN -->
        <div class="min-w-0">

          <!-- Title + meta -->
          <div class="sys-title-block">
            <div class="flex items-center gap-2 flex-wrap mb-3">
              <span class="sys-kicker">track console // review + overdub</span>
              {#if track.song}
                <span class="sys-chip-accent sys-code">song linked</span>
              {/if}
              {#if track.overdub_of}
                <span class="sys-chip sys-code text-text-muted/70">overdub node</span>
              {/if}
            </div>
            <h2 class="text-4xl md:text-[3.55rem] leading-none font-bold tracking-[0.05em] font-display uppercase {highlight ? 'animate-highlight' : ''}">{track.title}</h2>
            {#if track.description}
              <p class="text-base font-medium text-text-secondary mt-2">{track.description}</p>
            {/if}
            <div class="sys-panel-muted flex items-center gap-3 mt-4 px-4 py-3 label-sm text-text-muted flex-wrap">
              <span class="font-mono">{formatDuration(track.duration_ms)}</span>
              <span class="vr-divider">/</span>
              <span>{formatFileSize(track.file_size)}</span>
              <span class="vr-divider">/</span>
              <span>{track.uploader?.display_name || track.uploader?.email}</span>
              <span class="vr-divider">/</span>
              <span>{formatRelativeTime(track.created_at)}</span>
              <span class="vr-divider">/</span>
              <a href={`/api/bands/${slug}/tracks/${trackId}/stream?dl=1`} class="text-accent hover:text-accent-hover transition-colors">download</a>
            </div>
          </div>

          <!-- Player — skip for RPP session tracks (no audio on parent) -->
          {#if !isRppSession}
            {#if track.status === "ready"}
              <div class="{overdubs.length > 0 ? '' : 'mb-8'}">
                <div class="sys-section-head border-b-0">
                  <span class="sys-kicker">PLAYBACK // waveform + markers</span>
                  <span class="sys-code text-text-muted/45">{timedComments.length} markers</span>
                </div>
                {#key streamVersion}
                  <WaveformPlayer
                    bind:this={playerRef}
                    src={streamUrl}
                    peaks={track.waveform_data}
                    duration={track.duration_ms}
                    comments={timedComments}
                    onTimestampClick={handleTimestampClick}
                    onCommentClick={scrollToComment}
                    seamlessBottom={overdubs.length > 0}
                    externalPositionMs={overdubs.length > 0 ? mixerPositionMs : undefined}
                    externalPlaying={overdubs.length > 0 ? mixerPlaying : undefined}
                    onSeekRequest={overdubs.length > 0 ? (ms) => mixerRef?.seekTo(ms) : undefined}
                    onPlay={overdubs.length > 0 ? () => mixerRef?.playTrack() : undefined}
                    onPause={overdubs.length > 0 ? () => mixerRef?.pauseTrack() : undefined}
                  />
                {/key}
              </div>
            {:else if track.status === "processing"}
              <div class="bg-bg-surface border border-border p-12 text-center mb-8">
                <div class="flex items-center justify-center gap-3 text-accent">
                  <div class="w-2 h-2 bg-accent animate-pulse"></div>
                  <span class="label">processing</span>
                </div>
              </div>
            {:else}
              <div class="bg-bg-surface border border-border p-12 text-center mb-8 text-danger label">processing failed</div>
            {/if}
          {/if}

          <!-- Overdubs / RPP session mixer -->
          {#if !track.overdub_of}
            <div class="mb-8">

              <!-- RPP session header -->
              {#if isRppSession}
                <div class="flex items-center gap-4 px-3 py-2 border border-border bg-bg-surface mb-px">
                  <span class="label-sm font-mono text-accent/60 tracking-widest">REAPER SESSION</span>
                  <span class="label-sm text-text-muted/50">{track.rpp_session_name}</span>
                  <div class="flex items-center gap-3 ml-auto">
                    <a href={`/api/bands/${slug}/sync/rpp/${trackId}`} class="label-sm text-text-muted hover:text-accent transition-colors">download .rpp</a>
                    <a href={`/api/bands/${slug}/sync/binary`} class="label-sm text-text-muted hover:text-accent transition-colors" title="Download the sync utility — run it in your project folder to push/pull this session">download sync utility</a>
                  </div>
                </div>
              {/if}

              <!-- Mixer — always shown for RPP sessions, otherwise only when overdubs exist -->
              {#if overdubs.length > 0 || isRppSession}
                <div class="sys-section-head border-b-0 mt-4">
                  <span class="sys-kicker">MIX // lanes + offsets</span>
                  <span class="sys-code text-text-muted/45">{mixerTracks.length} lanes</span>
                </div>
                <MultiTrackMixer
                  bind:this={mixerRef}
                  tracks={mixerTracks}
                  {isAdmin}
                  bandSlug={slug}
                  parentTrackId={trackId}
                  hideSrc={isRppSession}
                  seamlessTop={!isRppSession}
                  onPositionChange={(ms) => (mixerPositionMs = ms)}
                  onPlayingChange={(p) => (mixerPlaying = p)}
                  onOffsetChange={(id, ms) => adjustOffset(id, ms)}
                  onRename={(id, title) => renameTrack(id, title)}
                  onDelete={(id) => deleteOverdub(id)}
                  onVote={(id) => voteOverdub(id)}
                  onBounceMix={(ids, toNew) => bounceMix(ids, toNew)}
                  onRefresh={() => loadOverdubs()}
                  onGainChange={(id, gain) => saveGain(id, gain)}
                />
                {#if isAdmin && overdubs.length > 1}
                  <button onclick={() => scrubOverdubs()} disabled={scrubbing} class="label-sm text-red-400/60 hover:text-red-400 transition-colors mt-3">{scrubbing ? "scrubbing..." : "scrub all overdubs"}</button>
                {/if}
              {/if}

              <!-- Overdub actions module -->
              <div class="mt-3">
                {#if activeOverdubMode}
                  <div class="sys-mode-bar mb-3">
                    <div class="min-w-0">
                      <div class="flex items-center gap-3 flex-wrap">
                        <span class="sys-code text-accent">{activeOverdubMode.code}</span>
                        <span class="label-sm text-accent">overdub mode</span>
                      </div>
                      <div class="sys-stat-value text-accent mt-2">{activeOverdubMode.title}</div>
                      <p class="label-sm text-text-secondary mt-2">{activeOverdubMode.detail}</p>
                    </div>
                    <button
                      onclick={() => { showOverdubUpload = false; showOverdubRecord = false; showLinkTrack = false; }}
                      class="label-sm text-text-muted hover:text-text-primary transition-colors shrink-0"
                    >
                      exit mode
                    </button>
                  </div>
                {/if}

                <!-- Mobile: collapsed by default -->
                <button
                  class="md:hidden w-full flex items-center justify-between px-4 py-2.5 border border-border bg-bg-surface label-sm text-text-muted hover:text-text-secondary transition-colors"
                  onclick={() => (showRecordingControls = !showRecordingControls)}
                >
                  <span>add overdub</span>
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" class="transition-transform {showRecordingControls ? 'rotate-180' : ''}">
                    <polyline points="6 9 12 15 18 9"/>
                  </svg>
                </button>

                <!-- Always visible on desktop, toggled on mobile -->
                <div class="{showRecordingControls ? 'block' : 'hidden'} md:block">
                  <div class="flex items-center gap-4 flex-wrap border border-border bg-bg-surface px-4 py-2.5">
                    <div class="flex items-center gap-2">
                      <button onclick={calibrate} disabled={calibrating} class="label-sm text-text-muted hover:text-accent transition-colors">{calibrating ? "calibrating..." : "calibrate"}</button>
                      <input
                        type="number"
                        step="1"
                        value={calibratedLatency ?? 0}
                        onchange={(e) => {
                          const val = parseInt((e.target as HTMLInputElement).value);
                          if (!isNaN(val)) {
                            calibratedLatency = val;
                            localStorage.setItem("overdub-latency-ms", String(val));
                          }
                        }}
                        class="w-16 bg-bg-primary border border-border px-2 py-1 label-sm font-mono text-text-secondary text-right focus:outline-none focus:border-accent transition-colors"
                      />
                      <span class="label-sm text-text-muted">ms</span>
                    </div>
                    <div class="w-px h-4 bg-border/50 hidden sm:block"></div>
                    <button
                      onclick={() => { showOverdubUpload = !showOverdubUpload; if (showOverdubUpload) { showOverdubRecord = false; showLinkTrack = false; } }}
                      class="label-sm text-text-muted hover:text-accent transition-colors"
                    >{showOverdubUpload ? "cancel" : "upload"}</button>
                    <button
                      onclick={() => { showOverdubRecord = !showOverdubRecord; if (showOverdubRecord) { showOverdubUpload = false; showLinkTrack = false; } }}
                      class="label-sm text-text-muted hover:text-accent transition-colors flex items-center gap-1.5"
                    >
                      <div class="w-2 h-2 rounded-full bg-red-400/60"></div>
                      {showOverdubRecord ? "cancel" : "record"}
                    </button>
                    <button
                      onclick={() => { if (showLinkTrack) { showLinkTrack = false; } else { openLinkTrack(); } }}
                      class="label-sm text-text-muted hover:text-accent transition-colors"
                    >{showLinkTrack ? "cancel" : "link existing"}</button>
                  </div>

                  {#if calibrationMsg}
                    <p class="label-sm text-text-muted mt-2">{calibrationMsg}</p>
                  {/if}

                  {#if showOverdubUpload}
                    <div class="mt-3 bg-bg-surface border border-border p-4 space-y-3">
                      <p class="label-sm text-text-muted">upload a pre-recorded overdub file. set the offset to where it aligns in the parent track.</p>
                      <div class="flex items-center gap-4 flex-wrap">
                        <input type="file" accept="audio/*" onchange={(e) => overdubFile = (e.target as HTMLInputElement).files?.[0] ?? null} class="label-sm text-text-secondary file:bg-bg-primary file:border file:border-border file:px-3 file:py-1.5 file:text-text-secondary file:label-sm file:mr-3 file:cursor-pointer" />
                        <div class="flex items-center gap-2">
                          <span class="label-sm text-text-muted">offset</span>
                          <input type="number" step="100" bind:value={uploadOffsetMs} class="w-24 bg-bg-primary border border-border px-2 py-1 label-sm font-mono text-text-secondary text-right focus:outline-none focus:border-accent transition-colors" />
                          <span class="label-sm text-text-muted">ms</span>
                        </div>
                        <button onclick={() => { uploadOffsetMs = mixerPositionMs || (playerRef?.getCurrentTimeMs() ?? 0); }} class="label-sm text-accent hover:text-accent-hover transition-colors">use player position</button>
                        <button onclick={uploadOverdubFile} disabled={!overdubFile || uploadingOverdub} class="px-4 py-1.5 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors">{uploadingOverdub ? "uploading..." : "upload"}</button>
                      </div>
                    </div>
                  {/if}

                  {#if showLinkTrack}
                    <div class="mt-3 bg-bg-surface border border-border p-4 space-y-3">
                      <p class="label-sm text-text-muted">link an existing track as an overdub. the original track stays in the track list — this creates a shared reference.</p>
                      <input type="text" placeholder="filter tracks..." bind:value={linkSearch} class="w-full bg-bg-primary border border-border px-3 py-1.5 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors" />
                      <div class="max-h-48 overflow-y-auto space-y-1">
                        {#each linkableTracks.filter(t => t.id !== trackId && !t.overdub_of && t.title.toLowerCase().includes(linkSearch.toLowerCase())) as t}
                          <button onclick={() => linkingTrackId = linkingTrackId === t.id ? null : t.id} class="w-full text-left px-3 py-2 label-sm transition-colors {linkingTrackId === t.id ? 'bg-accent text-bg-primary' : 'hover:bg-bg-primary text-text-secondary'}">
                            <span class="font-semibold">{t.title}</span>
                            {#if t.recorded_at}<span class="text-xs opacity-60 ml-2">{new Date(t.recorded_at).toLocaleDateString()}</span>{/if}
                          </button>
                        {/each}
                        {#if linkableTracks.filter(t => t.id !== trackId && !t.overdub_of && t.title.toLowerCase().includes(linkSearch.toLowerCase())).length === 0}
                          <p class="label-sm text-text-muted px-3 py-2">no tracks found</p>
                        {/if}
                      </div>
                      <div class="flex items-center gap-3 flex-wrap">
                        <div class="flex items-center gap-2">
                          <span class="label-sm text-text-muted">offset</span>
                          <input type="number" step="100" bind:value={linkOffsetMs} class="w-24 bg-bg-primary border border-border px-2 py-1 label-sm font-mono text-text-secondary text-right focus:outline-none focus:border-accent transition-colors" />
                          <span class="label-sm text-text-muted">ms</span>
                        </div>
                        <button onclick={() => { linkOffsetMs = mixerPositionMs || (playerRef?.getCurrentTimeMs() ?? 0); }} class="label-sm text-accent hover:text-accent-hover transition-colors">use player position</button>
                        <button onclick={linkTrack} disabled={!linkingTrackId} class="px-4 py-1.5 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors">link</button>
                      </div>
                    </div>
                  {/if}

                  {#if showOverdubRecord}
                    <div class="mt-3 bg-bg-surface border border-border p-4 space-y-3">
                      <div class="flex items-center gap-4 flex-wrap">
                        <span class="label-sm text-text-muted">punch in at:</span>
                        <div class="flex items-center gap-2">
                          <input type="number" step="100" bind:value={punchInMs} class="w-24 bg-bg-primary border border-border px-2 py-1 label-sm font-mono text-text-secondary text-right focus:outline-none focus:border-accent transition-colors" />
                          <span class="label-sm text-text-muted">ms</span>
                        </div>
                        <button onclick={() => { punchInMs = mixerPositionMs || (playerRef?.getCurrentTimeMs() ?? 0); }} class="label-sm text-accent hover:text-accent-hover transition-colors">use player position</button>
                        {#if punchInMs > 0}
                          <button onclick={() => punchInMs = 0} class="label-sm text-text-muted hover:text-danger transition-colors">reset</button>
                          <span class="label-sm text-text-muted font-mono">starts at {formatDuration(punchInMs)}</span>
                        {/if}
                      </div>
                      <p class="label-sm text-text-muted">headphones on — parent plays from {punchInMs > 0 ? formatDuration(punchInMs) : "the start"} when you hit record{calibratedLatency != null ? ` (${calibratedLatency}ms compensation)` : ""}</p>
                      <Recorder
                        bandSlug={slug}
                        onRecorded={() => { showOverdubRecord = false; loadOverdubs(); }}
                        overdubParentId={trackId}
                        parentStreamUrl={streamUrl}
                        latencyCompensation={calibratedLatency ?? 0}
                        {punchInMs}
                        bpm={track?.song?.bpm ?? 0}
                      />
                    </div>
                  {/if}
                </div>
              </div>

            </div>
          {/if}

          <!-- Comment input -->
          <div class="mb-8 border-t border-border/35 pt-6">
            <div class="sys-section-head mb-4">
              <span class="sys-kicker">REVIEW // comments + timestamps</span>
              <span class="sys-code text-text-muted/45">{comments.length} entries</span>
            </div>
            <form onsubmit={(e) => { e.preventDefault(); if (!mentionOpen) submitComment(); }} class="flex gap-3">
              <div class="flex-1 relative">
                {#if commentTimestamp != null}
                  <button type="button" onclick={clearTimestamp} class="absolute left-4 top-1/2 -translate-y-1/2 label-sm text-accent bg-accent/10 px-2 py-1 font-mono hover:bg-accent/20 transition-colors">
                    @{Math.floor(commentTimestamp / 1000)}s x
                  </button>
                {/if}
                <input
                  id="comment-input"
                  bind:value={newComment}
                  type="text"
                  placeholder={commentTimestamp != null ? "" : "add a comment... (use @ to mention)"}
                  oninput={handleCommentInput}
                  onkeydown={handleCommentKeydown}
                  class="w-full bg-bg-surface border border-border px-5 py-4 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
                  style:padding-left={commentTimestamp != null ? "6rem" : undefined}
                />
                {#if mentionOpen && mentionMatches.length > 0}
                  <div class="absolute bottom-full left-0 mb-1 w-64 bg-bg-surface border border-border z-10 max-h-48 overflow-y-auto">
                    {#each mentionMatches as member, i}
                      <button type="button" onmousedown={() => insertMention(member)} class="block w-full text-left px-4 py-2 label-sm transition-colors {i === mentionIndex ? 'bg-accent/10 text-accent' : 'text-text-secondary hover:bg-accent/10 hover:text-accent'}">
                        {member.user.display_name || member.user.email}
                      </button>
                    {/each}
                  </div>
                {/if}
              </div>
              <button type="submit" disabled={posting || !newComment.trim()} class="px-6 py-4 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors">post</button>
            </form>
            <div class="label-sm text-text-muted mt-3">click comment button or alt+click waveform to attach timestamp · type @ to mention</div>
          </div>

          <!-- Comments -->
          <div class="mb-8">
            <h3 class="label text-text-secondary mb-6">comments ({comments.length})</h3>
            <CommentList {comments} {trackId} onSeek={handleSeek} onRefresh={loadComments} />
          </div>

          <!-- Notes -->
          {#if track.notes}
            <div class="mb-8">
              <h3 class="label text-text-secondary mb-4">notes</h3>
              <pre class="bg-bg-surface border border-border p-6 text-sm font-mono text-text-secondary whitespace-pre-wrap leading-relaxed">{track.notes}</pre>
            </div>
          {/if}

        </div>

        <!-- RAIL COLUMN -->
        <aside class="min-w-0 mt-10 md:mt-0 space-y-5 border-t border-border pt-8 md:border-t-0 md:pt-0 md:sticky md:top-4 md:border-l md:border-border/35 md:pl-6">

          <div class="sys-panel relative overflow-hidden p-4">
            <span class="sys-edge-label">META</span>
            <div class="sys-code text-text-muted/45">TME // duration</div>
            <div class="sys-stat-value sys-stat-value-compact text-accent mt-2">{track ? formatDuration(track.duration_ms) : "--:--"}</div>
            <div class="sys-stat-grid mt-4 grid-cols-2">
              <div class="sys-stat">
                <div class="sys-code text-text-muted/45">MRK</div>
                <div class="sys-stat-value sys-stat-value-compact mt-2">{String(timedComments.length).padStart(2, "0")}</div>
              </div>
              <div class="sys-stat">
                <div class="sys-code text-text-muted/45">LAN</div>
                <div class="sys-stat-value sys-stat-value-compact mt-2">{String(mixerTracks.length).padStart(2, "0")}</div>
              </div>
            </div>
          </div>

          <!-- Edit + Delete -->
          <div class="flex items-center gap-4">
            <button onclick={startEditing} class="label text-text-muted hover:text-accent transition-colors">edit</button>
            {#if !confirmDelete}
              <button onclick={() => (confirmDelete = true)} class="label text-text-muted hover:text-danger transition-colors">delete</button>
            {:else}
              <span class="flex items-center gap-2">
                <span class="label-sm text-danger">sure?</span>
                <button onclick={deleteTrack} disabled={deleting} class="label-sm text-danger hover:text-red-300 transition-colors">{deleting ? "..." : "yes"}</button>
                <button onclick={() => (confirmDelete = false)} class="label-sm text-text-muted hover:text-text-secondary transition-colors">no</button>
              </span>
            {/if}
          </div>

          <!-- Tags -->
          <div>
            <div class="label-sm text-text-muted/50 tracking-widest mb-2">tags</div>
            <div class="flex items-center gap-2 flex-wrap">
              {#each track.tags as tag}
                <button onclick={() => removeTag(tag)} disabled={$isDemo} title={$isDemo ? "demo account is read-only" : "Remove tag"} class="label-sm text-accent bg-accent/10 px-3 py-1 hover:bg-danger/20 hover:text-danger disabled:opacity-50 disabled:cursor-not-allowed transition-colors">{tag} x</button>
              {/each}
              <form onsubmit={(e) => { e.preventDefault(); addTag(); }} class="flex">
                <input bind:value={newTag} type="text" placeholder={$isDemo ? "demo" : "+ tag"} disabled={savingTags || $isDemo} title={$isDemo ? "demo account is read-only" : ""} class="bg-transparent border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-24 disabled:opacity-50" />
              </form>
            </div>
          </div>

          <!-- Song assignment -->
          <div>
            <div class="label-sm text-text-muted/50 tracking-widest mb-2">song</div>
            {#if track.song}
              <div class="flex items-center gap-2">
                <span class="label-sm text-accent bg-accent/10 px-3 py-1">{track.song.name}</span>
                <button onclick={unassignSong} disabled={assigningSong} class="label-sm text-text-muted hover:text-danger transition-colors">x</button>
              </div>
            {:else}
              <div class="relative">
                <input
                  type="text"
                  bind:value={songInput}
                  onfocus={() => (songDropdownOpen = true)}
                  onblur={() => setTimeout(() => (songDropdownOpen = false), 150)}
                  placeholder="type to search or create..."
                  disabled={assigningSong}
                  class="w-full bg-bg-surface border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
                />
                {#if songDropdownOpen && (filteredSongs.length > 0 || songInput.trim())}
                  <div class="absolute top-full left-0 mt-1 w-full bg-bg-surface border border-border z-10 max-h-48 overflow-y-auto">
                    {#each filteredSongs as song}
                      <button type="button" onmousedown={() => pickSong(song)} class="block w-full text-left px-3 py-2 label-sm text-text-secondary hover:bg-accent/10 hover:text-accent transition-colors">{song.name}</button>
                    {/each}
                    {#if songInput.trim() && !songs.some((s) => s.name.toLowerCase() === songInput.trim().toLowerCase())}
                      <button type="button" onmousedown={createAndAssignSong} class="block w-full text-left px-3 py-2 label-sm text-accent hover:bg-accent/10 transition-colors border-t border-border">+ create "{songInput.trim()}"</button>
                    {/if}
                  </div>
                {/if}
              </div>
            {/if}
          </div>

          <!-- Set assignment -->
          <div>
            <div class="label-sm text-text-muted/50 tracking-widest mb-2">set</div>
            {#if track.set_id}
              {@const currentSet = allSets.find(s => s.id === track!.set_id)}
              {#if currentSet}
                <div class="flex items-center gap-2">
                  <button onclick={() => navigate(`/band/${slug}/set/${currentSet.id}`)} class="label-sm text-accent bg-accent/10 px-3 py-1 hover:bg-accent/20 transition-colors">{currentSet.name}</button>
                  <button onclick={() => assignSet(null)} disabled={assigningSet} class="label-sm text-text-muted hover:text-danger transition-colors">x</button>
                </div>
              {/if}
            {:else}
              <select
                onchange={(e) => { const val = (e.target as HTMLSelectElement).value; if (val) assignSet(val); }}
                disabled={assigningSet}
                class="w-full bg-bg-surface border border-border px-2 py-1 label-sm text-text-secondary focus:outline-none focus:border-accent transition-colors"
              >
                <option value="">assign to set...</option>
                {#each allSets as set}
                  <option value={set.id}>{set.name}</option>
                {/each}
              </select>
            {/if}
          </div>

          <!-- Personnel -->
          <div>
            <div class="label-sm text-text-muted/50 tracking-widest mb-2">personnel</div>
            <div class="space-y-1.5">
              {#each personnel as p}
                <div class="flex items-center gap-2">
                  <span class="label-sm text-text-secondary">{p.user.display_name || p.user.email}</span>
                  {#if p.role}<span class="label-sm text-text-muted">/ {p.role}</span>{/if}
                  <button onclick={() => removePersonnel(p.user_id)} class="label-sm text-text-muted hover:text-danger transition-colors ml-auto">x</button>
                </div>
              {/each}
              <form onsubmit={(e) => { e.preventDefault(); addPersonnel(); }} class="flex items-center gap-2 mt-2">
                <select bind:value={addPersonnelId} disabled={$isDemo} title={$isDemo ? "demo account is read-only" : ""} class="flex-1 min-w-0 bg-bg-surface border border-border px-2 py-1 label-sm text-text-secondary focus:outline-none focus:border-accent transition-colors disabled:opacity-50">
                  <option value="">{$isDemo ? "demo: read-only" : "+ add"}</option>
                  {#each members.filter((m) => !personnel.some((p) => p.user_id === m.user.id)) as member}
                    <option value={member.user.id}>{member.user.display_name || member.user.email}</option>
                  {/each}
                </select>
                {#if addPersonnelId}
                  <input bind:value={addPersonnelRole} type="text" placeholder="role..." class="flex-1 min-w-0 bg-bg-surface border border-border px-2 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors" />
                  <button type="submit" disabled={$isDemo} title={$isDemo ? "demo account is read-only" : ""} class="label-sm text-accent hover:text-accent-hover disabled:opacity-50 transition-colors shrink-0">add</button>
                {/if}
              </form>
            </div>
          </div>

          <!-- Source URL -->
          {#if track.source_url}
            <div>
              <div class="label-sm text-text-muted/50 tracking-widest mb-1">source</div>
              <a href={track.source_url} target="_blank" rel="noopener" class="label-sm text-accent hover:text-accent-hover transition-colors break-all">{track.source_url}</a>
            </div>
          {/if}

          <!-- Recording date -->
          {#if track.recorded_at}
            <div>
              <div class="label-sm text-text-muted/50 tracking-widest mb-1">recorded</div>
              <span class="label-sm text-text-secondary">{new Date(track.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
            </div>
          {/if}

          <!-- Bounce status -->
          {#if track.pre_bounce_id}
            <div>
              <div class="label-sm text-text-muted/50 tracking-widest mb-1">bounce</div>
              <div class="flex items-center gap-2 flex-wrap">
                <button onclick={() => navigate(`/band/${slug}/track/${track!.pre_bounce_id}`)} class="label-sm text-accent hover:text-accent-hover transition-colors">view original</button>
                {#if isAdmin}
                  {#if !confirmRestore}
                    <button onclick={() => confirmRestore = true} class="label-sm text-text-muted hover:text-danger transition-colors">restore</button>
                  {:else}
                    <span class="flex items-center gap-2">
                      <span class="label-sm text-danger">undo?</span>
                      <button onclick={restoreOriginal} disabled={restoring} class="label-sm text-danger hover:text-red-300 transition-colors">{restoring ? "..." : "yes"}</button>
                      <button onclick={() => confirmRestore = false} class="label-sm text-text-muted hover:text-text-secondary transition-colors">no</button>
                    </span>
                  {/if}
                {/if}
              </div>
            </div>
          {/if}

        </aside>

      </div>
    {/if}
  </div>
{/if}
