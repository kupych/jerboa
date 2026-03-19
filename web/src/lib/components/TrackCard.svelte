<script lang="ts">
  import { navigate } from "../stores/router";
  import { formatDuration, formatRelativeTime, formatFileSize } from "../utils/format";

  let { track, bandSlug }: {
    track: {
      id: string;
      title: string;
      description?: string;
      duration_ms: number;
      format: string;
      file_size: number;
      status: string;
      created_at: string;
      uploader?: { display_name: string; email: string };
    };
    bandSlug: string;
  } = $props();

  function open() {
    navigate(`/band/${bandSlug}/track/${track.id}`);
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="bg-bg-surface border border-border p-5 hover:border-accent/40 transition-colors cursor-pointer group"
  onclick={open}
>
  <div class="flex items-start justify-between gap-4">
    <div class="min-w-0 flex-1">
      <h3 class="text-sm font-semibold tracking-wider text-text-primary truncate group-hover:text-accent transition-colors">
        {track.title}
      </h3>
      {#if track.description}
        <p class="text-xs text-text-muted mt-1 truncate">{track.description}</p>
      {/if}
    </div>

    {#if track.status === "processing"}
      <div class="flex items-center gap-2 text-xs tracking-wider uppercase text-accent">
        <div class="w-1.5 h-1.5 bg-accent animate-pulse"></div>
        processing
      </div>
    {:else if track.status === "error"}
      <div class="text-xs tracking-wider uppercase text-danger">error</div>
    {/if}
  </div>

  <div class="flex items-center gap-3 mt-4 text-[11px] tracking-wider uppercase text-text-muted">
    {#if track.status === "ready"}
      <span class="font-mono">{formatDuration(track.duration_ms)}</span>
      <span class="text-border">/</span>
    {/if}
    <span>{track.format}</span>
    <span class="text-border">/</span>
    <span>{formatFileSize(track.file_size)}</span>
    <span class="text-border">/</span>
    <span>{track.uploader?.display_name || track.uploader?.email || "unknown"}</span>
    <span class="ml-auto">{formatRelativeTime(track.created_at)}</span>
  </div>
</div>
