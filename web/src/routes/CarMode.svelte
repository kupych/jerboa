<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { formatDuration } from "../lib/utils/format";

  let { slug }: { slug: string } = $props();

  interface Track {
    id: string;
    title: string;
    duration_ms: number;
    format: string;
    status: string;
    song_id?: string;
    song?: { id: string; name: string };
    created_at: string;
  }

  let tracks = $state<Track[]>([]);
  let currentIndex = $state(0);
  let audio: HTMLAudioElement | null = $state(null);
  let isPlaying = $state(false);
  let currentTime = $state(0);
  let duration = $state(0);
  let loading = $state(true);

  let currentTrack = $derived(tracks[currentIndex]);
  let streamUrl = $derived(currentTrack ? `/api/bands/${slug}/tracks/${currentTrack.id}/stream` : "");

  onMount(async () => {
    const all = await api<Track[]>(`/api/bands/${slug}/tracks`);
    tracks = all.filter((t) => t.status === "ready");
    loading = false;
  });

  function bindAudio(el: HTMLAudioElement) {
    audio = el;
    el.addEventListener("timeupdate", () => (currentTime = el.currentTime));
    el.addEventListener("durationchange", () => (duration = el.duration));
    el.addEventListener("play", () => (isPlaying = true));
    el.addEventListener("pause", () => (isPlaying = false));
    el.addEventListener("ended", playNext);
    return { destroy() { audio = null; } };
  }

  function toggle() {
    if (!audio) return;
    if (audio.paused) audio.play();
    else audio.pause();
  }

  function playNext() {
    currentIndex = (currentIndex + 1) % tracks.length;
    playFromStart();
  }

  function playPrev() {
    // If more than 3s in, restart current track
    if (audio && audio.currentTime > 3) {
      audio.currentTime = 0;
      return;
    }
    if (currentIndex > 0) {
      currentIndex--;
      playFromStart();
    }
  }

  function playTrack(index: number) {
    currentIndex = index;
    playFromStart();
  }

  function playFromStart() {
    // Wait for src to update, then play
    requestAnimationFrame(() => {
      if (audio) {
        audio.load();
        audio.play();
      }
    });
  }

  function seekTo(e: MouseEvent) {
    if (!audio || !duration) return;
    const bar = e.currentTarget as HTMLElement;
    const rect = bar.getBoundingClientRect();
    const ratio = (e.clientX - rect.left) / rect.width;
    audio.currentTime = ratio * duration;
  }

  let progressPct = $derived(duration > 0 ? (currentTime / duration) * 100 : 0);

  // Keep screen awake
  let wakeLock: WakeLockSentinel | null = null;
  onMount(async () => {
    try {
      if ("wakeLock" in navigator) {
        wakeLock = await navigator.wakeLock.request("screen");
      }
    } catch {}
  });
  onDestroy(() => { wakeLock?.release(); });

  // Media Session API — bluetooth/lockscreen controls
  $effect(() => {
    if (!currentTrack || !("mediaSession" in navigator)) return;
    navigator.mediaSession.metadata = new MediaMetadata({
      title: currentTrack.title,
      artist: currentTrack.song?.name || slug,
      album: slug,
    });
    navigator.mediaSession.setActionHandler("play", () => audio?.play());
    navigator.mediaSession.setActionHandler("pause", () => audio?.pause());
    navigator.mediaSession.setActionHandler("previoustrack", playPrev);
    navigator.mediaSession.setActionHandler("nexttrack", playNext);
    navigator.mediaSession.setActionHandler("seekto", (details) => {
      if (audio && details.seekTime != null) audio.currentTime = details.seekTime;
    });
  });
</script>

