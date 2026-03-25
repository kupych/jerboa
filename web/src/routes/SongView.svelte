<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { marked } from "marked";
  import { api, apiPatch, apiPost, uploadFile } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { player } from "../lib/stores/player";
  import { formatDuration, formatRelativeTime, formatTimestamp, setTypeCode } from "../lib/utils/format";
  import CommentList from "../lib/components/CommentList.svelte";
  import Recorder from "../lib/components/Recorder.svelte";
  import ChordProLyrics from "../lib/components/ChordProLyrics.svelte";

  marked.setOptions({ breaks: true, gfm: true });

  let { slug, songId }: { slug: string; songId: string } = $props();

  type Song = {
    id: string;
    name: string;
    lyrics: string;
    tabs: string;
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
  let lyricsInput = $state("");
  let tabsInput = $state("");
  let saving = $state(false);

  let editingTitle = $state(false);
  let titleInput = $state("");
  let savingTitle = $state(false);
  let showChords = $state(true);

  // BPM / tap tempo
  let bpmInput = $state(0);
  let savingBpm = $state(false);
  let tapTimes: number[] = [];

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
    bpmInput = song.bpm;
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

  // Take upload
  let takeUploading = $state(false);
  let takeProgress = $state(0);
  let takeError = $state("");
  let takeFileInput = $state<HTMLInputElement | null>(null);
  let takeDragging = $state(false);

  async function uploadTake(file: File) {
    takeError = "";
    takeUploading = true;
    takeProgress = 0;
    try {
      const track = await uploadFile<{ id: string }>(
        `/api/bands/${slug}/tracks`,
        file,
        {},
        (pct) => (takeProgress = pct),
      );
      await apiPatch(`/api/bands/${slug}/tracks/${track.id}/song`, {
        song_id: songId,
      });
      await loadTracks();
    } catch (e: any) {
      takeError = e.message || "Upload failed";
    } finally {
      takeUploading = false;
      if (takeFileInput) takeFileInput.value = "";
    }
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
</script>

{#if song}
  <div class="space-y-8">
    <!-- Breadcrumb -->
    <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5">
      <button onclick={() => navigate(`/band/${slug}`)} class="hover:text-accent/60 transition-colors py-1">{slug.toUpperCase()}</button>
      <span>/</span>
      <span class="text-text-muted/50">S:{song.name.replace(/\s+/g, "").toUpperCase()}</span>
    </div>

    <!-- Header -->
    <div class="flex items-center gap-4">
      {#if editingTitle}
        <form onsubmit={(e) => { e.preventDefault(); saveTitle(); }} class="flex items-center gap-3 flex-1">
          <input
            bind:value={titleInput}
            type="text"
            class="flex-1 bg-bg-primary border border-border px-4 py-2 text-2xl font-display font-bold tracking-wider text-text-primary focus:outline-none focus:border-accent transition-colors"
          />
          <button type="submit" disabled={savingTitle || !titleInput.trim()} class="label text-accent hover:text-accent-hover disabled:opacity-50 transition-colors">{savingTitle ? "..." : "save"}</button>
          <button type="button" onclick={() => (editingTitle = false)} class="label text-text-muted hover:text-text-secondary transition-colors">cancel</button>
        </form>
      {:else}
        <h1
          class="text-2xl font-display font-bold tracking-wider text-text-primary cursor-pointer hover:text-accent transition-colors"
          onclick={() => { editingTitle = true; titleInput = song!.name; }}
          title="click to edit"
        >{song.name}</h1>
      {/if}

      <!-- BPM -->
      <div class="flex items-center gap-3 mt-4">
        <span class="label-sm text-text-muted">bpm:</span>
        <input
          type="number"
          min="0"
          max="300"
          step="1"
          bind:value={bpmInput}
          onchange={saveBpm}
          class="w-16 bg-bg-primary border border-border px-2 py-1 label-sm font-mono text-text-secondary text-right focus:outline-none focus:border-accent transition-colors"
        />
        <button
          onclick={handleTap}
          class="label-sm text-text-muted hover:text-accent transition-colors px-3 py-1 border border-border hover:border-accent"
        >tap</button>
        {#if bpmInput > 0 && bpmInput !== song.bpm}
          <button
            onclick={saveBpm}
            disabled={savingBpm}
            class="label-sm text-accent hover:text-accent-hover transition-colors"
          >{savingBpm ? "..." : "save"}</button>
        {/if}
      </div>
    </div>

    <!-- Lyrics -->
    <section>
      <div class="flex items-center justify-between mb-3">
        <h2 class="label text-text-muted"><span class="text-accent/15 font-mono mr-2">01</span>lyrics</h2>
        <div class="flex items-center gap-3">
          {#if !editingLyrics && song.lyrics && hasChordPro(song.lyrics)}
            <button
              onclick={() => (showChords = !showChords)}
              class="label-sm transition-colors {showChords ? 'text-accent' : 'text-text-muted hover:text-accent'}"
            >{showChords ? "hide chords" : "show chords"}</button>
          {/if}
          {#if !editingLyrics}
            <button
              onclick={() => { editingLyrics = true; lyricsInput = song!.lyrics; }}
              class="label-sm text-text-muted hover:text-accent transition-colors"
            >edit</button>
          {/if}
        </div>
      </div>

      {#if editingLyrics}
        <div class="space-y-3">
          <textarea
            bind:value={lyricsInput}
            rows="16"
            class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y"
            placeholder="lyrics with optional [Am]chordpro [G]notation..."
          ></textarea>
          <div class="flex gap-3">
            <button
              onclick={saveLyrics}
              disabled={saving}
              class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors"
            >{saving ? "..." : "save"}</button>
            <button
              onclick={() => { editingLyrics = false; lyricsInput = song!.lyrics; }}
              class="label-sm text-text-muted hover:text-text-secondary transition-colors"
            >cancel</button>
          </div>
        </div>
      {:else if song.lyrics}
        {#if hasChordPro(song.lyrics)}
          <div class="bg-bg-surface border border-border p-5">
            <ChordProLyrics text={song.lyrics} {showChords} />
          </div>
        {:else}
          <div class="prose bg-bg-surface border border-border p-5">{@html marked.parse(song.lyrics)}</div>
        {/if}
      {:else}
        <p class="text-sm text-text-muted italic">no lyrics yet</p>
      {/if}
    </section>

    <!-- Tabs -->
    <section>
      <div class="flex items-center justify-between mb-3">
        <h2 class="label text-text-muted"><span class="text-accent/15 font-mono mr-2">02</span>tabs / chords</h2>
        {#if !editingTabs}
          <button
            onclick={() => { editingTabs = true; tabsInput = song!.tabs; }}
            class="label-sm text-text-muted hover:text-accent transition-colors"
          >edit</button>
        {/if}
      </div>

      {#if editingTabs}
        <div class="space-y-3">
          <textarea
            bind:value={tabsInput}
            rows="16"
            class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors font-mono resize-y"
            placeholder="paste tabs or chord charts with optional [Am]chordpro [G]notation..."
          ></textarea>
          <div class="flex gap-3">
            <button
              onclick={saveTabs}
              disabled={saving}
              class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors"
            >{saving ? "..." : "save"}</button>
            <button
              onclick={() => { editingTabs = false; tabsInput = song!.tabs; }}
              class="label-sm text-text-muted hover:text-text-secondary transition-colors"
            >cancel</button>
          </div>
        </div>
      {:else if song.tabs}
        {#if hasChordPro(song.tabs)}
          <div class="bg-bg-surface border border-border p-5">
            <ChordProLyrics text={song.tabs} {showChords} />
          </div>
        {:else}
          <pre class="text-sm text-text-secondary whitespace-pre-wrap font-mono leading-relaxed bg-bg-surface border border-border p-5">{song.tabs}</pre>
        {/if}
      {:else}
        <p class="text-sm text-text-muted italic">no tabs yet</p>
      {/if}
    </section>

    <!-- Takes -->
      <section>
        <h2 class="label text-text-muted mb-3"><span class="text-accent/15 font-mono mr-2">03</span>takes</h2>

        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="border border-dashed mb-3 p-4 text-center transition-colors {takeDragging ? 'border-accent bg-accent/5' : 'border-border hover:border-text-muted'}"
          ondragover={(e) => { e.preventDefault(); takeDragging = true; }}
          ondragleave={() => (takeDragging = false)}
          ondrop={(e) => { e.preventDefault(); takeDragging = false; const f = e.dataTransfer?.files[0]; if (f) uploadTake(f); }}
        >
          {#if takeUploading}
            <div class="flex items-center gap-4">
              <div class="flex-1 bg-bg-primary h-1">
                <div class="bg-accent h-1 transition-all duration-300" style="width: {takeProgress}%"></div>
              </div>
              <span class="label-sm font-mono text-text-muted">{takeProgress}%</span>
            </div>
          {:else}
            <div class="flex items-center justify-center gap-2">
              <svg class="text-text-muted" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                <polyline points="17 8 12 3 7 8"/>
                <line x1="12" y1="3" x2="12" y2="15"/>
              </svg>
              <span class="text-sm text-text-muted font-semibold">
                drop a take or
                <button onclick={() => takeFileInput?.click()} class="text-accent hover:text-accent-hover underline underline-offset-4">browse</button>
              </span>
            </div>
            <input
              bind:this={takeFileInput}
              type="file"
              accept=".mp3,.wav,.flac,.ogg,.aac,.m4a,.aiff,.aif,.opus"
              class="hidden"
              onchange={(e) => { const f = (e.target as HTMLInputElement).files?.[0]; if (f) uploadTake(f); }}
            />
          {/if}
          {#if takeError}
            <div class="mt-2 label-sm text-danger">{takeError}</div>
          {/if}
        </div>

        <Recorder bandSlug={slug} {songId} bpm={song?.bpm ?? 0} onRecorded={loadTracks} />

        {#if directTracks.length > 0 || setTakes.length > 0}
        <div class="space-y-2">
          <!-- Direct takes -->
          {#each directTracks as track}
            <div class="bg-bg-surface border border-border">
              <div class="flex items-center gap-4 p-4">
                {#if track.status === "ready"}
                  <button
                    onclick={() => togglePlay(track.id)}
                    class="w-8 h-8 flex items-center justify-center text-text-muted hover:text-accent transition-colors shrink-0"
                  >
                    {#if playingKey === track.id && audioEl && !audioEl.paused}
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
                {:else}
                  <div class="w-8 h-8 flex items-center justify-center shrink-0">
                    {#if track.status === "processing"}
                      <div class="w-2 h-2 bg-accent animate-pulse"></div>
                    {:else}
                      <span class="label-sm text-danger">!</span>
                    {/if}
                  </div>
                {/if}

                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2 flex-wrap">
                    <button
                      onclick={() => navigate(`/band/${slug}/track/${track.id}`)}
                      class="text-sm font-semibold text-text-primary hover:text-accent transition-colors truncate text-left font-display tracking-wide"
                    >{track.title}</button>
                    {#if track.tags?.length}
                      {#each track.tags as tag}
                        <span class="label-sm text-accent bg-accent/10 px-1.5 py-0.5">{tag}</span>
                      {/each}
                    {/if}
                  </div>
                  {#if track.personnel?.length}
                    <div class="flex items-center gap-2 mt-1 flex-wrap">
                      {#each track.personnel as p}
                        <span class="label-sm text-text-muted">{p.user.display_name || p.user.email}{#if p.role} · {p.role}{/if}</span>
                      {/each}
                    </div>
                  {/if}
                </div>

                <div class="flex items-center gap-3 label-sm text-text-muted shrink-0">
                  {#if track.status === "ready"}
                    <span class="font-mono">{formatDuration(track.duration_ms)}</span>
                  {/if}
                  <span>{track.uploader?.display_name || track.uploader?.email || ""}</span>
                  <span>{formatRelativeTime(track.created_at)}</span>
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
              <div class="flex items-center gap-4 p-4">
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

                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2 flex-wrap">
                    <button
                      onclick={() => navigate(`/band/${slug}/${take.set_id ? "set/" + take.set_id : "take/" + take.track_id}`)}
                      class="text-sm font-semibold text-text-primary hover:text-accent transition-colors truncate text-left font-display tracking-wide"
                    >{take.track_title}</button>
                    <button
                      onclick={() => navigate(`/band/${slug}/set/${take.set_id}`)}
                      class="label-sm text-text-muted bg-bg-primary border border-border px-1.5 py-0.5 hover:text-accent hover:border-accent/40 transition-colors"
                    >{take.set_name}</button>
                    <span class="label-sm text-accent bg-accent/10 px-1.5 py-0.5 font-mono">{setTypeCode(take.set_type)}</span>
                    {#if take.tags?.length}
                      {#each take.tags as tag}
                        <span class="label-sm text-accent bg-accent/10 px-1.5 py-0.5">{tag}</span>
                      {/each}
                    {/if}
                  </div>
                  <div class="flex items-center gap-2 mt-1 flex-wrap">
                    <span class="label-sm text-text-muted font-mono">{formatDuration(take.start_ms)} — {formatDuration(take.end_ms)}</span>
                    <button
                      onclick={() => toggleTakeComments(take)}
                      class="label-sm text-text-muted hover:text-accent transition-colors"
                    >{isExpanded ? "hide comments" : "comments"}</button>
                  </div>
                </div>

                <div class="flex items-center gap-3 label-sm text-text-muted shrink-0">
                  <span class="font-mono">{formatDuration(takeDurationMs)}</span>
                  <span>{take.uploader?.display_name || take.uploader?.email || ""}</span>
                  <span>{formatRelativeTime(take.created_at)}</span>
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
                      <span class="label-sm text-marker bg-marker/10 px-2 py-2 font-mono shrink-0">
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
        <h2 class="label text-text-muted mb-3"><span class="text-accent/15 font-mono mr-2">04</span>sets</h2>
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
                <span class="label-sm text-accent bg-accent/10 px-1.5 py-0.5 shrink-0 font-mono">{setTypeCode(s.set_type)}</span>
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
