<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { marked } from "marked";
  import { api, apiDelete, apiPatch, apiPost } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { player } from "../lib/stores/player";
  import { formatDuration, formatRelativeTime, formatTimestamp, setTypeCode, setTypeLabel } from "../lib/utils/format";
  import { sanitizeHtml } from "../lib/utils/sanitize";

  // marked.parse emits raw HTML — route every render through sanitizeHtml
  // so stored <script>/on*=/javascript: payloads can't execute in other
  // members' sessions.
  const renderMd = (src: string) => sanitizeHtml(marked.parse(src ?? "") as string);
  import CommentList from "../lib/components/CommentList.svelte";
  import Recorder from "../lib/components/Recorder.svelte";
  import TrackUpload from "../lib/components/TrackUpload.svelte";
  import { globalPlayer, playerState } from "../lib/stores/globalPlayer";
  import ChordProLyrics from "../lib/components/ChordProLyrics.svelte";

  marked.setOptions({ breaks: true, gfm: true });

  let { slug, songId }: { slug: string; songId: string } = $props();

  type Song = {
    id: string;
    name: string;
    lyrics: string;
    tabs: string;
    notes: string;
    bpm: number;
    created_at: string;
  };

  type Track = {
    id: string;
    title: string;
    description?: string;
    duration_ms: number;
    format: string;
    file_size: number;
    status: string;
    tags: string[];
    source_url?: string;
    song_id?: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
    personnel?: Array<{ user: { display_name: string; email: string }; role: string }>;
  };

  type SetTake = {
    set_item_id: string;
    set_id: string;
    set_name: string;
    set_type: string;
    start_ms: number;
    end_ms: number;
    track_id: string;
    track_title: string;
    waveform_data?: number[];
    duration_ms: number;
    format: string;
    file_size: number;
    tags: string[];
    recorded_at?: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
  };

  type SetSummary = {
    id: string;
    name: string;
    set_type: string;
    recorded_at?: string;
    item_count: number;
  };

  type Comment = {
    id: string;
    body: string;
    timestamp_ms: number | null;
    created_at: string;
    user: { id: string; display_name: string; email: string; avatar_url?: string };
    replies?: Comment[];
  };

  let song = $state<Song | null>(null);
  let tracks = $state<Track[]>([]);
  let setTakes = $state<SetTake[]>([]);
  let songSets = $state<SetSummary[]>([]);

  // Deduplicate: hide direct takes that also appear as set takes
  let directTracks = $derived(
    tracks.filter((t) => !setTakes.some((st) => st.track_id === t.id))
  );

  // Comments for set takes
  let expandedTakeId = $state<string | null>(null);
  let trackComments = $state<Map<string, Comment[]>>(new Map());
  let takeCommentInput = $state("");
  let postingComment = $state(false);

  let editingLyrics = $state(false);
  let editingTabs = $state(false);
  let editingNotes = $state(false);
  let lyricsInput = $state("");
  let tabsInput = $state("");
  let notesInput = $state("");
  let saving = $state(false);

  let editingTitle = $state(false);
  let titleInput = $state("");
  let savingTitle = $state(false);
  let showChords = $state(true);
  let expandedSection = $state<"notes" | "lyrics" | "tabs" | null>(null);

  // Dynamic section numbers (avoid skipping 02 when no sets)
  let notesNum = $derived(songSets.length > 0 ? '03' : '02');
  let lyricsNum = $derived(songSets.length > 0 ? '04' : '03');
  let tabsNum = $derived(songSets.length > 0 ? '05' : '04');

  // BPM / tap tempo
  let bpmInput = $state(0);
  let savingBpm = $state(false);
  let tapTimes: number[] = [];

  // Delete
  let confirmDelete = $state(false);
  let deleting = $state(false);

  async function deleteSong() {
    deleting = true;
    try {
      await apiDelete(`/api/bands/${slug}/songs/${songId}`);
      navigate(`/band/${slug}`);
    } finally {
      deleting = false;
    }
  }
  function handleTap() {
    const now = performance.now();
    // Reset if gap > 2 seconds
    if (tapTimes.length > 0 && now - tapTimes[tapTimes.length - 1] > 2000) {
      tapTimes = [];
    }
    tapTimes.push(now);
    if (tapTimes.length < 2) return;
    // Keep last 8 taps
    if (tapTimes.length > 8) tapTimes = tapTimes.slice(-8);
    // Average interval
    let sum = 0;
    for (let i = 1; i < tapTimes.length; i++) {
      sum += tapTimes[i] - tapTimes[i - 1];
    }
    const avgMs = sum / (tapTimes.length - 1);
    bpmInput = Math.round(60000 / avgMs);
  }

  async function saveBpm() {
    if (!song || bpmInput < 0) return;
    savingBpm = true;
    try {
      song = await apiPatch<Song>(`/api/bands/${slug}/songs/${songId}`, {
        bpm: bpmInput,
      });
    } finally {
      savingBpm = false;
    }
  }

  function hasChordPro(text: string): boolean {
    return /\[[A-G][^\]]*\]/.test(text);
  }

  async function saveTitle() {
    if (!song || !titleInput.trim()) return;
    savingTitle = true;
    try {
      song = await apiPatch<Song>(`/api/bands/${slug}/songs/${songId}`, {
        name: titleInput.trim(),
      });
      editingTitle = false;
    } finally {
      savingTitle = false;
    }
  }

  // playingKey uniquely identifies what's playing: trackId for direct, setItemId for set takes
  let playingKey = $state<string | null>(null);
  let playingTrackId = $state<string | null>(null);
  let audioEl = $state<HTMLAudioElement | null>(null);
  let currentTime = $state(0);
  let audioDuration = $state(0);
  // Constrained playback bounds (for set takes)
  let playStartSec = $state(0);
  let playEndSec = $state(0);
  let pendingSeekSec = $state<number | null>(null);

  // Unified playlist for next/prev navigation
  let allPlayable = $derived([
    ...directTracks
      .filter((t) => t.status === "ready")
      .map((t) => ({ type: "direct" as const, key: t.id, trackId: t.id, startMs: 0, endMs: 0 })),
    ...setTakes.map((t) => ({
      type: "set" as const,
      key: t.set_item_id,
      trackId: t.track_id,
      startMs: t.start_ms,
      endMs: t.end_ms,
    })),
  ]);

  function playByIndex(idx: number) {
    if (idx < 0 || idx >= allPlayable.length) return;
    const item = allPlayable[idx];
    if (item.type === "direct") {
      togglePlay(item.trackId);
    } else {
      togglePlay(item.trackId, item.key, item.startMs, item.endMs);
    }
  }

  function playNext() {
    const idx = playingKey ? allPlayable.findIndex((p) => p.key === playingKey) : -1;
    playByIndex(idx + 1);
  }

  function playPrev() {
    const idx = playingKey ? allPlayable.findIndex((p) => p.key === playingKey) : -1;
    playByIndex(idx - 1);
  }

  function togglePlay(trackId: string, key?: string, startMs?: number, endMs?: number) {
    const k = key || trackId;
    const isSetTake = startMs != null && startMs > 0;

    // For simple direct tracks, use the persistent global player
    if (!isSetTake) {
      const track = directTracks.find((t) => t.id === trackId);
      if (track) {
        // Stop local set-take audio first
        audioEl?.pause();
        globalPlayer.play({
          id: track.id,
          title: track.title,
          bandSlug: slug,
          songName: song?.name,
          durationMs: track.duration_ms,
        });
        // Keep local state in sync for UI
        if (playingKey === k) {
          playingKey = null;
          playingTrackId = null;
        } else {
          playingKey = k;
          playingTrackId = trackId;
        }
        return;
      }
    }

    // Set takes with time boundaries — use local audio
    // Pause the global player first
    const globalAudio = globalPlayer.getAudioElement();
    if (globalAudio && !globalAudio.paused) globalAudio.pause();

    if (playingKey === k) {
      if (audioEl?.paused) {
        audioEl.play();
      } else {
        audioEl?.pause();
      }
      return;
    }
    audioEl?.pause();
    playingKey = k;
    playingTrackId = trackId;
    playStartSec = startMs != null ? startMs / 1000 : 0;
    playEndSec = endMs != null ? endMs / 1000 : 0;
    currentTime = 0;
    audioDuration = 0;
  }

  function onTimeUpdate(e: Event) {
    const el = e.target as HTMLAudioElement;
    currentTime = el.currentTime;
    audioDuration = el.duration || 0;
    // Stop at end boundary for constrained playback
    if (playEndSec > 0 && el.currentTime >= playEndSec) {
      el.pause();
      // Auto-advance to next take
      const idx = playingKey ? allPlayable.findIndex((p) => p.key === playingKey) : -1;
      if (idx >= 0 && idx < allPlayable.length - 1) {
        playByIndex(idx + 1);
      } else {
        playingKey = null;
        playingTrackId = null;
        currentTime = 0;
      }
    }
  }

  function onLoadedMetadata() {
    if (!audioEl) return;
    if (pendingSeekSec != null) {
      audioEl.currentTime = pendingSeekSec;
      pendingSeekSec = null;
    } else if (playStartSec > 0) {
      audioEl.currentTime = playStartSec;
    }
  }

  function onEnded() {
    // Auto-advance to next take
    const idx = playingKey ? allPlayable.findIndex((p) => p.key === playingKey) : -1;
    if (idx >= 0 && idx < allPlayable.length - 1) {
      playByIndex(idx + 1);
    } else {
      playingKey = null;
      playingTrackId = null;
      currentTime = 0;
    }
  }

  function seekDirect(e: MouseEvent, track: Track) {
    const bar = e.currentTarget as HTMLElement;
    const rect = bar.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    if (audioEl && playingKey === track.id) {
      audioEl.currentTime = pct * (track.duration_ms / 1000);
    }
  }

  function seekSetTake(e: MouseEvent, take: SetTake) {
    const bar = e.currentTarget as HTMLElement;
    const rect = bar.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    const regionDur = (take.end_ms - take.start_ms) / 1000;
    if (audioEl && playingKey === take.set_item_id) {
      audioEl.currentTime = take.start_ms / 1000 + pct * regionDur;
    }
  }

  // --- Comments for set takes ---

  async function toggleTakeComments(take: SetTake) {
    if (expandedTakeId === take.set_item_id) {
      expandedTakeId = null;
      return;
    }
    expandedTakeId = take.set_item_id;
    takeCommentInput = "";
    if (!trackComments.has(take.track_id)) {
      await refreshTakeComments(take.track_id);
    }
  }

  async function refreshTakeComments(trackId: string) {
    const comments = await api<Comment[]>(`/api/tracks/${trackId}/comments`);
    trackComments.set(trackId, comments);
    trackComments = new Map(trackComments);
  }

  function getFilteredComments(take: SetTake): Comment[] {
    const all = trackComments.get(take.track_id) || [];
    return all
      .filter(
        (c) =>
          c.timestamp_ms != null &&
          c.timestamp_ms >= take.start_ms &&
          c.timestamp_ms <= take.end_ms
      )
      .map((c) => ({
        ...c,
        // Adjust timestamps to be relative to region start
        timestamp_ms: c.timestamp_ms != null ? c.timestamp_ms - take.start_ms : null,
        replies: c.replies?.map((r) => ({
          ...r,
          timestamp_ms: r.timestamp_ms != null ? r.timestamp_ms - take.start_ms : null,
        })),
      }));
  }

  function handleTakeCommentSeek(take: SetTake, relativeMs: number) {
    const absoluteSec = (relativeMs + take.start_ms) / 1000;
    if (playingKey !== take.set_item_id) {
      pendingSeekSec = absoluteSec;
      togglePlay(take.track_id, take.set_item_id, take.start_ms, take.end_ms);
    } else if (audioEl) {
      audioEl.currentTime = absoluteSec;
    }
  }

  async function postTakeComment(take: SetTake) {
    if (!takeCommentInput.trim()) return;
    postingComment = true;
    try {
      // Attach current playback position as absolute timestamp if playing this take
      let timestampMs: number | null = null;
      if (playingKey === take.set_item_id && audioEl) {
        timestampMs = Math.round(audioEl.currentTime * 1000);
      }
      await apiPost(`/api/tracks/${take.track_id}/comments`, {
        body: takeCommentInput.trim(),
        timestamp_ms: timestampMs,
      });
      takeCommentInput = "";
      await refreshTakeComments(take.track_id);
    } finally {
      postingComment = false;
    }
  }

  // --- Data loading ---

  // Keep player store in sync with current audio element
  $effect(() => {
    player.setMediaElement(audioEl);
  });

  // If the global player starts playing, pause local set-take audio
  $effect(() => {
    if ($playerState.playing && audioEl && !audioEl.paused) {
      audioEl.pause();
    }
  });

  onMount(() => {
    loadSong();
    loadTracks();
    loadSetTakes();
    loadSets();
    player.registerPlaylist({ onNext: playNext, onPrev: playPrev });
  });

  onDestroy(() => {
    player.unregisterPlaylist();
    player.setMediaElement(null);
  });

  async function loadSong() {
    song = await api<Song>(`/api/bands/${slug}/songs/${songId}`);
    lyricsInput = song.lyrics;
    tabsInput = song.tabs;
    notesInput = song.notes;
    bpmInput = song.bpm;
    // On desktop, default-open the first populated document section
    if (expandedSection === null && window.innerWidth >= 768) {
      expandedSection = song.lyrics ? "lyrics" : song.notes ? "notes" : song.tabs ? "tabs" : null;
    }
  }

  async function loadTracks() {
    const all = await api<Track[]>(`/api/bands/${slug}/tracks`);
    tracks = all.filter((t) => t.song_id === songId);
  }

  async function loadSetTakes() {
    setTakes = await api<SetTake[]>(`/api/bands/${slug}/songs/${songId}/takes`);
  }

  async function loadSets() {
    songSets = await api<SetSummary[]>(`/api/bands/${slug}/songs/${songId}/sets`);
  }


  async function saveLyrics() {
    if (!song) return;
    saving = true;
    try {
      song = await apiPatch<Song>(`/api/bands/${slug}/songs/${songId}`, {
        lyrics: lyricsInput,
      });
      editingLyrics = false;
    } finally {
      saving = false;
    }
  }

  async function saveTabs() {
    if (!song) return;
    saving = true;
    try {
      song = await apiPatch<Song>(`/api/bands/${slug}/songs/${songId}`, {
        tabs: tabsInput,
      });
      editingTabs = false;
    } finally {
      saving = false;
    }
  }

  async function saveNotes() {
    if (!song) return;
    saving = true;
    try {
      song = await apiPatch<Song>(`/api/bands/${slug}/songs/${songId}`, {
        notes: notesInput,
      });
      editingNotes = false;
    } finally {
      saving = false;
    }
  }
