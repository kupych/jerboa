<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import WaveSurfer from "wavesurfer.js";
  import { formatTimestamp, formatDuration } from "../utils/format";
  import { player } from "../stores/player";

  let {
    src,
    peaks = undefined,
    duration = 0,
    comments = [],
    regions = [],
    clickToTag = false,
    onTimestampClick = undefined,
  }: {
    src: string;
    peaks?: number[];
    duration?: number;
    comments?: Array<{ id: string; timestamp_ms: number; body: string; user_name: string }>;
    regions?: Array<{ start_ms: number; end_ms: number; label: string; color?: string }>;
    clickToTag?: boolean;
    onTimestampClick?: (ms: number) => void;
  } = $props();

  let container: HTMLDivElement;
  let wavesurfer: WaveSurfer | null = null;
  let isPlaying = $state(false);
  let currentTime = $state(0);
  let totalDuration = $state(duration / 1000);
  let hoveredComment = $state<string | null>(null);

  onMount(() => {
    wavesurfer = WaveSurfer.create({
      container,
      waveColor: getComputedStyle(document.documentElement)
        .getPropertyValue("--color-waveform")
        .trim(),
      progressColor: getComputedStyle(document.documentElement)
        .getPropertyValue("--color-waveform-progress")
        .trim(),
      cursorColor: getComputedStyle(document.documentElement)
        .getPropertyValue("--color-accent")
        .trim(),
      barWidth: 2,
      barGap: 1,
      barRadius: 0,
      height: 160,
      normalize: true,
      backend: "MediaElement",
    });

    if (peaks && peaks.length > 0) {
      wavesurfer.load(src, [peaks], duration / 1000);
    } else {
      wavesurfer.load(src);
    }

    wavesurfer.on("play", () => (isPlaying = true));
    wavesurfer.on("pause", () => (isPlaying = false));
    wavesurfer.on("timeupdate", (t) => (currentTime = t));
    wavesurfer.on("decode", (d) => (totalDuration = d));

    // Register media element for global keyboard shortcuts
    player.setMediaElement(wavesurfer.getMediaElement());
  });

  onDestroy(() => {
    player.setMediaElement(null);
    wavesurfer?.destroy();
  });

  function togglePlay() {
    wavesurfer?.playPause();
  }

  function handleWaveformClick(e: MouseEvent) {
    if (!onTimestampClick || !wavesurfer || totalDuration <= 0) return;
    // Calculate position from click coordinates
    const rect = container.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const ratio = Math.max(0, Math.min(1, x / rect.width));
    const ms = Math.round(ratio * totalDuration * 1000);
    if (clickToTag || e.altKey) {
      onTimestampClick(ms);
    }
  }

  export function seekTo(ms: number) {
    if (wavesurfer && totalDuration > 0) {
      wavesurfer.seekTo(ms / 1000 / totalDuration);
    }
  }

  function seekToComment(ms: number) {
    seekTo(ms);
  }

  function commentPosition(ms: number): number {
    if (totalDuration <= 0) return 0;
    return (ms / 1000 / totalDuration) * 100;
  }

  function regionLeft(ms: number): number {
    if (totalDuration <= 0) return 0;
    return (ms / 1000 / totalDuration) * 100;
  }

  function regionWidth(startMs: number, endMs: number): number {
    if (totalDuration <= 0) return 0;
    return ((endMs - startMs) / 1000 / totalDuration) * 100;
  }

  let hoveredRegion = $state<number | null>(null);
  let hoverX = $state<number | null>(null);
  let hoverMs = $state(0);

  function handleWaveformMouseMove(e: MouseEvent) {
    if (!clickToTag || totalDuration <= 0) { hoverX = null; return; }
    const rect = container.getBoundingClientRect();
    const x = e.clientX - rect.left;
    hoverX = (x / rect.width) * 100;
    hoverMs = Math.round((x / rect.width) * totalDuration * 1000);
  }

  function handleWaveformMouseLeave() {
    hoverX = null;
  }
</script>

