<script lang="ts">
  import { onDestroy } from "svelte";
  import { formatDuration } from "../utils/format";

  let {
    src,
    peaks: propPeaks = [],
    duration_ms: propDuration = 0,
  }: {
    src: string;
    peaks?: number[];
    duration_ms?: number;
  } = $props();

  let canvas = $state<HTMLCanvasElement | null>(null);
  let audio: HTMLAudioElement | null = null;
  let playing = $state(false);
  let currentMs = $state(0);
  let localDuration = $state(propDuration);
  let localPeaks = $state<number[]>(propPeaks);
  let peaksLoading = $state(false);

  let playheadPct = $derived(localDuration > 0 ? (currentMs / localDuration) * 100 : 0);
  let hasPeaks = $derived(localPeaks.length > 0);

  // Draw whenever canvas mounts, peaks arrive, or playhead moves
  $effect(() => {
    if (!canvas || !hasPeaks) return;
    drawWaveform(localDuration > 0 ? currentMs / localDuration : 0);
  });

  function drawWaveform(playedFrac: number) {
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const w = canvas.width, h = canvas.height;
    ctx.clearRect(0, 0, w, h);

    const style = getComputedStyle(document.documentElement);
    const active = style.getPropertyValue("--color-waveform-progress").trim() || "#5b9cf6";
    const dim = style.getPropertyValue("--color-waveform").trim() || "#334155";

    const barW = w / localPeaks.length;
    for (let i = 0; i < localPeaks.length; i++) {
      ctx.fillStyle = i / localPeaks.length <= playedFrac ? active : dim;
      const bh = Math.max(2, localPeaks[i] * h * 0.85);
      const y = (h - bh) / 2;
      ctx.fillRect(Math.floor(i * barW), Math.floor(y), Math.max(1, Math.ceil(barW) - 1), Math.ceil(bh));
    }
  }

  async function computePeaks() {
    if (peaksLoading || localPeaks.length > 0) return;
    peaksLoading = true;
    try {
      const res = await fetch(src, { cache: "no-cache" });
      const ab = await res.arrayBuffer();
      const actx = new AudioContext();
      const buf = await actx.decodeAudioData(ab);
      actx.close();

      localDuration = buf.duration * 1000;

      const n = 60;
      const ch = buf.getChannelData(0);
      const blockSize = Math.floor(ch.length / n);
      const peaks: number[] = [];
      for (let i = 0; i < n; i++) {
        let max = 0;
        const start = i * blockSize;
        for (let j = 0; j < blockSize; j++) {
          const v = Math.abs(ch[start + j] || 0);
          if (v > max) max = v;
        }
        peaks.push(max);
      }
      localPeaks = peaks;
    } catch { /* ignore */ } finally {
      peaksLoading = false;
    }
  }

  function ensureAudio(): HTMLAudioElement {
    if (!audio) {
      audio = new Audio(src);
      audio.addEventListener("loadedmetadata", () => {
        if (audio) localDuration = audio.duration * 1000;
      });
      audio.addEventListener("timeupdate", () => {
        currentMs = (audio?.currentTime ?? 0) * 1000;
        if (hasPeaks) drawWaveform(localDuration > 0 ? currentMs / localDuration : 0);
      });
      audio.addEventListener("ended", () => {
        playing = false;
        currentMs = 0;
        if (hasPeaks) drawWaveform(0);
      });
    }
    return audio;
  }

  function togglePlay(e: MouseEvent) {
    e.stopPropagation();
    const a = ensureAudio();
    if (!hasPeaks) computePeaks();
    if (playing) {
      a.pause();
      playing = false;
    } else {
      a.play();
      playing = true;
    }
  }

  function handleSeek(e: MouseEvent) {
    e.stopPropagation();
    if (localDuration <= 0) return;
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const frac = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    currentMs = frac * localDuration;
    const a = ensureAudio();
    a.currentTime = currentMs / 1000;
    if (hasPeaks) drawWaveform(frac);
    if (!playing) { a.play(); playing = true; }
  }

  onDestroy(() => { audio?.pause(); });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="flex items-center gap-2 mt-2" onclick={(e) => e.stopPropagation()}>
  <button
    onclick={togglePlay}
    class="w-6 h-6 flex items-center justify-center bg-accent/10 hover:bg-accent/20 text-accent transition-colors shrink-0"
  >
    {#if peaksLoading}
      <div class="w-2.5 h-2.5 border border-accent/60 border-t-accent animate-spin"></div>
    {:else if playing}
      <svg width="8" height="8" viewBox="0 0 12 12" fill="currentColor">
        <rect x="1" y="1" width="3.5" height="10"/><rect x="7.5" y="1" width="3.5" height="10"/>
      </svg>
    {:else}
      <svg width="8" height="8" viewBox="0 0 12 12" fill="currentColor">
        <polygon points="2,0 12,6 2,12"/>
      </svg>
    {/if}
  </button>

  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="flex-1 min-w-0 relative h-8 cursor-pointer" onclick={handleSeek}>
    {#if hasPeaks}
      <canvas bind:this={canvas} width="300" height="32" class="w-full h-full block"></canvas>
      <div class="absolute top-0 bottom-0 w-px bg-white/50 pointer-events-none" style="left: {playheadPct}%"></div>
    {:else}
      <div class="w-full h-px bg-border/40 absolute top-1/2 -translate-y-1/2"></div>
    {/if}
  </div>

  <span class="label-sm font-mono text-text-muted/50 shrink-0 tabular-nums w-24 whitespace-nowrap text-right">
    {formatDuration(currentMs)} / {formatDuration(localDuration)}
  </span>
</div>
