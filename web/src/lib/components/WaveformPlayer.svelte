<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import WaveSurfer from "wavesurfer.js";
  import { formatTimestamp, formatDuration } from "../utils/format";

  let {
    src,
    peaks = undefined,
    duration = 0,
    comments = [],
    onTimestampClick = undefined,
  }: {
    src: string;
    peaks?: number[];
    duration?: number;
    comments?: Array<{ id: string; timestamp_ms: number; body: string; user_name: string }>;
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
  });

  onDestroy(() => {
    wavesurfer?.destroy();
  });

  function togglePlay() {
    wavesurfer?.playPause();
  }

  function handleWaveformClick(e: MouseEvent) {
    if (!onTimestampClick || !wavesurfer) return;
    if (e.altKey) {
      const ms = Math.round(currentTime * 1000);
      onTimestampClick(ms);
    }
  }

  function seekToComment(ms: number) {
    if (wavesurfer && totalDuration > 0) {
      wavesurfer.seekTo(ms / 1000 / totalDuration);
    }
  }

  function commentPosition(ms: number): number {
    if (totalDuration <= 0) return 0;
    return (ms / 1000 / totalDuration) * 100;
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

    {#if onTimestampClick}
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
  <div class="relative" onclick={handleWaveformClick}>
    <div bind:this={container} class="w-full"></div>

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
</div>
