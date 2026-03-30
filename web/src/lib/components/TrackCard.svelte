<script lang="ts">
  import { navigate } from "../stores/router";
  import { apiPost, apiDelete } from "../api";
  import { formatDuration, formatRelativeTime, formatFileSize } from "../utils/format";
  import { globalPlayer, playerState } from "../stores/globalPlayer";

  let { track, bandSlug, onRetry, onDelete }: {
    track: {
      id: string;
      title: string;
      description?: string;
      duration_ms: number;
      format: string;
      file_size: number;
      status: string;
      tags: string[];
      source_url?: string;
      sample_rate?: number;
      recorded_at?: string;
      created_at: string;
      uploader?: { display_name: string; email: string };
    };
    bandSlug: string;
    onRetry?: () => void;
    onDelete?: () => void;
  } = $props();

  let retrying = $state(false);
  let deleting = $state(false);

  const prefetched = new Set<string>();
  function prefetch() {
    if (track.status !== "ready") return;
    const url = `/api/bands/${bandSlug}/tracks/${track.id}/stream`;
    if (prefetched.has(url)) return;
    prefetched.add(url);
    fetch(url, { priority: "low" } as RequestInit).catch(() => {});
  }

  function openAndPrefetch() {
    prefetch();
    open();
  }

  function open() {
    if (track.status === "error") return;
    navigate(`/band/${bandSlug}/track/${track.id}`);
  }

  async function retry(e: Event) {
    e.stopPropagation();
    retrying = true;
    try {
      await apiPost(`/api/bands/${bandSlug}/tracks/${track.id}/retry`, {});
      track.status = "processing";
      onRetry?.();
    } finally {
      retrying = false;
    }
  }

  async function remove(e: Event) {
    e.stopPropagation();
    deleting = true;
    try {
      await apiDelete(`/api/bands/${bandSlug}/tracks/${track.id}`);
      onDelete?.();
    } finally {
      deleting = false;
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="bg-bg-surface border border-border px-4 py-3 transition-colors {track.status === 'error' ? '' : 'hover:border-accent/40 cursor-pointer'} group"
  onclick={openAndPrefetch}
  onmouseenter={prefetch}
>
  <div class="flex items-start justify-between gap-4">
    {#if track.status === "ready"}
      <div class="flex items-center shrink-0 mt-0.5">
        <button
          onclick={(e) => {
            e.stopPropagation();
            globalPlayer.play({ id: track.id, title: track.title, bandSlug });
          }}
          class="w-8 h-8 flex items-center justify-center text-text-muted hover:text-accent transition-colors"
        >
          {#if $playerState.track?.id === track.id && $playerState.playing}
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="4" width="4" height="16"/>
              <rect x="14" y="4" width="4" height="16"/>
            </svg>
          {:else}
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="5,3 19,12 5,21"/>
            </svg>
          {/if}
        </button>
        <button
          onclick={(e) => {
            e.stopPropagation();
            globalPlayer.enqueue({ id: track.id, title: track.title, bandSlug });
          }}
          class="w-6 h-6 flex items-center justify-center text-text-muted/0 group-hover:text-text-muted/40 hover:!text-accent transition-colors"
          title="Add to queue"
        >
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <line x1="12" y1="5" x2="12" y2="19"/>
            <line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
        </button>
      </div>
    {/if}
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-3">
        <h3 class="text-base font-semibold tracking-wider text-text-primary truncate group-hover:text-accent transition-colors font-display">
          {track.title}
        </h3>
        {#if track.tags?.length}
          <div class="flex gap-2">
            {#each track.tags as tag}
              <span class="label-sm text-accent border border-accent/30 px-2 py-0.5">{tag}</span>
            {/each}
          </div>
        {/if}
      </div>
      {#if track.description}
        <p class="text-sm font-medium text-text-muted mt-2 truncate">{track.description}</p>
      {/if}
    </div>

    {#if track.status === "processing"}
      <div class="flex items-center gap-2 label-sm text-accent">
        <div class="w-1.5 h-1.5 bg-accent animate-pulse"></div>
        processing
      </div>
    {:else if track.status === "error"}
      <div class="flex items-center gap-3">
        <span class="label-sm text-danger">error</span>
        {#if track.source_url}
          <button
            onclick={retry}
            disabled={retrying}
            class="label-sm text-text-muted hover:text-accent transition-colors"
          >{retrying ? "..." : "retry"}</button>
        {/if}
        <button
          onclick={remove}
          disabled={deleting}
          class="label-sm text-text-muted hover:text-red-400 transition-colors"
        >{deleting ? "..." : "delete"}</button>
      </div>
    {/if}
  </div>

  <div class="flex items-center gap-3 mt-2 label-sm text-text-muted">
    {#if track.status === "ready"}
      <span class="font-mono">{formatDuration(track.duration_ms)}</span>
      <span class="vr-divider">/</span>
    {/if}
    <span>{track.format}{#if track.sample_rate} · {track.sample_rate >= 1000 ? `${(track.sample_rate / 1000).toFixed(track.sample_rate % 1000 ? 1 : 0)}kHz` : `${track.sample_rate}Hz`}{/if}</span>
    <span class="vr-divider">/</span>
    <span>{formatFileSize(track.file_size)}</span>
    <span class="vr-divider">/</span>
    <span>{track.uploader?.display_name || track.uploader?.email || "unknown"}</span>
    {#if track.recorded_at && !track.title.includes(track.recorded_at.slice(0, 10))}
      <span class="vr-divider">/</span>
      <span>rec {new Date(track.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
    {/if}
    <span class="ml-auto">{formatRelativeTime(track.created_at)}</span>
  </div>
</div>