<div class="bg-bg-surface border border-border p-6">
  <!-- Transport -->
  <div class="flex items-center gap-5 mb-5">
    <button
      onclick={togglePlay}
      class="w-10 h-10 flex items-center justify-center bg-accent hover:bg-accent-hover text-bg-primary transition-colors"
    >
      {#if isPlaying}
        <svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor">
          <rect x="1" y="1" width="3.5" height="10"/>
          <rect x="7.5" y="1" width="3.5" height="10"/>
        </svg>
      {:else}
        <svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor">
          <polygon points="2,0 12,6 2,12"/>
        </svg>
      {/if}
    </button>

    <div class="label-sm text-text-secondary font-mono tabular-nums">
      {formatTimestamp(currentTime * 1000)}
      <span class="text-text-muted mx-1">/</span>
      {formatDuration(totalDuration * 1000)}
    </div>

    {#if onTimestampClick && !clickToTag}
      <button
        onclick={() => onTimestampClick!(Math.round(currentTime * 1000))}
        class="ml-auto label text-marker hover:text-marker-hover transition-colors flex items-center gap-2"
        title="Add comment at current position"
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
        </svg>
        comment
      </button>
    {/if}
  </div>

  <!-- Waveform -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="relative {clickToTag ? 'cursor-crosshair' : ''}"
    onclick={handleWaveformClick}
    onmousemove={handleWaveformMouseMove}
    onmouseleave={handleWaveformMouseLeave}
  >
    <div bind:this={container} class="w-full"></div>

    <!-- Hover cursor line for tagging -->
    {#if hoverX != null && clickToTag}
      <div
        class="absolute top-0 h-full w-0.5 bg-orange-400 pointer-events-none z-20"
        style="left: {hoverX}%"
      >
        <div class="absolute -top-5 left-1/2 -translate-x-1/2 label-sm font-mono text-orange-400 whitespace-nowrap" style="font-size: 10px;">
          {formatDuration(hoverMs)}
        </div>
      </div>
    {/if}

    {#each regions as region, ri}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <div
        class="absolute top-0 h-full cursor-pointer transition-opacity"
        style="left: {regionLeft(region.start_ms)}%; width: {regionWidth(region.start_ms, region.end_ms)}%; background: {region.color || 'rgba(99,102,241,0.25)'};"
        onmouseenter={() => (hoveredRegion = ri)}
        onmouseleave={() => (hoveredRegion = null)}
        onclick={(e) => { e.stopPropagation(); seekToComment(region.start_ms); }}
      >
        <div class="absolute -top-5 left-1 label-sm text-text-secondary truncate max-w-full" style="font-size: 10px;">{region.label}</div>
        {#if hoveredRegion === ri}
          <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-3 px-3 py-2 bg-bg-elevated border border-border whitespace-nowrap z-10">
            <div class="label-sm text-text-primary">{region.label}</div>
            <div class="text-text-muted label-sm font-mono mt-1">{formatDuration(region.start_ms)} &mdash; {formatDuration(region.end_ms)}</div>
          </div>
        {/if}
      </div>
    {/each}

    {#each comments as comment}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <div
        class="absolute top-0 h-full w-0.5 bg-marker/50 hover:bg-marker cursor-pointer transition-colors"
        style="left: {commentPosition(comment.timestamp_ms)}%"
        onmouseenter={() => (hoveredComment = comment.id)}
        onmouseleave={() => (hoveredComment = null)}
        onclick={(e) => { e.stopPropagation(); seekToComment(comment.timestamp_ms); }}
      >
        <div class="absolute -top-1 -left-[3px] w-2 h-2 bg-marker"></div>

        {#if hoveredComment === comment.id}
          <div class="absolute bottom-full left-1/2 -translate-x-1/2 mb-3 px-4 py-3 bg-bg-elevated border border-border whitespace-nowrap z-10">
            <div class="label-sm text-text-secondary">{comment.user_name}</div>
            <div class="text-sm font-medium text-text-primary mt-1 max-w-48 truncate">{comment.body}</div>
            <div class="text-text-muted label-sm font-mono mt-1">{formatTimestamp(comment.timestamp_ms)}</div>
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <!-- Axis labels -->
  <div class="flex justify-between mt-1 select-none">
    <span class="text-[8px] font-mono font-semibold text-text-muted/30 tracking-wider">0:00</span>
    <span class="text-[8px] font-mono font-semibold text-text-muted/30 tracking-wider">{formatDuration(totalDuration * 1000)}</span>
  </div>
</div>