{#if loading}
  <div class="flex items-center justify-center min-h-[80vh]">
    <div class="flex items-center gap-2 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
  </div>
{:else if tracks.length === 0}
  <div class="flex items-center justify-center min-h-[80vh]">
    <div class="label text-text-muted">no tracks</div>
  </div>
{:else}
  <!-- Hidden audio element -->
  {#key currentIndex}
    <audio src={streamUrl} preload="auto" use:bindAudio></audio>
  {/key}

  <div class="flex flex-col min-h-[80vh]">
    <!-- Exit -->
    <div class="mb-6">
      <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5">
        <button onclick={() => navigate(`/band/${slug}`)} class="hover:text-accent/60 transition-colors py-1">{slug.toUpperCase()}</button>
        <span>/</span>
        <span class="text-text-muted/50">CAR</span>
      </div>
    </div>

    <!-- Now playing -->
    <div class="flex-1 flex flex-col items-center justify-center gap-8 mb-8">
      <div class="text-center">
        <div class="text-[10px] font-mono font-semibold tracking-[0.3em] text-text-muted/40 uppercase mb-3">
          {currentIndex + 1} / {tracks.length}
        </div>
        <h1 class="text-2xl md:text-4xl font-display font-bold tracking-wider text-text-primary">
          {currentTrack.title}
        </h1>
        {#if currentTrack.song}
          <div class="label text-accent mt-2">{currentTrack.song.name}</div>
        {/if}
      </div>

      <!-- Progress bar -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="w-full max-w-lg px-4">
        <div
          class="w-full h-2 bg-bg-surface cursor-pointer relative"
          onclick={seekTo}
        >
          <div class="h-full bg-accent transition-[width] duration-200" style="width: {progressPct}%"></div>
        </div>
        <div class="flex justify-between mt-2">
          <span class="text-[10px] font-mono font-semibold text-text-muted/50">{formatDuration(currentTime * 1000)}</span>
          <span class="text-[10px] font-mono font-semibold text-text-muted/50">{formatDuration(duration * 1000)}</span>
        </div>
      </div>

      <!-- Big controls -->
      <div class="flex items-center gap-8">
        <button onclick={playPrev} class="text-text-secondary hover:text-text-primary transition-colors p-4">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="currentColor">
            <rect x="3" y="5" width="3" height="14"/>
            <polygon points="21,5 9,12 21,19"/>
          </svg>
        </button>
        <button onclick={toggle} class="w-20 h-20 flex items-center justify-center bg-accent hover:bg-accent-hover text-bg-primary transition-colors rounded-full">
          {#if isPlaying}
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
              <rect x="5" y="4" width="5" height="16"/>
              <rect x="14" y="4" width="5" height="16"/>
            </svg>
          {:else}
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="6,3 21,12 6,21"/>
            </svg>
          {/if}
        </button>
        <button onclick={playNext} class="text-text-secondary hover:text-text-primary transition-colors p-4">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="currentColor">
            <polygon points="3,5 15,12 3,19"/>
            <rect x="18" y="5" width="3" height="14"/>
          </svg>
        </button>
      </div>
    </div>

    <div class="text-[9px] font-mono font-semibold tracking-[0.2em] text-text-muted/15 uppercase select-none text-center">I'm out cruising in the desert, it feels so fucking good yeah</div>

    <!-- Track list -->
    <div class="border-t border-border pt-4">
      <div class="space-y-1 max-h-[30vh] overflow-y-auto">
        {#each tracks as track, i}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="flex items-center gap-3 px-4 py-3 cursor-pointer transition-colors {i === currentIndex ? 'bg-accent/10 text-accent' : 'text-text-secondary hover:bg-bg-surface'}"
            onclick={() => playTrack(i)}
          >
            <span class="text-[10px] font-mono font-semibold w-6 text-right opacity-50">{i + 1}</span>
            <span class="flex-1 label truncate">{track.title}</span>
            <span class="text-[10px] font-mono font-semibold opacity-50">{formatDuration(track.duration_ms)}</span>
          </div>
        {/each}
      </div>
    </div>
  </div>
{/if}
