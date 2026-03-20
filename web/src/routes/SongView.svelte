<script lang="ts">
  import { onMount } from "svelte";
  import { marked } from "marked";
  import { api, apiPatch } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, formatRelativeTime } from "../lib/utils/format";

  marked.setOptions({ breaks: true, gfm: true });

  let { slug, songId }: { slug: string; songId: string } = $props();

  type Song = {
    id: string;
    name: string;
    lyrics: string;
    tabs: string;
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
  };

  let song = $state<Song | null>(null);
  let tracks = $state<Track[]>([]);

  let editingLyrics = $state(false);
  let editingTabs = $state(false);
  let lyricsInput = $state("");
  let tabsInput = $state("");
  let saving = $state(false);

  let playingId = $state<string | null>(null);
  let audioEl = $state<HTMLAudioElement | null>(null);
  let currentTime = $state(0);
  let audioDuration = $state(0);

  function togglePlay(trackId: string) {
    if (playingId === trackId) {
      if (audioEl?.paused) {
        audioEl.play();
      } else {
        audioEl?.pause();
      }
      return;
    }
    audioEl?.pause();
    playingId = trackId;
    currentTime = 0;
    audioDuration = 0;
  }

  function onTimeUpdate(e: Event) {
    const el = e.target as HTMLAudioElement;
    currentTime = el.currentTime;
    audioDuration = el.duration || 0;
  }

  function onEnded() {
    playingId = null;
    currentTime = 0;
  }

  function seek(e: MouseEvent, track: Track) {
    const bar = e.currentTarget as HTMLElement;
    const rect = bar.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    const dur = track.duration_ms / 1000;
    if (audioEl && playingId === track.id) {
      audioEl.currentTime = pct * dur;
    }
  }

  function formatSecs(s: number): string {
    const m = Math.floor(s / 60);
    const sec = Math.floor(s % 60);
    return `${m}:${sec.toString().padStart(2, "0")}`;
  }

  onMount(() => {
    loadSong();
    loadTracks();
  });

  async function loadSong() {
    song = await api<Song>(`/api/bands/${slug}/songs/${songId}`);
    lyricsInput = song.lyrics;
    tabsInput = song.tabs;
  }

  async function loadTracks() {
    const all = await api<Track[]>(`/api/bands/${slug}/tracks`);
    tracks = all.filter((t) => t.song_id === songId);
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
    <!-- Header -->
    <div class="flex items-center gap-4">
      <button onclick={() => navigate(`/band/${slug}`)} class="label text-text-muted hover:text-accent transition-colors">&larr; back</button>
      <h1 class="text-2xl font-display font-bold tracking-wider text-text-primary">{song.name}</h1>
    </div>

    <!-- Lyrics -->
    <section>
      <div class="flex items-center justify-between mb-3">
        <h2 class="label text-text-muted">lyrics</h2>
        {#if !editingLyrics}
          <button
            onclick={() => { editingLyrics = true; lyricsInput = song!.lyrics; }}
            class="label-sm text-text-muted hover:text-accent transition-colors"
          >edit</button>
        {/if}
      </div>

      {#if editingLyrics}
        <div class="space-y-3">
          <textarea
            bind:value={lyricsInput}
            rows="16"
            class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y"
            placeholder="supports markdown..."
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
        <div class="prose bg-bg-surface border border-border p-5">{@html marked.parse(song.lyrics)}</div>
      {:else}
        <p class="text-sm text-text-muted italic">no lyrics yet</p>
      {/if}
    </section>

    <!-- Tabs -->
    <section>
      <div class="flex items-center justify-between mb-3">
        <h2 class="label text-text-muted">tabs / chords</h2>
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
            placeholder="paste tabs or chord charts here..."
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
        <pre class="text-sm text-text-secondary whitespace-pre-wrap font-mono leading-relaxed bg-bg-surface border border-border p-5">{song.tabs}</pre>
      {:else}
        <p class="text-sm text-text-muted italic">no tabs yet</p>
      {/if}
    </section>

    <!-- Takes -->
    {#if tracks.length > 0}
      <section>
        <h2 class="label text-text-muted mb-3">takes</h2>
        <div class="space-y-2">
          {#each tracks as track}
            <div class="bg-bg-surface border border-border">
              <div class="flex items-center gap-4 p-4">
                <!-- Play button -->
                {#if track.status === "ready"}
                  <button
                    onclick={() => togglePlay(track.id)}
                    class="w-8 h-8 flex items-center justify-center text-text-muted hover:text-accent transition-colors shrink-0"
                  >
                    {#if playingId === track.id && audioEl && !audioEl.paused}
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

                <!-- Track info -->
                <div class="flex-1 min-w-0">
                  <button
                    onclick={() => navigate(`/band/${slug}/track/${track.id}`)}
                    class="text-sm font-semibold text-text-primary hover:text-accent transition-colors truncate block text-left font-display tracking-wide"
                  >{track.title}</button>
                </div>

                <!-- Meta -->
                <div class="flex items-center gap-3 label-sm text-text-muted shrink-0">
                  {#if track.status === "ready"}
                    <span class="font-mono">{formatDuration(track.duration_ms)}</span>
                  {/if}
                  <span>{track.uploader?.display_name || track.uploader?.email || ""}</span>
                  <span>{formatRelativeTime(track.created_at)}</span>
                </div>
              </div>

              <!-- Progress bar -->
              {#if playingId === track.id && track.status === "ready"}
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="h-1 bg-bg-primary cursor-pointer"
                  onclick={(e) => seek(e, track)}
                >
                  <div
                    class="h-full bg-accent transition-[width] duration-100"
                    style="width: {audioDuration > 0 ? (currentTime / audioDuration) * 100 : 0}%"
                  ></div>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Hidden audio element -->
    {#if playingId}
      <audio
        bind:this={audioEl}
        src={`/api/bands/${slug}/tracks/${playingId}/stream`}
        autoplay
        ontimeupdate={onTimeUpdate}
        onended={onEnded}
      ></audio>
    {/if}
  </div>
{:else}
  <div class="label text-text-muted">loading...</div>
{/if}
