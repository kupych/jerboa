<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPatch } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import TrackCard from "../lib/components/TrackCard.svelte";

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
            class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors font-mono resize-y"
            placeholder="paste lyrics here..."
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
        <pre class="text-sm text-text-secondary whitespace-pre-wrap font-mono leading-relaxed bg-bg-surface border border-border p-5">{song.lyrics}</pre>
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

    <!-- Tracks / Takes -->
    {#if tracks.length > 0}
      <section>
        <h2 class="label text-text-muted mb-3">takes</h2>
        <div class="space-y-3">
          {#each tracks as track}
            <TrackCard {track} bandSlug={slug} onDelete={loadTracks} />
          {/each}
        </div>
      </section>
    {/if}
  </div>
{:else}
  <div class="label text-text-muted">loading...</div>
{/if}
