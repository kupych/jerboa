<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { formatDuration } from "../lib/utils/format";
  import { globalPlayer } from "../lib/stores/globalPlayer";

  let { slug }: { slug: string } = $props();

  interface Track {
    id: string;
    title: string;
    duration_ms: number;
    format: string;
    sample_rate?: number;
    file_size: number;
    status: string;
    song_id?: string;
    song?: { id: string; name: string };
    created_at: string;
  }

  let tracks = $state<Track[]>([]);
  let currentIndex = $state(0);
  let audio = $state<HTMLAudioElement | null>(null);
  let isPlaying = $state(false);
  let currentTime = $state(0);
  let duration = $state(0);
  let loading = $state(true);
  let hasLoadedTrack = $state(false);

  let currentTrack = $derived(tracks[currentIndex]);
  let progressPct = $derived(duration > 0 ? (currentTime / duration) * 100 : 0);
  let marqueeText = $derived(
    currentTrack
      ? `${currentIndex + 1}. ${currentTrack.title}${currentTrack.song ? ` - ${currentTrack.song.name}` : ""}`
      : ""
  );

  // Format info for the LCD display
  let bitrateText = $derived(() => {
    if (!currentTrack) return "";
    const sizeBytes = currentTrack.file_size;
    const durationSec = currentTrack.duration_ms / 1000;
    if (durationSec <= 0) return "";
    const kbps = Math.round((sizeBytes * 8) / durationSec / 1000);
    return `${kbps}`;
  });

  let sampleRateText = $derived(() => {
    if (!currentTrack?.sample_rate) return "";
    const sr = currentTrack.sample_rate;
    return sr >= 1000 ? `${(sr / 1000).toFixed(sr % 1000 ? 1 : 0)}` : `${sr}`;
  });

  // LCD time — split into segments for that digital clock feel
  let timeMinutes = $derived(Math.floor(currentTime / 60).toString().padStart(2, "0"));
  let timeSeconds = $derived(Math.floor(currentTime % 60).toString().padStart(2, "0"));

  // Audio graph: source → preamp → eq filters → analyser → destination
  let audioCtx: AudioContext | null = null;
  let analyserNode: AnalyserNode | null = null;
  let preampNode: GainNode | null = null;
  let eqFilters: BiquadFilterNode[] = [];
  let canvas = $state<HTMLCanvasElement | null>(null);
  let analyserConnected = false;
  let spectrumRaf = 0;
  let accentColor = "#2ec4b6";

  // EQ state
  const eqBandFreqs = [60, 170, 310, 600, 1000, 3000, 6000, 12000, 14000, 16000];
  const eqLabels = ["60", "170", "310", "600", "1K", "3K", "6K", "12K", "14K", "16K"];
  let eqGains = $state([0, 0, 0, 0, 0, 0, 0, 0, 0, 0]);
  let preampDb = $state(0);
  let showEq = $state(false);
  let showPlaylist = $state(true);
  let activePreset = $state<string | null>("flat");

  const eqPresets: Record<string, number[]> = {
    flat:    [0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
    rock:    [5, 4, 2, 0, -1, 1, 3, 5, 6, 5],
    bass:    [8, 6, 4, 2, 0, 0, 0, 0, 0, 0],
    treble:  [0, 0, 0, 0, 0, 2, 4, 6, 7, 8],
    vocal:   [-2, -1, 0, 3, 5, 5, 3, 0, -1, -2],
    scoop:   [5, 3, 0, -3, -5, -5, -3, 0, 3, 5],
  };

  // Shuffle / repeat
  let shuffle = $state(false);
  let repeat = $state<"off" | "all" | "one">("off");

  // Wake lock
  let wakeLock: WakeLockSentinel | null = null;

  onMount(async () => {
    globalPlayer.stop();

    accentColor = getComputedStyle(document.documentElement)
      .getPropertyValue("--color-accent")
      .trim();

    const all = await api<Track[]>(`/api/bands/${slug}/tracks`);
    tracks = all.filter((t) => t.status === "ready");
    loading = false;

    try {
      if ("wakeLock" in navigator) {
        wakeLock = await navigator.wakeLock.request("screen");
      }
    } catch {}
  });

  onDestroy(() => {
    wakeLock?.release();
    cancelAnimationFrame(spectrumRaf);
    audioCtx?.close();
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

  function connectAnalyser() {
    if (analyserConnected || !audio) return;
    try {
      audioCtx = new AudioContext();
      const source = audioCtx.createMediaElementSource(audio);

      // Preamp gain
      preampNode = audioCtx.createGain();
      preampNode.gain.value = 1.0;
      source.connect(preampNode);

      // 10-band EQ: peaking filters chained in series
      let lastNode: AudioNode = preampNode;
      eqFilters = eqBandFreqs.map((freq, i) => {
        const f = audioCtx!.createBiquadFilter();
        f.type = "peaking";
        f.frequency.value = freq;
        f.Q.value = 1.4;
        f.gain.value = eqGains[i];
        lastNode.connect(f);
        lastNode = f;
        return f;
      });

      // Analyser (post-EQ so spectrum reflects EQ changes)
      analyserNode = audioCtx.createAnalyser();
      analyserNode.fftSize = 128;
      analyserNode.smoothingTimeConstant = 0.75;
      lastNode.connect(analyserNode);
      analyserNode.connect(audioCtx.destination);

      analyserConnected = true;
      drawSpectrum();
    } catch {}
  }

  // --- EQ controls ---

  function setEqBand(index: number, dB: number) {
    eqGains[index] = dB;
    if (eqFilters[index]) eqFilters[index].gain.value = dB;
    activePreset = null;
  }

  function setPreamp(dB: number) {
    preampDb = dB;
    if (preampNode) preampNode.gain.value = Math.pow(10, dB / 20);
  }

  function applyPreset(name: string) {
    const p = eqPresets[name];
    if (!p) return;
    p.forEach((dB, i) => {
      eqGains[i] = dB;
      if (eqFilters[i]) eqFilters[i].gain.value = dB;
    });
    eqGains = [...eqGains];
    preampDb = 0;
    if (preampNode) preampNode.gain.value = 1.0;
    activePreset = name;
  }

  // --- Spectrum ---

  function hexToRgba(hex: string, alpha: number): string {
    const h = hex.startsWith("#") ? hex.slice(1) : hex;
    const r = parseInt(h.slice(0, 2), 16);
    const g = parseInt(h.slice(2, 4), 16);
    const b = parseInt(h.slice(4, 6), 16);
    return `rgba(${r}, ${g}, ${b}, ${alpha})`;
  }

  function drawSpectrum() {
    if (!analyserNode || !canvas) {
      spectrumRaf = requestAnimationFrame(drawSpectrum);
      return;
    }
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    if (canvas.width !== rect.width * dpr || canvas.height !== rect.height * dpr) {
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      ctx.scale(dpr, dpr);
    }

    const w = rect.width;
    const h = rect.height;
    ctx.clearRect(0, 0, w, h);

    const bufferLength = analyserNode.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);
    analyserNode.getByteFrequencyData(dataArray);

    const numBars = 20;
    const step = Math.floor(bufferLength / numBars);
    const gap = 2;
    const barWidth = (w - (numBars - 1) * gap) / numBars;

    for (let i = 0; i < numBars; i++) {
      let sum = 0;
      for (let j = 0; j < step; j++) sum += dataArray[i * step + j];
      const avg = sum / step;
      const barHeight = Math.max(1, (avg / 255) * h);
      const x = i * (barWidth + gap);

      // Draw segmented bars like Winamp
      const segHeight = 3;
      const segGap = 1;
      const numSegs = Math.ceil(barHeight / (segHeight + segGap));
      const totalSegs = Math.ceil(h / (segHeight + segGap));

      for (let s = 0; s < numSegs; s++) {
        const ratio = s / totalSegs;
        // Green at bottom, yellow in middle, red at top
        let r: number, g: number, b: number;
        if (ratio < 0.6) {
          // Use accent color
          const ah = accentColor.startsWith("#") ? accentColor.slice(1) : accentColor;
          r = parseInt(ah.slice(0, 2), 16);
          g = parseInt(ah.slice(2, 4), 16);
          b = parseInt(ah.slice(4, 6), 16);
        } else if (ratio < 0.85) {
          // Warm up toward yellow/amber
          r = 220; g = 180; b = 40;
        } else {
          // Peak — red
          r = 220; g = 50; b = 50;
        }
        const y = h - (s + 1) * (segHeight + segGap);
        ctx.fillStyle = `rgb(${r}, ${g}, ${b})`;
        ctx.fillRect(x, y, barWidth, segHeight);
      }
    }

    spectrumRaf = requestAnimationFrame(drawSpectrum);
  }

  // --- Transport ---

  function toggle() {
    if (!tracks.length || !audio) return;
    if (!hasLoadedTrack) {
      playTrack(currentIndex);
      return;
    }
    if (audio.paused) {
      if (audioCtx?.state === "suspended") audioCtx.resume();
      audio.play();
    } else {
      audio.pause();
    }
  }

  function stop() {
    if (!audio) return;
    audio.pause();
    audio.currentTime = 0;
    isPlaying = false;
  }

  function playTrack(index: number) {
    if (!audio) return;
    currentIndex = index;
    if (!analyserConnected) connectAnalyser();
    if (audioCtx?.state === "suspended") audioCtx.resume();
    hasLoadedTrack = true;
    audio.src = `/api/bands/${slug}/tracks/${tracks[index].id}/stream`;
    audio.load();
    audio.play();
  }

  function playNext() {
    if (repeat === "one") {
      playTrack(currentIndex);
      return;
    }
    if (shuffle) {
      let next: number;
      do { next = Math.floor(Math.random() * tracks.length); } while (next === currentIndex && tracks.length > 1);
      playTrack(next);
      return;
    }
    if (currentIndex < tracks.length - 1) {
      playTrack(currentIndex + 1);
    } else if (repeat === "all") {
      playTrack(0);
    }
  }

  function playPrev() {
    if (audio && audio.currentTime > 3) {
      audio.currentTime = 0;
      return;
    }
    if (currentIndex > 0) playTrack(currentIndex - 1);
  }

  function seekTo(e: MouseEvent) {
    if (!audio || !duration) return;
    const bar = e.currentTarget as HTMLElement;
    const rect = bar.getBoundingClientRect();
    const ratio = (e.clientX - rect.left) / rect.width;
    audio.currentTime = ratio * duration;
  }

  function cycleRepeat() {
    if (repeat === "off") repeat = "all";
    else if (repeat === "all") repeat = "one";
    else repeat = "off";
  }

  // Media Session API
  $effect(() => {
    if (!currentTrack || !("mediaSession" in navigator)) return;
    navigator.mediaSession.metadata = new MediaMetadata({
      title: currentTrack.title,
      artist: currentTrack.song?.name || slug,
      album: slug,
    });
    navigator.mediaSession.setActionHandler("play", () => toggle());
    navigator.mediaSession.setActionHandler("pause", () => audio?.pause());
    navigator.mediaSession.setActionHandler("previoustrack", playPrev);
    navigator.mediaSession.setActionHandler("nexttrack", playNext);
    navigator.mediaSession.setActionHandler("seekto", (d) => {
      if (audio && d.seekTime != null) audio.currentTime = d.seekTime;
    });
  });
</script>

{#if loading}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[#0a0a0a]">
    <div class="flex items-center gap-2 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
  </div>
{:else if tracks.length === 0}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[#0a0a0a]">
    <div class="label text-text-muted">no tracks</div>
  </div>
{:else}
  <audio use:bindAudio class="hidden"></audio>

  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-50 flex flex-col items-center justify-start md:justify-center bg-[#0a0a0a] overflow-y-auto">
    <!-- Ambient glow behind the whole player -->
    <div class="wa-glow" style="--glow-color: {accentColor};"></div>

    <div class="wa-wrapper" style="--glow-color: {accentColor};">
      <!-- ═══════════ MAIN PLAYER WINDOW ═══════════ -->
      <div class="wa-panel">
        <!-- Title bar -->
        <div class="wa-titlebar">
          <button
            onclick={() => navigate(`/band/${slug}`)}
            class="wa-titlebar-btn"
            title="Exit car mode"
          >
            <svg width="7" height="7" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="4"><line x1="4" y1="4" x2="20" y2="20"/><line x1="20" y1="4" x2="4" y2="20"/></svg>
          </button>
          <div class="wa-titlebar-text">JERBOAMP</div>
          <div class="wa-titlebar-grip"></div>
        </div>

        <!-- LCD display area -->
        <div class="wa-lcd" style="--glow-color: {accentColor};">
          <div class="flex gap-3 items-start">
            <!-- Big time display -->
            <div class="wa-time">
              <span class="wa-time-digit">{timeMinutes}</span><!--
              --><span class="wa-time-colon" class:wa-blink={isPlaying}>:</span><!--
              --><span class="wa-time-digit">{timeSeconds}</span>
            </div>

            <div class="flex-1 min-w-0 flex flex-col gap-1.5">
              <!-- Spectrum vis - compact, inline -->
              <div class="wa-spectrum-inline">
                <canvas bind:this={canvas} class="w-full h-full block"></canvas>
              </div>

              <!-- Scrolling title -->
              <div class="wa-marquee-box">
                <div
                  class="marquee-track inline-flex"
                  style="animation-play-state: {isPlaying ? 'running' : 'paused'}; --marquee-speed: {Math.max(6, marqueeText.length * 0.3)}s;"
                >
                  <span class="wa-marquee-text">{marqueeText}</span>
                  <span class="wa-marquee-text wa-marquee-ghost">{marqueeText}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Info strip: bitrate / sample rate / format -->
          <div class="wa-info-strip">
            {#if bitrateText()}
              <span class="wa-info-val">{bitrateText()}</span><span class="wa-info-unit">kbps</span>
            {/if}
            {#if sampleRateText()}
              <span class="wa-info-val">{sampleRateText()}</span><span class="wa-info-unit">kHz</span>
            {/if}
            {#if currentTrack}
              <span class="wa-info-badge">{currentTrack.format.toUpperCase()}</span>
            {/if}
          </div>
        </div>

        <!-- Seek bar -->
        <div class="wa-seek-row">
          <div class="wa-seek-bar" onclick={seekTo}>
            <div class="wa-seek-fill" style="width: {progressPct}%"></div>
            <div class="wa-seek-thumb" style="left: {progressPct}%"></div>
          </div>
        </div>

        <!-- Transport buttons -->
        <div class="wa-transport">
          <button onclick={playPrev} class="wa-tbtn" title="Previous">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
              <rect x="3" y="5" width="3" height="14"/>
              <polygon points="20,5 10,12 20,19"/>
            </svg>
          </button>
          <button onclick={toggle} class="wa-tbtn wa-tbtn-play" title="Play/Pause">
            {#if isPlaying}
              <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                <rect x="5" y="4" width="5" height="16"/>
                <rect x="14" y="4" width="5" height="16"/>
              </svg>
            {:else}
              <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                <polygon points="6,3 20,12 6,21"/>
              </svg>
            {/if}
          </button>
          <button onclick={stop} class="wa-tbtn" title="Stop">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
              <rect x="4" y="4" width="16" height="16"/>
            </svg>
          </button>
          <button onclick={playNext} class="wa-tbtn" title="Next">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="4,5 14,12 4,19"/>
              <rect x="18" y="5" width="3" height="14"/>
            </svg>
          </button>

          <div class="wa-transport-spacer"></div>

          <!-- Shuffle -->
          <button
            onclick={() => (shuffle = !shuffle)}
            class="wa-tbtn-sm"
            class:wa-active={shuffle}
            title="Shuffle"
          >
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
              <polyline points="16 3 21 3 21 8"/>
              <line x1="4" y1="20" x2="21" y2="3"/>
              <polyline points="21 16 21 21 16 21"/>
              <line x1="15" y1="15" x2="21" y2="21"/>
              <line x1="4" y1="4" x2="9" y2="9"/>
            </svg>
          </button>

          <!-- Repeat -->
          <button
            onclick={cycleRepeat}
            class="wa-tbtn-sm"
            class:wa-active={repeat !== "off"}
            title="Repeat: {repeat}"
          >
            {#if repeat === "one"}
              <span class="wa-repeat-one">1</span>
            {/if}
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
              <polyline points="17 1 21 5 17 9"/>
              <path d="M3 11V9a4 4 0 0 1 4-4h14"/>
              <polyline points="7 23 3 19 7 15"/>
              <path d="M21 13v2a4 4 0 0 1-4 4H3"/>
            </svg>
          </button>

          <div class="wa-transport-spacer"></div>

          <!-- EQ toggle -->
          <button
            onclick={() => (showEq = !showEq)}
            class="wa-tbtn-label"
            class:wa-active={showEq}
          >EQ</button>

          <!-- PL toggle -->
          <button
            onclick={() => (showPlaylist = !showPlaylist)}
            class="wa-tbtn-label"
            class:wa-active={showPlaylist}
          >PL</button>
        </div>
      </div>

      <!-- ═══════════ EQUALIZER WINDOW ═══════════ -->
      {#if showEq}
        <div class="wa-panel wa-eq-panel animate-dropdown">
          <div class="wa-titlebar">
            <div class="wa-titlebar-text">EQUALIZER</div>
            <div class="wa-titlebar-grip"></div>
            <div class="flex items-center gap-1">
              <span class="wa-preset-label">PRESETS</span>
            </div>
          </div>

          <div class="wa-eq-body">
            <!-- Preset buttons -->
            <div class="wa-eq-presets">
              {#each Object.keys(eqPresets) as name}
                <button
                  onclick={() => applyPreset(name)}
                  class="wa-preset-btn"
                  class:wa-active={activePreset === name}
                >{name.toUpperCase()}</button>
              {/each}
            </div>

            <!-- Sliders -->
            <div class="wa-eq-sliders">
              <!-- dB scale -->
              <div class="wa-eq-scale">
                <span>+12</span>
                <span>0</span>
                <span>-12</span>
              </div>

              <!-- Preamp -->
              <div class="wa-eq-band">
                <input
                  type="range" min="-12" max="12" step="1"
                  value={preampDb}
                  oninput={(e) => setPreamp(Number((e.target as HTMLInputElement).value))}
                  class="eq-slider"
                />
                <span class="wa-eq-freq wa-eq-freq-accent">PRE</span>
              </div>

              <div class="wa-eq-divider"></div>

              {#each eqBandFreqs as _, i}
                <div class="wa-eq-band">
                  <input
                    type="range" min="-12" max="12" step="1"
                    value={eqGains[i]}
                    oninput={(e) => setEqBand(i, Number((e.target as HTMLInputElement).value))}
                    class="eq-slider"
                  />
                  <span class="wa-eq-freq">{eqLabels[i]}</span>
                </div>
              {/each}
            </div>
          </div>
        </div>
      {/if}

      <!-- ═══════════ PLAYLIST WINDOW ═══════════ -->
      {#if showPlaylist}
        <div class="wa-panel wa-pl-panel animate-dropdown">
          <div class="wa-titlebar">
            <div class="wa-titlebar-text">PLAYLIST</div>
            <div class="wa-titlebar-grip"></div>
            <div class="wa-pl-counter">{currentIndex + 1}/{tracks.length}</div>
          </div>

          <div class="wa-pl-body">
            {#each tracks as track, i}
              <div
                class="wa-pl-row"
                class:wa-pl-active={i === currentIndex}
                onclick={() => playTrack(i)}
              >
                <span class="wa-pl-num">
                  {#if i === currentIndex && isPlaying}
                    <svg width="6" height="6" viewBox="0 0 24 24" fill="currentColor" class="inline"><polygon points="5,3 19,12 5,21"/></svg>
                  {:else}
                    {i + 1}.
                  {/if}
                </span>
                <span class="wa-pl-title">{track.title}{track.song ? ` - ${track.song.name}` : ""}</span>
                <span class="wa-pl-dur">{formatDuration(track.duration_ms)}</span>
              </div>
            {/each}
          </div>

          <!-- Playlist footer with total -->
          <div class="wa-pl-footer">
            <span>{tracks.length} tracks</span>
            <span>{formatDuration(tracks.reduce((a, t) => a + t.duration_ms, 0))}</span>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* ═══════════════════════════════════════════
     JERBOAMP — SYNTHWAVE / WINAMP DARK EDITION
     ═══════════════════════════════════════════ */

  /* Ambient glow — big diffuse halo behind everything */
  .wa-glow {
    position: fixed;
    inset: 0;
    pointer-events: none;
    background:
      radial-gradient(ellipse 70% 60% at 50% 45%, color-mix(in srgb, var(--glow-color) 12%, transparent) 0%, transparent 100%),
      radial-gradient(ellipse 40% 30% at 50% 50%, color-mix(in srgb, var(--glow-color) 6%, transparent) 0%, transparent 100%);
    z-index: 0;
  }

  /* Outer wrapper */
  .wa-wrapper {
    position: relative;
    z-index: 1;
    width: 100%;
    max-width: 480px;
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 12px;

    /* Neon glow on the whole assembly */
    filter: drop-shadow(0 0 30px color-mix(in srgb, var(--glow-color) 18%, transparent))
            drop-shadow(0 0 60px color-mix(in srgb, var(--glow-color) 10%, transparent));
  }

  /* ── PANEL (each window) ── */
  .wa-panel {
    background: linear-gradient(180deg, #2c2c30 0%, #1c1c20 100%);
    border: 1px solid #444;
    border-radius: 6px;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.08),
      inset 0 -1px 0 rgba(0,0,0,0.4),
      0 2px 8px rgba(0,0,0,0.5);
    overflow: hidden;
  }

  /* ── TITLE BAR ── */
  .wa-titlebar {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 26px;
    padding: 0 8px;
    background: linear-gradient(180deg, #404048 0%, #2a2a30 50%, #333338 100%);
    border-bottom: 1px solid #1a1a1e;
    user-select: none;
  }

  .wa-titlebar-btn {
    width: 16px;
    height: 14px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(180deg, #666 0%, #444 100%);
    border: 1px solid;
    border-color: #777 #333 #333 #777;
    border-radius: 3px;
    color: #aaa;
    cursor: pointer;
    flex-shrink: 0;
  }
  .wa-titlebar-btn:hover { color: #fff; background: linear-gradient(180deg, #888 0%, #555 100%); }
  .wa-titlebar-btn:active {
    border-color: #333 #777 #777 #333;
    background: linear-gradient(180deg, #444 0%, #555 100%);
  }

  .wa-titlebar-text {
    font-family: var(--font-display, var(--font-mono));
    font-size: 10px;
    font-weight: 800;
    letter-spacing: 0.25em;
    color: #fff;
    text-shadow:
      0 0 6px var(--glow-color, #2ec4b6),
      0 0 14px color-mix(in srgb, var(--glow-color) 50%, transparent);
    flex-shrink: 0;
  }

  .wa-titlebar-grip {
    flex: 1;
    height: 8px;
    background: repeating-linear-gradient(
      0deg,
      transparent 0px, transparent 1px,
      #444 1px, #444 2px,
      transparent 2px, transparent 3px
    );
    opacity: 0.35;
    border-radius: 2px;
  }

  /* ── LCD DISPLAY ── */
  .wa-lcd {
    margin: 5px 6px;
    padding: 10px 12px;
    background: #060806;
    border: 1px solid #222;
    border-radius: 4px;
    box-shadow:
      inset 0 2px 8px rgba(0,0,0,0.9),
      inset 0 0 20px rgba(0,0,0,0.4),
      0 0 16px color-mix(in srgb, var(--glow-color) 8%, transparent);
  }

  /* Big digital time */
  .wa-time {
    font-family: var(--font-mono);
    font-size: 34px;
    font-weight: 900;
    line-height: 1;
    color: var(--color-accent, #2ec4b6);
    text-shadow:
      0 0 8px var(--glow-color, #2ec4b6),
      0 0 20px color-mix(in srgb, var(--glow-color) 40%, transparent),
      0 0 40px color-mix(in srgb, var(--glow-color) 15%, transparent);
    letter-spacing: 2px;
    flex-shrink: 0;
    padding-top: 2px;
    font-variant-numeric: tabular-nums;
  }
  .wa-time-colon { margin: 0 -2px; }
  .wa-blink { animation: blink-colon 1s step-end infinite; }
  @keyframes blink-colon {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.15; }
  }

  /* Spectrum inline */
  .wa-spectrum-inline {
    height: 30px;
    background: transparent;
  }

  /* Marquee box */
  .wa-marquee-box {
    overflow: hidden;
    height: 20px;
    display: flex;
    align-items: center;
  }
  .wa-marquee-text {
    white-space: nowrap;
    font-family: var(--font-display, var(--font-mono));
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.15em;
    line-height: 1;
    color: var(--color-accent, #2ec4b6);
    text-shadow:
      0 0 6px var(--glow-color, #2ec4b6),
      0 0 12px color-mix(in srgb, var(--glow-color) 25%, transparent);
    padding-right: 60px;
    flex-shrink: 0;
  }
  .wa-marquee-ghost { opacity: 0.25; }

  @keyframes marquee-scroll {
    0% { transform: translateX(0); }
    100% { transform: translateX(-50%); }
  }
  .marquee-track {
    animation: marquee-scroll var(--marquee-speed, 12s) linear infinite;
  }

  /* Info strip: kbps, khz, format */
  .wa-info-strip {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 8px;
    padding-top: 6px;
    border-top: 1px solid #161816;
  }
  .wa-info-val {
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 800;
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 4px var(--glow-color, #2ec4b6);
  }
  .wa-info-unit {
    font-family: var(--font-mono);
    font-size: 8px;
    font-weight: 700;
    color: var(--color-accent, #2ec4b6);
    opacity: 0.45;
    margin-left: -6px;
    text-transform: uppercase;
  }
  .wa-info-badge {
    font-family: var(--font-mono);
    font-size: 8px;
    font-weight: 800;
    letter-spacing: 0.1em;
    color: var(--color-accent, #2ec4b6);
    border: 1px solid color-mix(in srgb, var(--color-accent) 25%, transparent);
    border-radius: 3px;
    padding: 1px 6px;
    text-shadow: 0 0 4px var(--glow-color, #2ec4b6);
    margin-left: auto;
  }

  /* ── SEEK BAR ── */
  .wa-seek-row {
    padding: 4px 6px 5px;
  }
  .wa-seek-bar {
    position: relative;
    height: 10px;
    background: #060806;
    border: 1px solid #222;
    border-radius: 5px;
    box-shadow: inset 0 1px 4px rgba(0,0,0,0.8);
    cursor: pointer;
    overflow: hidden;
  }
  .wa-seek-fill {
    height: 100%;
    background: linear-gradient(90deg,
      color-mix(in srgb, var(--color-accent) 30%, transparent),
      var(--color-accent)
    );
    box-shadow: 0 0 8px color-mix(in srgb, var(--color-accent) 50%, transparent);
    border-radius: 5px 0 0 5px;
    transition: none;
  }
  .wa-seek-thumb {
    position: absolute;
    top: -1px;
    width: 14px;
    height: 12px;
    margin-left: -7px;
    background: linear-gradient(180deg, #bbb 0%, #777 50%, #555 100%);
    border: 1px solid;
    border-color: #ccc #444 #444 #ccc;
    border-radius: 3px;
    pointer-events: none;
    box-shadow: 0 0 6px color-mix(in srgb, var(--color-accent) 20%, transparent);
  }

  /* ── TRANSPORT BUTTONS ── */
  .wa-transport {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px 6px 7px;
  }

  .wa-tbtn {
    width: 42px;
    height: 34px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(180deg, #505058 0%, #333338 50%, #2a2a30 100%);
    border: 1px solid;
    border-color: #606068 #222228 #222228 #606068;
    border-radius: 5px;
    color: #ccc;
    cursor: pointer;
    flex-shrink: 0;
    box-shadow: 0 1px 3px rgba(0,0,0,0.4);
  }
  .wa-tbtn:hover {
    color: #fff;
    background: linear-gradient(180deg, #5a5a62 0%, #3a3a40 50%, #333338 100%);
  }
  .wa-tbtn:active {
    border-color: #222228 #606068 #606068 #222228;
    background: linear-gradient(180deg, #2a2a30 0%, #333338 100%);
    box-shadow: inset 0 1px 3px rgba(0,0,0,0.5);
  }

  .wa-tbtn-play {
    width: 50px;
    color: var(--color-accent, #2ec4b6);
    box-shadow:
      0 1px 3px rgba(0,0,0,0.4),
      0 0 14px color-mix(in srgb, var(--color-accent) 12%, transparent);
  }
  .wa-tbtn-play:hover {
    color: var(--color-accent-hover, #4ad4c8);
    box-shadow:
      0 1px 3px rgba(0,0,0,0.4),
      0 0 20px color-mix(in srgb, var(--color-accent) 22%, transparent);
  }

  .wa-tbtn-sm {
    width: 30px;
    height: 26px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(180deg, #3e3e44 0%, #28282e 100%);
    border: 1px solid;
    border-color: #4a4a50 #1e1e24 #1e1e24 #4a4a50;
    border-radius: 4px;
    color: #555;
    cursor: pointer;
    flex-shrink: 0;
    position: relative;
  }
  .wa-tbtn-sm:hover { color: #999; }
  .wa-tbtn-sm.wa-active {
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 6px var(--glow-color, #2ec4b6);
    box-shadow: 0 0 8px color-mix(in srgb, var(--color-accent) 15%, transparent);
  }

  .wa-repeat-one {
    position: absolute;
    top: 0px;
    right: 2px;
    font-size: 7px;
    font-weight: 900;
    font-family: var(--font-mono);
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 4px var(--glow-color, #2ec4b6);
  }

  .wa-tbtn-label {
    height: 26px;
    padding: 0 10px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(180deg, #3e3e44 0%, #28282e 100%);
    border: 1px solid;
    border-color: #4a4a50 #1e1e24 #1e1e24 #4a4a50;
    border-radius: 4px;
    font-family: var(--font-mono);
    font-size: 9px;
    font-weight: 800;
    letter-spacing: 0.12em;
    color: #555;
    cursor: pointer;
    flex-shrink: 0;
  }
  .wa-tbtn-label:hover { color: #999; }
  .wa-tbtn-label.wa-active {
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 6px var(--glow-color, #2ec4b6);
    box-shadow: 0 0 8px color-mix(in srgb, var(--color-accent) 12%, transparent);
    border-color: color-mix(in srgb, var(--color-accent) 25%, #4a4a50) #1e1e24 #1e1e24 color-mix(in srgb, var(--color-accent) 25%, #4a4a50);
  }

  .wa-transport-spacer {
    flex: 1;
  }

  /* ── EQUALIZER ── */
  .wa-eq-body {
    padding: 6px 8px;
  }

  .wa-eq-presets {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
    margin-bottom: 8px;
  }

  .wa-preset-btn {
    padding: 3px 7px;
    font-family: var(--font-mono);
    font-size: 7px;
    font-weight: 700;
    letter-spacing: 0.06em;
    color: #555;
    background: linear-gradient(180deg, #333338 0%, #222228 100%);
    border: 1px solid #3a3a40;
    border-radius: 3px;
    cursor: pointer;
  }
  .wa-preset-btn:hover { color: #999; }
  .wa-preset-btn.wa-active {
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 4px var(--glow-color, #2ec4b6);
    border-color: color-mix(in srgb, var(--color-accent) 35%, #3a3a40);
    box-shadow: 0 0 6px color-mix(in srgb, var(--color-accent) 10%, transparent);
  }

  .wa-preset-label {
    font-family: var(--font-mono);
    font-size: 8px;
    font-weight: 700;
    color: #555;
    letter-spacing: 0.1em;
  }

  .wa-eq-sliders {
    display: flex;
    align-items: flex-start;
    gap: 0;
    position: relative;
  }

  .wa-eq-scale {
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    height: 80px;
    margin-right: 4px;
    flex-shrink: 0;
  }
  .wa-eq-scale span {
    font-family: var(--font-mono);
    font-size: 7px;
    font-weight: 600;
    color: rgba(136,136,136,0.25);
    line-height: 1;
  }

  .wa-eq-band {
    display: flex;
    flex-direction: column;
    align-items: center;
    flex: 1;
    min-width: 0;
  }

  .wa-eq-divider {
    width: 1px;
    height: 70px;
    background: #2a2a2e;
    margin: 5px 2px 0;
    flex-shrink: 0;
    align-self: flex-start;
    border-radius: 1px;
  }

  .wa-eq-freq {
    font-family: var(--font-mono);
    font-size: 7px;
    font-weight: 600;
    color: rgba(136,136,136,0.3);
    text-transform: uppercase;
    margin-top: 4px;
    text-align: center;
    letter-spacing: 0.05em;
  }
  .wa-eq-freq-accent {
    color: color-mix(in srgb, var(--color-accent) 50%, transparent);
  }

  /* ── PLAYLIST ── */
  .wa-pl-body {
    max-height: 180px;
    overflow-y: auto;
    background: #060806;
    margin: 0 2px;
    border: 1px solid #1a1a1e;
    border-radius: 3px;
    box-shadow: inset 0 2px 6px rgba(0,0,0,0.7);
  }

  .wa-pl-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 10px;
    cursor: pointer;
    border-bottom: 1px solid #0c0e0c;
    transition: background 80ms;
  }
  .wa-pl-row:last-child { border-bottom: none; }
  .wa-pl-row:hover { background: rgba(255,255,255,0.03); }
  .wa-pl-row.wa-pl-active {
    background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  }

  .wa-pl-num {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 700;
    width: 20px;
    text-align: right;
    flex-shrink: 0;
    color: #555;
  }
  .wa-pl-active .wa-pl-num {
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 6px var(--glow-color, #2ec4b6);
  }

  .wa-pl-title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-display, var(--font-mono));
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    color: #777;
  }
  .wa-pl-active .wa-pl-title {
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 6px var(--glow-color, #2ec4b6);
  }

  .wa-pl-dur {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 600;
    color: #4a4a4a;
    flex-shrink: 0;
  }
  .wa-pl-active .wa-pl-dur {
    color: var(--color-accent, #2ec4b6);
    text-shadow: 0 0 4px var(--glow-color, #2ec4b6);
  }

  .wa-pl-counter {
    font-family: var(--font-mono);
    font-size: 9px;
    font-weight: 700;
    color: #555;
    letter-spacing: 0.05em;
  }

  .wa-pl-footer {
    display: flex;
    justify-content: space-between;
    padding: 5px 10px;
    font-family: var(--font-mono);
    font-size: 8px;
    font-weight: 700;
    color: #444;
    border-top: 1px solid #222228;
  }

  /* ── EQ SLIDER (vertical, Winamp-style) ── */
  .eq-slider {
    writing-mode: vertical-lr;
    direction: rtl;
    -webkit-appearance: none;
    appearance: none;
    width: 100%;
    max-width: 28px;
    height: 80px;
    background: transparent;
    margin: 0;
    padding: 0;
    cursor: pointer;
  }

  .eq-slider::-webkit-slider-runnable-track {
    width: 4px;
    background: linear-gradient(to right, #161618, #1e1e22);
    border: 1px solid #2a2a2e;
    border-radius: 2px;
  }
  .eq-slider::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 20px;
    height: 8px;
    margin-left: -8px;
    background: linear-gradient(180deg, #999 0%, #666 50%, #444 100%);
    border: 1px solid;
    border-color: #aaa #333 #333 #aaa;
    border-radius: 2px;
    cursor: pointer;
    box-shadow: 0 0 4px rgba(0,0,0,0.4);
  }
  .eq-slider::-webkit-slider-thumb:hover {
    background: linear-gradient(180deg, #bbb 0%, #888 50%, #666 100%);
  }

  .eq-slider::-moz-range-track {
    width: 4px;
    background: linear-gradient(to right, #161618, #1e1e22);
    border: 1px solid #2a2a2e;
    border-radius: 2px;
  }
  .eq-slider::-moz-range-thumb {
    width: 20px;
    height: 8px;
    background: linear-gradient(180deg, #999 0%, #666 50%, #444 100%);
    border: 1px solid;
    border-color: #aaa #333 #333 #aaa;
    border-radius: 2px;
    cursor: pointer;
    box-shadow: 0 0 4px rgba(0,0,0,0.4);
  }
  .eq-slider::-moz-range-thumb:hover {
    background: linear-gradient(180deg, #bbb 0%, #888 50%, #666 100%);
  }

  /* ── Mobile: fill width, fat touch targets ── */
  @media (max-width: 520px) {
    .wa-wrapper {
      padding: 4px;
      max-width: 100%;
    }
    .wa-tbtn {
      width: 50px;
      height: 44px;
    }
    .wa-tbtn-play {
      width: 60px;
    }
    .wa-tbtn-sm {
      width: 36px;
      height: 32px;
    }
    .wa-tbtn-label {
      height: 32px;
      padding: 0 12px;
      font-size: 10px;
    }
    .wa-time {
      font-size: 30px;
    }
    .wa-pl-body {
      max-height: 260px;
    }
    .wa-pl-row {
      padding: 9px 10px;
    }
    .wa-pl-title {
      font-size: 12px;
    }
  }
</style>