</script>

{#if song}
  <div class="space-y-4 md:space-y-6">
    <!-- Breadcrumb -->
    <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5">
      <button onclick={() => navigate(`/band/${slug}`)} class="hover:text-accent/60 transition-colors py-1">{slug.toUpperCase()}</button>
      <span class="text-text-muted/20">&rsaquo;</span>
      <span class="text-text-muted/50">{song.name.toUpperCase()}</span>
    </div>

    <!-- Header -->
    <div class="flex items-center gap-4">
      {#if editingTitle}
        <form onsubmit={(e) => { e.preventDefault(); saveTitle(); }} class="flex items-center gap-3 flex-1">
          <input
            bind:value={titleInput}
            type="text"
            class="flex-1 bg-bg-primary border border-border px-4 py-2 text-3xl md:text-4xl font-display font-bold tracking-wider text-text-primary focus:outline-none focus:border-accent transition-colors"
          />
          <button type="submit" disabled={savingTitle || !titleInput.trim()} class="label text-accent hover:text-accent-hover disabled:opacity-50 transition-colors">{savingTitle ? "..." : "save"}</button>
          <button type="button" onclick={() => (editingTitle = false)} class="label text-text-muted hover:text-text-secondary transition-colors">cancel</button>
        </form>
      {:else}
        <h1
          class="text-3xl md:text-4xl font-display font-bold tracking-wider text-text-primary cursor-pointer hover:text-accent transition-colors"
          onclick={() => { editingTitle = true; titleInput = song!.name; }}
          title="click to edit"
        >{song.name}</h1>
      {/if}

      <!-- BPM -->
      <div class="flex items-center gap-2 mt-2">
        <span class="label-sm text-text-muted/60">bpm</span>
        <input
          type="number"
          min="0"
          max="300"
          step="1"
          bind:value={bpmInput}
          onchange={saveBpm}
          placeholder="--"
          class="w-14 bg-bg-primary border border-border px-2 py-1 label-sm font-mono text-text-secondary text-right placeholder:text-text-muted/40 focus:outline-none focus:border-accent transition-colors {bpmInput === 0 ? 'text-text-muted/40' : ''}"
        />
        <button
          onclick={handleTap}
          class="label-sm text-text-muted hover:text-accent active:scale-95 transition-all px-2 py-1 border border-border hover:border-accent"
        >tap</button>
        {#if bpmInput > 0 && bpmInput !== song.bpm}
          <button
            onclick={saveBpm}
            disabled={savingBpm}
            class="label-sm text-accent hover:text-accent-hover transition-colors"
          >{savingBpm ? "..." : "save"}</button>
        {/if}
        {#if !confirmDelete}
          <button
            onclick={() => (confirmDelete = true)}
            class="label text-text-muted hover:text-danger transition-colors"
          >
            delete
          </button>
        {:else}
          <span class="flex items-center gap-2">
            <span class="label-sm text-danger">sure?</span>
            <button
              onclick={deleteSong}
              disabled={deleting}
              class="label-sm text-danger hover:text-red-300 transition-colors"
            >{deleting ? "..." : "yes"}</button>
            <button
              onclick={() => (confirmDelete = false)}
              class="label-sm text-text-muted hover:text-text-secondary transition-colors"
            >no</button>
          </span>
        {/if}
      </div>
    </div>

    <!-- Two-column layout on desktop -->
    <div class="md:grid md:grid-cols-[2fr_3fr] md:gap-x-10 md:items-start">
    <div class="space-y-6"><!-- left column: takes + sets -->

    <!-- Takes -->
    <section>
        <h2 class="label text-text-muted mb-3"><span class="text-accent/15 font-mono mr-2">01</span>takes</h2>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-1 mb-2">
          <TrackUpload bandSlug={slug} {songId} onUploaded={loadTracks} />
          <Recorder bandSlug={slug} {songId} bpm={song?.bpm ?? 0} onRecorded={loadTracks} />
        </div>
        <div class="flex gap-2 mb-4">
          <a href="/api/bands/{slug}/sync/binary?platform=linux&song_id={songId}" class="label-sm text-text-muted/40 hover:text-text-muted transition-colors">reaper sync (linux)</a>
          <span class="label-sm text-text-muted/20">·</span>
          <a href="/api/bands/{slug}/sync/binary?platform=windows&song_id={songId}" class="label-sm text-text-muted/40 hover:text-text-muted transition-colors">reaper sync (windows)</a>
        </div>

        {#if directTracks.length > 0 || setTakes.length > 0}
        <div class="space-y-2">
          <!-- Direct takes -->
          {#each directTracks as track}
            <div class="bg-bg-surface border border-border">
              <div class="flex flex-col p-4 gap-1">
                <!-- Top row: play + title -->
                <div class="flex items-center gap-2 min-w-0">
                  {#if track.status === "ready"}
                    <div class="flex items-center shrink-0">
                      <button
                        onclick={() => togglePlay(track.id)}
                        class="w-8 h-8 flex items-center justify-center text-text-muted hover:text-accent transition-colors"
                      >
                        {#if $playerState.track?.id === track.id && $playerState.playing}
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                            <rect x="6" y="4" width="4" height="16"/>
                            <rect x="14" y="4" width="4" height="16"/>
                          </svg>
                        {:else}
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                            <polygon points="5,3 19,12 5,21"/>
                          </svg>
                        {/if}
                      </button>
                      <button
                        onclick={() => globalPlayer.enqueue({ id: track.id, title: track.title, bandSlug: slug, songName: song?.name })}
                        class="w-6 h-6 flex items-center justify-center text-text-muted/0 hover:text-accent transition-colors"
                        title="Add to queue"
                      >
                        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                          <line x1="12" y1="5" x2="12" y2="19"/>
                          <line x1="5" y1="12" x2="19" y2="12"/>
                        </svg>
                      </button>
                    </div>
                  {:else}
                    <div class="w-8 h-8 flex items-center justify-center shrink-0">
                      {#if track.status === "processing"}
                        <div class="w-2 h-2 bg-accent animate-pulse"></div>
                      {:else}
                        <span class="label-sm text-danger">!</span>
                      {/if}
                    </div>
                  {/if}
                  <button
                    onclick={() => navigate(`/band/${slug}/track/${track.id}`)}
                    class="flex-1 min-w-0 text-sm font-semibold text-text-primary hover:text-accent transition-colors truncate text-left font-display tracking-wide"
                  >{track.title}</button>
                </div>
                <!-- Bottom row: meta + tags + personnel, indented past play button -->
                <div class="flex items-center gap-2 flex-wrap pl-[3.5rem] label-sm text-text-muted">
                  {#if track.status === "ready"}
                    <span class="font-mono">{formatDuration(track.duration_ms)}</span>
                  {/if}
                  <span>{track.uploader?.display_name || track.uploader?.email || ""}</span>
                  <span>{formatRelativeTime(track.created_at)}</span>
                  {#if track.tags?.length}
                    {#each track.tags as tag}
                      <span class="text-accent border border-accent/30 px-1.5 py-0.5">{tag}</span>
                    {/each}
                  {/if}
                  {#if track.personnel?.length}
                    {#each track.personnel as p}
                      <span>{p.user.display_name || p.user.email}{#if p.role} · {p.role}{/if}</span>
                    {/each}
                  {/if}
                </div>
              </div>

              {#if playingKey === track.id && track.status === "ready"}
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="h-1 bg-bg-primary cursor-pointer"
                  onclick={(e) => seekDirect(e, track)}
                >
                  <div
                    class="h-full bg-accent transition-[width] duration-100"
                    style="width: {audioDuration > 0 ? (currentTime / audioDuration) * 100 : 0}%"
                  ></div>
                </div>
              {/if}
            </div>
          {/each}

          <!-- Set-derived takes -->
          {#each setTakes as take}
            {@const takeDurationMs = take.end_ms - take.start_ms}
            {@const isExpanded = expandedTakeId === take.set_item_id}
            {@const filteredComments = isExpanded ? getFilteredComments(take) : []}
            <div class="bg-bg-surface border border-border">
              <div class="flex flex-col p-4 gap-1">
                <!-- Top row: play + title -->
                <div class="flex items-center gap-2 min-w-0">
                  <button
                    onclick={() => togglePlay(take.track_id, take.set_item_id, take.start_ms, take.end_ms)}
                    class="w-8 h-8 flex items-center justify-center text-text-muted hover:text-accent transition-colors shrink-0"
                  >
                    {#if playingKey === take.set_item_id && audioEl && !audioEl.paused}
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                        <rect x="6" y="4" width="4" height="16"/>
                        <rect x="14" y="4" width="4" height="16"/>
                      </svg>
                    {:else}
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                        <polygon points="5,3 19,12 5,21"/>
                      </svg>
                    {/if}
                  </button>
                  <button
                    onclick={() => navigate(`/band/${slug}/${take.set_id ? "set/" + take.set_id : "take/" + take.track_id}`)}
                    class="flex-1 min-w-0 text-sm font-semibold text-text-primary hover:text-accent transition-colors truncate text-left font-display tracking-wide"
                  >{take.track_title}</button>
                </div>
                <!-- Bottom row: meta, indented past play button -->
                <div class="flex items-center gap-2 flex-wrap pl-10 label-sm text-text-muted">
                  <button
                    onclick={() => navigate(`/band/${slug}/set/${take.set_id}`)}
                    class="bg-bg-primary border border-border px-1.5 py-0.5 hover:text-accent hover:border-accent/40 transition-colors"
                  >{take.set_name}</button>
                  <span class="text-accent bg-accent/10 px-1.5 py-0.5">{setTypeLabel(take.set_type)}</span>
                  {#if take.tags?.length}
                    {#each take.tags as tag}
                      <span class="text-accent border border-accent/30 px-1.5 py-0.5">{tag}</span>
                    {/each}
                  {/if}
                  <span class="font-mono">{formatDuration(take.start_ms)} — {formatDuration(take.end_ms)}</span>
                  <span class="font-mono">{formatDuration(takeDurationMs)}</span>
                  <span>{take.uploader?.display_name || take.uploader?.email || ""}</span>
                  <a
                    href={`/api/bands/${slug}/tracks/${take.track_id}/stream?dl=1&start_ms=${take.start_ms}&end_ms=${take.end_ms}&title=${encodeURIComponent(`${song?.name ?? "take"} (${take.set_name})`)}`}
                    class="text-accent hover:text-accent-hover transition-colors"
                  >dl</a>
                  <button
                    onclick={() => toggleTakeComments(take)}
                    class="hover:text-accent transition-colors"
                  >{isExpanded ? "hide comments" : "comments"}</button>
                  <span class="font-mono">{formatRelativeTime(take.created_at)}</span>
                </div>
              </div>

              <!-- Progress bar -->
              {#if playingKey === take.set_item_id}
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                {@const regionStartSec = take.start_ms / 1000}
                {@const regionDurSec = takeDurationMs / 1000}
                {@const progress = regionDurSec > 0 ? Math.max(0, Math.min(1, (currentTime - regionStartSec) / regionDurSec)) : 0}
                <div
                  class="h-1 bg-bg-primary cursor-pointer"
                  onclick={(e) => seekSetTake(e, take)}
                >
                  <div
                    class="h-full bg-accent transition-[width] duration-100"
                    style="width: {progress * 100}%"
                  ></div>
                </div>
              {/if}

              <!-- Expandable comments -->
              {#if isExpanded}
                <div class="border-t border-border">
                  <!-- Comment input -->
                  <form
                    onsubmit={(e) => { e.preventDefault(); postTakeComment(take); }}
                    class="flex gap-2 p-4 pb-2"
                  >
                    {#if playingKey === take.set_item_id && audioEl}
                      <span class="label-sm text-accent bg-accent/10 px-2 py-2 font-mono shrink-0">
                        @{formatTimestamp(Math.max(0, Math.round(audioEl.currentTime * 1000) - take.start_ms))}
                      </span>
                    {/if}
                    <input
                      bind:value={takeCommentInput}
                      type="text"
                      placeholder="add a comment..."
                      class="flex-1 bg-bg-primary border border-border px-4 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
                    />
                    <button
                      type="submit"
                      disabled={postingComment || !takeCommentInput.trim()}
                      class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors"
                    >post</button>
                  </form>
                  {#if playingKey === take.set_item_id}
                    <div class="px-4 pb-2 label-sm text-text-muted">
                      timestamp auto-attached from playback position
                    </div>
                  {/if}
                  <CommentList
                    comments={filteredComments}
                    trackId={take.track_id}
                    onSeek={(ms) => handleTakeCommentSeek(take, ms)}
                    onRefresh={() => refreshTakeComments(take.track_id)}
                  />
                </div>
              {/if}
            </div>
          {/each}
        </div>
        {/if}
      </section>

    <!-- Sets -->
    {#if songSets.length > 0}
      <section>
        <h2 class="label text-text-muted mb-3"><span class="text-accent/15 font-mono mr-2">02</span>sets</h2>
        <div class="space-y-2">
          {#each songSets as s}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="bg-bg-surface border border-border p-4 hover:border-accent/40 transition-colors cursor-pointer group flex items-center justify-between"
              onclick={() => navigate(`/band/${slug}/set/${s.id}`)}
            >
              <div class="flex items-center gap-3 min-w-0">
                <span class="text-sm font-semibold text-text-primary font-display tracking-wide group-hover:text-accent transition-colors truncate">{s.name}</span>
                <span class="label-sm text-accent bg-accent/10 px-1.5 py-0.5 shrink-0 font-mono" title={setTypeLabel(s.set_type)}>{setTypeCode(s.set_type)}</span>
              </div>
              <div class="flex items-center gap-3 label-sm text-text-muted shrink-0">
                <span>{s.item_count} {s.item_count === 1 ? 'song' : 'songs'}</span>
                {#if s.recorded_at}
                  <span>{new Date(s.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    </div><!-- /left column -->

    <!-- Right column: notes / lyrics / tabs — top offset aligns with dropzones, not the takes heading -->
    <div class="mt-6 md:mt-0 md:pt-[31px]">
    <div class="space-y-0">
      <!-- Notes -->
      <section class="border border-border">
        <button
          onclick={() => { expandedSection = expandedSection === "notes" ? null : "notes"; }}
          class="w-full flex items-center justify-between px-5 py-3 hover:bg-bg-surface/50 transition-colors"
        >
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-accent/20 font-mono label shrink-0">{notesNum}</span>
            <span class="label {song.notes ? 'text-text-primary' : 'text-text-muted'}">notes</span>
            {#if song.notes}
              <span class="label-sm text-text-muted/50 truncate hidden sm:inline">{song.notes.split('\n')[0].slice(0, 72)}{song.notes.split('\n')[0].length > 72 ? '…' : ''}</span>
            {:else}
              <span class="label-sm text-text-muted/30">empty</span>
            {/if}
          </div>
          <span class="label-sm transition-transform shrink-0 {expandedSection === 'notes' ? 'rotate-90 text-accent' : song.notes ? 'text-text-muted' : 'text-text-muted/20'}">&rsaquo;</span>
        </button>
        {#if editingNotes}
          <div class="px-5 pb-5 border-t border-border/50 pt-4">
            <textarea
              bind:value={notesInput}
              rows="14"
              class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y md:min-h-[320px]"
              placeholder="production notes, arrangement ideas, references..."
            ></textarea>
            <div class="flex gap-3 mt-3 md:sticky md:bottom-4 md:bg-bg-primary md:py-2 md:border-t md:border-border/40">
              <button onclick={saveNotes} disabled={saving} class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors">{saving ? "..." : "save"}</button>
              <button onclick={() => { editingNotes = false; notesInput = song!.notes; }} class="label-sm text-text-muted hover:text-text-secondary transition-colors">cancel</button>
            </div>
          </div>
        {:else if expandedSection === "notes"}
          <div class="px-5 pb-5 border-t border-border/50 pt-4">
            {#if song.notes}
              <div class="prose bg-bg-surface border border-border p-5">{@html renderMd(song.notes)}</div>
              <button onclick={() => { editingNotes = true; notesInput = song!.notes; }} class="label-sm text-text-muted hover:text-accent transition-colors mt-3">edit</button>
            {:else}
              <button
                onclick={() => { editingNotes = true; notesInput = ""; }}
                class="text-sm text-text-muted hover:text-accent italic transition-colors"
              >no notes yet — click to add</button>
            {/if}
          </div>
        {/if}
      </section>

      <!-- Lyrics -->
      <section class="border border-border border-t-0">
        <button
          onclick={() => { expandedSection = expandedSection === "lyrics" ? null : "lyrics"; }}
          class="w-full flex items-center justify-between px-5 py-3 hover:bg-bg-surface/50 transition-colors"
        >
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-accent/20 font-mono label shrink-0">{lyricsNum}</span>
            <span class="label {song.lyrics ? 'text-text-primary' : 'text-text-muted'}">lyrics</span>
            {#if song.lyrics}
              {#if hasChordPro(song.lyrics)}
                <span class="label-sm text-accent/60 bg-accent/10 px-1.5 py-0.5 shrink-0">CP</span>
              {/if}
              <span class="label-sm text-text-muted/50 truncate hidden sm:inline">{song.lyrics.split('\n')[0].slice(0, 72)}{song.lyrics.split('\n')[0].length > 72 ? '…' : ''}</span>
            {:else}
              <span class="label-sm text-text-muted/30">empty</span>
            {/if}
          </div>
          <span class="label-sm transition-transform shrink-0 {expandedSection === 'lyrics' ? 'rotate-90 text-accent' : song.lyrics ? 'text-text-muted' : 'text-text-muted/20'}">&rsaquo;</span>
        </button>
        {#if editingLyrics}
          <div class="px-5 pb-5 border-t border-border/50 pt-4">
            <textarea
              bind:value={lyricsInput}
              rows="20"
              class="w-full bg-bg-primary border border-border px-4 py-4 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y md:min-h-[480px] leading-relaxed"
              placeholder="lyrics with optional [Am]chordpro [G]notation..."
            ></textarea>
            <div class="flex gap-3 mt-3 md:sticky md:bottom-4 md:bg-bg-primary md:py-2 md:border-t md:border-border/40">
              <button onclick={saveLyrics} disabled={saving} class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors">{saving ? "..." : "save"}</button>
              <button onclick={() => { editingLyrics = false; lyricsInput = song!.lyrics; }} class="label-sm text-text-muted hover:text-text-secondary transition-colors">cancel</button>
            </div>
          </div>
        {:else if expandedSection === "lyrics"}
          <div class="px-5 pb-5 border-t border-border/50 pt-4">
            {#if song.lyrics}
              {#if hasChordPro(song.lyrics)}
                <div class="flex items-center justify-end mb-3">
                  <button
                    onclick={() => (showChords = !showChords)}
                    class="label-sm transition-colors {showChords ? 'text-accent' : 'text-text-muted hover:text-accent'}"
                  >{showChords ? "hide chords" : "show chords"}</button>
                </div>
                <div class="bg-bg-surface border border-border p-5">
                  <ChordProLyrics text={song.lyrics} {showChords} />
                </div>
              {:else}
                <div class="prose bg-bg-surface border border-border p-5">{@html renderMd(song.lyrics)}</div>
              {/if}
              <button onclick={() => { editingLyrics = true; lyricsInput = song!.lyrics; }} class="label-sm text-text-muted hover:text-accent transition-colors mt-3">edit</button>
            {:else}
              <button
                onclick={() => { editingLyrics = true; lyricsInput = ""; }}
                class="text-sm text-text-muted hover:text-accent italic transition-colors"
              >no lyrics yet — click to add</button>
            {/if}
          </div>
        {/if}
      </section>

      <!-- Tabs -->
      <section class="border border-border border-t-0">
        <button
          onclick={() => { expandedSection = expandedSection === "tabs" ? null : "tabs"; }}
          class="w-full flex items-center justify-between px-5 py-3 hover:bg-bg-surface/50 transition-colors"
        >
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-accent/20 font-mono label shrink-0">{tabsNum}</span>
            <span class="label {song.tabs ? 'text-text-primary' : 'text-text-muted'}">tabs / chords</span>
            {#if song.tabs}
              <span class="label-sm text-text-muted/50 truncate hidden sm:inline">{song.tabs.split('\n')[0].slice(0, 72)}{song.tabs.split('\n')[0].length > 72 ? '…' : ''}</span>
            {:else}
              <span class="label-sm text-text-muted/30">empty</span>
            {/if}
          </div>
          <span class="label-sm transition-transform shrink-0 {expandedSection === 'tabs' ? 'rotate-90 text-accent' : song.tabs ? 'text-text-muted' : 'text-text-muted/20'}">&rsaquo;</span>
        </button>
        {#if editingTabs}
          <div class="px-5 pb-5 border-t border-border/50 pt-4">
            <textarea
              bind:value={tabsInput}
              rows="20"
              class="w-full bg-bg-primary border border-border px-4 py-4 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors font-mono resize-y md:min-h-[480px] leading-relaxed"
              placeholder="paste tabs or chord charts..."
            ></textarea>
            <div class="flex gap-3 mt-3 md:sticky md:bottom-4 md:bg-bg-primary md:py-2 md:border-t md:border-border/40">
              <button onclick={saveTabs} disabled={saving} class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors">{saving ? "..." : "save"}</button>
              <button onclick={() => { editingTabs = false; tabsInput = song!.tabs; }} class="label-sm text-text-muted hover:text-text-secondary transition-colors">cancel</button>
            </div>
          </div>
        {:else if expandedSection === "tabs"}
          <div class="px-5 pb-5 border-t border-border/50 pt-4">
            {#if song.tabs}
              {#if hasChordPro(song.tabs)}
                <div class="bg-bg-surface border border-border p-5">
                  <ChordProLyrics text={song.tabs} {showChords} />
                </div>
              {:else}
                <pre class="text-sm text-text-secondary whitespace-pre-wrap font-mono leading-relaxed bg-bg-surface border border-border p-5">{song.tabs}</pre>
              {/if}
              <button onclick={() => { editingTabs = true; tabsInput = song!.tabs; }} class="label-sm text-text-muted hover:text-accent transition-colors mt-3">edit</button>
            {:else}
              <button
                onclick={() => { editingTabs = true; tabsInput = ""; }}
                class="text-sm text-text-muted hover:text-accent italic transition-colors"
              >no tabs yet — click to add</button>
            {/if}
          </div>
        {/if}
      </section>
    </div><!-- /sections -->
    </div><!-- /right column -->
    </div><!-- /two-column grid -->

    <!-- Hidden audio element -->
    {#if playingTrackId}
      <audio
        bind:this={audioEl}
        src={`/api/bands/${slug}/tracks/${playingTrackId}/stream`}
        autoplay
        ontimeupdate={onTimeUpdate}
        onloadedmetadata={onLoadedMetadata}
        onended={onEnded}
      ></audio>
    {/if}
  </div>
{:else}
  <div class="flex items-center gap-2 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
{/if}
