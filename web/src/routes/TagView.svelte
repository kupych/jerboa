<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, formatRelativeTime } from "../lib/utils/format";

  let { slug, tag }: { slug: string; tag: string } = $props();

  interface Track {
    id: string;
    title: string;
    duration_ms: number;
    status: string;
    tags: string[];
    created_at: string;
    song_id?: string;
    song?: { id: string; name: string };
    uploader?: { display_name: string; email: string };
  }

  let tracks = $state<Track[]>([]);
  let loading = $state(true);

  onMount(async () => {
    try {
      const all = await api<Track[]>(`/api/bands/${slug}/tracks`);
      tracks = all.filter((t) => t.tags?.includes(tag));
    } finally {
      loading = false;
    }
  });

  interface Group { songName: string | null; songId: string | null; tracks: Track[] }

  // Group by song; ungrouped tracks go under null
  let grouped = $derived(
    (() => {
      const map = new Map<string, Group>();
      const unsorted: Track[] = [];
      for (const t of tracks) {
        if (t.song_id && t.song) {
          if (!map.has(t.song_id)) map.set(t.song_id, { songName: t.song.name, songId: t.song_id, tracks: [] });
          map.get(t.song_id)!.tracks.push(t);
        } else {
          unsorted.push(t);
        }
      }
      const result: Group[] = [...map.values()].sort((a, b) => (a.songName ?? "").localeCompare(b.songName ?? ""));
      if (unsorted.length) result.push({ songName: null, songId: null, tracks: unsorted });
      return result;
    })()
  );
</script>

<div class="max-w-2xl mx-auto px-4 py-6 space-y-6">
  <!-- Header -->
  <div class="flex items-center gap-3">
    <button
      onclick={() => navigate(`/band/${slug}`)}
      class="text-text-muted hover:text-accent transition-colors"
    >
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="15 18 9 12 15 6"/>
      </svg>
    </button>
    <div>
      <div class="label-sm text-text-muted mb-0.5">tag</div>
      <h1 class="text-xl font-bold tracking-wider font-display text-text-primary">{tag}</h1>
    </div>
    {#if !loading}
      <span class="label-sm text-text-muted ml-auto">{tracks.length} {tracks.length === 1 ? "take" : "takes"}</span>
    {/if}
  </div>

  {#if loading}
    <div class="flex justify-center py-12">
      <div class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></div>
    </div>
  {:else if tracks.length === 0}
    <div class="text-center py-12 label text-text-muted">no takes with this tag</div>
  {:else}
    <div class="space-y-6">
      {#each grouped as group}
        <div>
          {#if group.songName}
            <button
              onclick={() => navigate(`/band/${slug}/song/${group.songId}`)}
              class="label text-text-muted hover:text-accent transition-colors mb-2 block"
            >{group.songName}</button>
          {:else}
            <div class="label text-text-muted mb-2">unassigned</div>
          {/if}
          <div class="space-y-1">
            {#each group.tracks as track}
              <div class="bg-bg-surface border border-border flex flex-col px-4 py-3 gap-1">
                <button
                  onclick={() => navigate(`/band/${slug}/track/${track.id}`)}
                  class="text-sm font-semibold font-display tracking-wide text-text-primary hover:text-accent transition-colors text-left truncate"
                >{track.title}</button>
                <div class="flex items-center gap-3 label-sm text-text-muted flex-wrap">
                  {#if track.status === "ready"}
                    <span class="font-mono">{formatDuration(track.duration_ms)}</span>
                  {:else}
                    <span class="text-accent/60">processing</span>
                  {/if}
                  <span>{track.uploader?.display_name || track.uploader?.email || ""}</span>
                  <span>{formatRelativeTime(track.created_at)}</span>
                  {#each track.tags.filter((t) => t !== tag) as otherTag}
                    <button
                      onclick={() => navigate(`/band/${slug}/tag/${encodeURIComponent(otherTag)}`)}
                      class="border border-border px-1.5 py-0.5 hover:border-accent/50 hover:text-accent transition-colors"
                    >{otherTag}</button>
                  {/each}
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
