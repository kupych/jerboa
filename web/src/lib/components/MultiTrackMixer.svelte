<script lang="ts">
  import { onDestroy } from "svelte";
  import { formatDuration } from "../utils/format";

  export interface MixerTrack {
    id: string;
    title: string;
    duration_ms: number;
    offset_ms: number;
    isParent: boolean;
    streamUrl: string;
    uploader?: { display_name: string; email: string };
    vote_count?: number;
    user_voted?: boolean;
    gain?: number;
    loudness_lufs?: number | null;
  }

  let {
    tracks,
    isAdmin = false,
    bandSlug,
    parentTrackId,
    hideSrc = false,
    seamlessTop = false,
    onOffsetChange,
    onRename,
    onDelete,
    onVote,
    onBounceMix,
    onRefresh,
    onPositionChange,
    onPlayingChange,
    onTrimChange,
    onGainChange,
  }: {
    tracks: MixerTrack[];
    isAdmin?: boolean;
    bandSlug: string;
    parentTrackId: string;
    hideSrc?: boolean;
    seamlessTop?: boolean;
    onOffsetChange?: (id: string, ms: number) => void;
    onRename?: (id: string, title: string) => void;
    onDelete?: (id: string) => void;
    onVote?: (overdubId: string | null) => void;
    onBounceMix?: (overdubIds: string[], toNew: boolean) => void;
    onRefresh?: () => void;
    onPositionChange?: (ms: number) => void;
    onPlayingChange?: (playing: boolean) => void;
    onTrimChange?: (id: string, startMs: number, endMs: number) => void;
    onGainChange?: (id: string, gain: number) => void;
  } = $props();

  export function seekTo(ms: number) { commitSeek(ms); }
  export function playTrack() { play(); }
  export function pauseTrack() { pause(); }
  export function stopTrack() { stop(); }

  // === Audio engine ===
  let audioCtx: AudioContext | null = null;
  let bufferCache = new Map<string, AudioBuffer>();
  const canvasMap = new Map<string, HTMLCanvasElement>();
  const laneMap = new Map<string, HTMLDivElement>();
  let activeSources: AudioBufferSourceNode[] = [];
  let activeGains = new Map<string, GainNode>();
  let autoStopTimer: ReturnType<typeof setTimeout> | null = null;

  // === Transport state ===
  let loading = $state(false);
  let loadError = $state("");
  let playing = $state(false);
  let positionMs = $state(0);
  let seekMs = $state(0);
  let rafId = 0;
  let playStartCtxTime = 0;
  let playStartMs = 0;
  let userSeeking = false;

  // === Per-track state ===
  let muted = $state(new Set<string>());
  let soloId = $state<string | null>(null);
  let gainValues = $state<Record<string, number>>({});
  let trimValues = $state<Record<string, { startMs: number; endMs: number }>>({});

  // === UI state ===
  let editingId = $state<string | null>(null);
  let editingTitle = $state("");
  let confirmDeleteId = $state<string | null>(null);
  let bouncing = $state(false);

  // === Peaks (computed from AudioBuffer, not reactive) ===
  const peaksCache = new Map<string, Float32Array>();
  let peaksVersion = $state(0); // increment to trigger canvas redraws

  // Init gains and trims for new tracks
  $effect(() => {
    let gChanged = false, tChanged = false;
    const nextG = { ...gainValues };
    const nextT = { ...trimValues };
    for (const t of tracks) {
      if (!(t.id in nextG)) { nextG[t.id] = t.gain ?? 1; gChanged = true; }
      if (!(t.id in nextT)) {
        nextT[t.id] = { startMs: 0, endMs: t.duration_ms > 0 ? t.duration_ms : 9999999 };
        tChanged = true;
      }
    }
    if (gChanged) gainValues = nextG;
    if (tChanged) trimValues = nextT;
  });

  // Redraw canvases when peaks load or trim changes
  $effect(() => {
    peaksVersion; // subscribe
    for (const t of tracks) {
      const canvas = canvasMap.get(t.id);
      const peaks = peaksCache.get(t.id);
      if (canvas && peaks) redrawCanvas(t.id);
    }
  });

  // Also redraw when trimValues change
  $effect(() => {
    const _tv = trimValues; // subscribe
    for (const t of tracks) {
      const canvas = canvasMap.get(t.id);
      const peaks = peaksCache.get(t.id);
      if (canvas && peaks) redrawCanvas(t.id);
    }
  });

  // === Waveform rendering ===
  function extractPeaks(buffer: AudioBuffer, n: number): Float32Array {
    const nch = buffer.numberOfChannels;
    const len = buffer.length;
    const blockSize = Math.max(1, Math.floor(len / n));
    const peaks = new Float32Array(n);
    const channels = Array.from({ length: nch }, (_, i) => buffer.getChannelData(i));
    for (let i = 0; i < n; i++) {
      let max = 0;
      const s = i * blockSize;
      const e = Math.min(s + blockSize, len);
      for (let j = s; j < e; j++) {
        for (const ch of channels) {
          const v = Math.abs(ch[j]);
          if (v > max) max = v;
        }
      }
      peaks[i] = max;
    }
    return peaks;
  }

  function redrawCanvas(id: string) {
    const canvas = canvasMap.get(id);
    const peaks = peaksCache.get(id);
    if (!canvas || !peaks) return;

    const t = tracks.find((t) => t.id === id);
    const dur = t ? trackDuration(t) : 0;
    const trim = trimValues[id];
    const startFrac = dur > 0 && trim ? trim.startMs / dur : 0;
    const endFrac = dur > 0 && trim ? Math.min(trim.endMs, dur) / dur : 1;

    const w = canvas.width;
    const h = canvas.height;
    const ctx2d = canvas.getContext("2d");
    if (!ctx2d) return;
    ctx2d.clearRect(0, 0, w, h);

    const style = getComputedStyle(document.documentElement);
    const waveActive = style.getPropertyValue("--color-waveform-progress").trim() || "#5b9cf6";
    const waveDim = style.getPropertyValue("--color-waveform").trim() || "#334155";

    const barW = w / peaks.length;
    ctx2d.fillStyle = waveActive;
    for (let i = 0; i < peaks.length; i++) {
      const frac = i / peaks.length;
      if (frac < startFrac || frac > endFrac) continue;
      const bh = Math.max(2, peaks[i] * h * 0.85);
      const y = (h - bh) / 2;
      ctx2d.fillRect(
        Math.floor(i * barW),
        Math.floor(y),
        Math.max(1, Math.ceil(barW) - 1),
        Math.ceil(bh)
      );
    }
  }

  function initCanvas(node: HTMLCanvasElement, id: string) {
    canvasMap.set(id, node);
    if (peaksCache.get(id)) redrawCanvas(id);
    return {
      update(newId: string) {
        canvasMap.delete(id);
        id = newId;
        canvasMap.set(id, node);
        if (peaksCache.get(id)) redrawCanvas(id);
      },
      destroy() { canvasMap.delete(id); },
    };
  }

  function initLane(node: HTMLDivElement, id: string) {
    laneMap.set(id, node);
    return {
      update(newId: string) { laneMap.delete(id); id = newId; laneMap.set(id, node); },
      destroy() { laneMap.delete(id); },
    };
  }

  // === Timeline math ===
  function timelineShift(ts: MixerTrack[]): number {
    const offsets = ts.filter((t) => !t.isParent).map((t) => t.offset_ms);
    if (offsets.length === 0) return 0;
    return Math.max(0, -Math.min(0, ...offsets));
  }

  function trackPos(t: MixerTrack, shift: number): number {
    return t.isParent ? shift : shift + t.offset_ms;
  }

  function trackDuration(t: MixerTrack): number {
    const buf = bufferCache.get(t.id);
    if (buf) return buf.duration * 1000;
    return t.duration_ms > 0 ? t.duration_ms : 0;
  }

  let totalMs = $derived(
    tracks.length === 0
      ? 0
      : (() => {
          const shift = timelineShift(tracks);
          return Math.max(...tracks.map((t) => trackPos(t, shift) + trackDuration(t)));
        })()
  );

  // Trim handle positions as % of clip width
  function trimStartPct(t: MixerTrack): number {
    const dur = trackDuration(t);
    if (dur <= 0) return 0;
    const trim = trimValues[t.id];
    return trim ? (trim.startMs / dur) * 100 : 0;
  }

  function trimEndPct(t: MixerTrack): number {
    const dur = trackDuration(t);
    if (dur <= 0) return 100;
    const trim = trimValues[t.id];
    return trim ? (Math.min(trim.endMs, dur) / dur) * 100 : 100;
  }

  // === DAW pixel timeline ===
  let pxPerSec = $state(50); // will be auto-fit once totalMs is known
  let hasAutoFit = false;
  let scrollContainer = $state<HTMLDivElement | null>(null);
  let timelineWidthPx = $derived(Math.max((totalMs / 1000) * pxPerSec, 200));

  // Auto-fit to container width on first load
  $effect(() => {
    if (totalMs > 0 && scrollContainer && !hasAutoFit) {
      hasAutoFit = true;
      const availW = scrollContainer.clientWidth - 256;
      if (availW > 0) pxPerSec = Math.max(4, availW / (totalMs / 1000));
    }
  });

  function fullClipStartPx(t: MixerTrack): number {
    const shift = timelineShift(tracks);
    return (trackPos(t, shift) / 1000) * pxPerSec;
  }

  function fullClipWidthPx(t: MixerTrack): number {
    return (trackDuration(t) / 1000) * pxPerSec;
  }

  let playheadPx = $derived((positionMs / 1000) * pxPerSec);

  // Auto-scroll playhead into view while playing
  $effect(() => {
    const px = playheadPx;
    if (!playing || !scrollContainer) return;
    const el = scrollContainer;
    const panelW = 192;
    const viewW = el.clientWidth - panelW;
    const relX = px - el.scrollLeft;
    if (relX > viewW * 0.8) el.scrollLeft = px - viewW * 0.3;
    else if (relX < 0) el.scrollLeft = Math.max(0, px - 40);
  });

  let timeMarkers = $derived(
    totalMs <= 0
      ? []
      : (() => {
          const secs = totalMs / 1000;
          const intervals = [5, 10, 15, 30, 60, 120, 300, 600];
          const step = intervals.find((s) => secs / s <= 8) ?? 600;
          const marks: number[] = [];
          for (let s = step; s < secs; s += step) marks.push(s * 1000);
          return marks;
        })()
  );

  function effectiveGain(id: string): number {
    if (soloId !== null && soloId !== id) return 0;
    if (muted.has(id)) return 0;
    return gainValues[id] ?? 1;
  }

  // === Context ===
  function ensureCtx(): AudioContext {
    if (!audioCtx || audioCtx.state === "closed") {
      audioCtx = new AudioContext();
      bufferCache.clear();
      activeGains.clear();
      peaksCache.clear();
    }
    return audioCtx;
  }

  // === Loading ===
  // Loads each track individually with up to 3 attempts (2.5s apart).
  // This handles the race where the server hasn't finished transcoding WebM→Ogg/Opus
  // yet on the first request — by the second attempt the sibling usually exists.
  // One failed track doesn't block the others.
  async function loadMissing(): Promise<boolean> {
    const ctx = ensureCtx();
    const missing = tracks.filter((t) => !bufferCache.has(t.id));
    if (missing.length === 0) return true;

    loading = true;
    loadError = "";
    let anyFailed = false;

    try {
      await Promise.all(
        missing.map(async (t) => {
          for (let attempt = 0; attempt < 3; attempt++) {
            try {
              // cache:'no-cache' so the server can serve a freshly-transcoded
              // Ogg/Opus sibling instead of a stale WebM cache hit.
              const res = await fetch(t.streamUrl, { cache: "no-cache" });
              if (!res.ok) throw new Error(`HTTP ${res.status}`);
              const ab = await res.arrayBuffer();
              const buf = await ctx.decodeAudioData(ab);
              if (buf.duration === 0) throw new Error("zero duration");
              bufferCache.set(t.id, buf);
              peaksCache.set(t.id, extractPeaks(buf, 400));
              const dur = buf.duration * 1000;
              const existing = trimValues[t.id];
              // Update if: never set, set beyond audio end, sentinel value, or still matches
              // the DB duration (auto-initialized, not user-modified) — handles wrong/stale duration_ms.
              const autoInit = t.duration_ms > 0 ? t.duration_ms : 9999999;
              if (!existing || existing.endMs > dur || existing.endMs === 9999999 || existing.endMs === autoInit) {
                trimValues = { ...trimValues, [t.id]: { startMs: existing?.startMs ?? 0, endMs: dur } };
              }
              return; // success
            } catch (e) {
              if (attempt < 2) {
                // Give the server time to finish transcoding before retrying
                await new Promise((r) => setTimeout(r, 2500));
              } else {
                console.warn(`[mixer] failed to decode track ${t.id} after 3 attempts:`, e);
                anyFailed = true;
              }
            }
          }
        })
      );
      peaksVersion++;
      if (anyFailed) loadError = "one or more tracks failed to decode — try playing again";
      return !anyFailed;
    } catch (e) {
      loadError = "failed to load audio";
      return false;
    } finally {
      loading = false;
    }
  }

  // === Playback (trim-aware) ===
  async function play() {
    if (playing) return;
    // Create and resume the AudioContext synchronously within the user gesture BEFORE
    // any awaits — Chrome's autoplay policy requires resume() to be called while the
    // gesture token is still active. Awaiting loadMissing() (which does network fetches)
    // would expire the token, leaving the context suspended and sources silent.
    const ctx = ensureCtx();
    void ctx.resume();

    if (!(await loadMissing())) return;
    await ctx.resume(); // ensure running (no-op if already resumed)

    const from = seekMs;
    const shift = timelineShift(tracks);
    const now = ctx.currentTime;
    playStartCtxTime = now;
    playStartMs = from;

    stopSources();
    activeGains.clear();

    for (const t of tracks) {
      const buf = bufferCache.get(t.id);
      if (!buf) continue;
      const tpos = trackPos(t, shift);
      const dur = trackDuration(t);
      if (dur <= 0) continue;

      const trim = trimValues[t.id];
      const startTrim = trim?.startMs ?? 0;
      const endTrim = trim ? Math.min(trim.endMs, dur) : dur;
      const clipStart = tpos + startTrim;
      const clipEnd = tpos + endTrim;

      if (from >= clipEnd) continue;

      const ctxDelay = Math.max(0, clipStart - from) / 1000;
      const bufOffset = (startTrim + Math.max(0, from - clipStart)) / 1000;
      const duration = (clipEnd - Math.max(from, clipStart)) / 1000;
      if (duration <= 0) continue;

      const gainNode = ctx.createGain();
      gainNode.gain.value = effectiveGain(t.id);
      gainNode.connect(ctx.destination);
      activeGains.set(t.id, gainNode);

      const src = ctx.createBufferSource();
      src.buffer = buf;
      src.connect(gainNode);
      src.start(now + ctxDelay, bufOffset, duration);
      activeSources.push(src);
    }

    playing = true;
    positionMs = from;
    startRaf();

    // Auto-stop at effective end (respects trims)
    const effectiveEnd = tracks.reduce((max, t) => {
      const dur = trackDuration(t);
      const trim = trimValues[t.id];
      const endTrim = trim ? Math.min(trim.endMs, dur) : dur;
      return Math.max(max, trackPos(t, timelineShift(tracks)) + endTrim);
    }, 0);
    const remaining = Math.max(0, effectiveEnd - from);
    autoStopTimer = setTimeout(() => {
      stopSources();
      playing = false;
      cancelAnimationFrame(rafId);
      seekMs = 0;
      positionMs = 0;
    }, remaining + 250);
  }

  function pause() {
    if (!playing) return;
    seekMs = positionMs;
    stopAll();
  }

  function stop() {
    seekMs = 0;
    positionMs = 0;
    stopAll();
  }

  function commitSeek(ms: number) {
    userSeeking = false;
    const to = Math.max(0, Math.min(ms, totalMs));
    seekMs = to;
    positionMs = to;
    if (playing) { stopAll(); play(); }
  }

  function stopSources() {
    for (const src of activeSources) {
      try { src.stop(); } catch { /* already stopped */ }
    }
    activeSources = [];
    if (autoStopTimer) { clearTimeout(autoStopTimer); autoStopTimer = null; }
  }

  function stopAll() {
    stopSources();
    playing = false;
    cancelAnimationFrame(rafId);
  }

  function startRaf() {
    rafId = requestAnimationFrame(function loop() {
      if (!playing || !audioCtx) return;
      if (!userSeeking) {
        const elapsed = (audioCtx.currentTime - playStartCtxTime) * 1000;
        positionMs = Math.min(playStartMs + elapsed, totalMs);
        onPositionChange?.(positionMs);
      }
      rafId = requestAnimationFrame(loop);
    });
  }

  // === Per-track controls ===
  function toggleMute(id: string) {
    const next = new Set(muted);
    next.has(id) ? next.delete(id) : next.add(id);
    muted = next;
    applyGain(id);
  }

  function toggleSolo(id: string) {
    soloId = soloId === id ? null : id;
    for (const t of tracks) applyGain(t.id);
  }

  function setGainVal(id: string, val: number) {
    gainValues = { ...gainValues, [id]: val };
    applyGain(id);
  }

  function applyGain(id: string) {
    const gn = activeGains.get(id);
    if (gn && audioCtx) gn.gain.setValueAtTime(effectiveGain(id), audioCtx.currentTime);
  }

  function startRename(t: MixerTrack) { editingId = t.id; editingTitle = t.title; }
  function commitRename() {
    if (editingId && editingTitle.trim() && onRename) onRename(editingId, editingTitle.trim());
    editingId = null;
  }
  function cancelRename() { editingId = null; }

  // === Trim drag ===
  type TrimDrag = { id: string; side: "left" | "right"; startX: number; origMs: number };
  let trimDrag: TrimDrag | null = null;

  function startTrimDrag(e: PointerEvent, id: string, side: "left" | "right") {
    e.preventDefault();
    e.stopPropagation();
    const trim = trimValues[id];
    const dur = trackDuration(tracks.find((t) => t.id === id)!);
    trimDrag = {
      id, side,
      startX: e.clientX,
      origMs: side === "left" ? (trim?.startMs ?? 0) : (trim ? Math.min(trim.endMs, dur) : dur),
    };
    window.addEventListener("pointermove", onTrimMove);
    window.addEventListener("pointerup", onTrimUp, { once: true });
  }

  function onTrimMove(e: PointerEvent) {
    if (!trimDrag || totalMs <= 0) return;
    const t = tracks.find((t) => t.id === trimDrag!.id);
    if (!t) return;
    const dur = trackDuration(t);

    const dMs = ((e.clientX - trimDrag.startX) / pxPerSec) * 1000;
    const existing = trimValues[trimDrag.id] ?? { startMs: 0, endMs: dur };
    const next = { ...existing };

    if (trimDrag.side === "left") {
      next.startMs = Math.max(0, Math.min(trimDrag.origMs + dMs, next.endMs - 100));
    } else {
      next.endMs = Math.max(next.startMs + 100, Math.min(trimDrag.origMs + dMs, dur));
    }
    trimValues = { ...trimValues, [trimDrag.id]: next };

    const peaks = peaksCache.get(trimDrag.id);
    if (peaks) redrawCanvas(trimDrag.id);
  }

  function onTrimUp() {
    if (trimDrag) {
      const trim = trimValues[trimDrag.id];
      if (trim) onTrimChange?.(trimDrag.id, trim.startMs, trim.endMs);
    }
    trimDrag = null;
    window.removeEventListener("pointermove", onTrimMove);
  }

  // === Bounce ===
  function getUnmutedOverdubIds(): string[] {
    return tracks.filter((t) => !t.isParent && !muted.has(t.id)).map((t) => t.id);
  }

  function doBounce(toNew: boolean) {
    const ids = getUnmutedOverdubIds();
    if (ids.length === 0) return;
    bouncing = true;
    onBounceMix?.(ids, toNew);
    setTimeout(() => { bouncing = false; }, 2000);
  }

  // === Gain helpers ===
  function gainToDb(g: number): string {
    if (g <= 0) return '-∞';
    const db = 20 * Math.log10(g);
    return (db >= 0 ? '+' : '') + db.toFixed(1);
  }

  function normalize() {
    const withLufs = tracks.filter((t) => t.loudness_lufs != null);
    if (withLufs.length < 2) return;

    // Target the loudest track so everything gets boosted up to meet it,
    // rather than pulled down to meet a quiet parent with lots of overhead.
    const targetLufs = Math.max(...withLufs.map((t) => t.loudness_lufs!));

    const nextG = { ...gainValues };
    for (const t of withLufs) {
      const g = Math.min(Math.pow(10, (targetLufs - t.loudness_lufs!) / 20), 8);
      nextG[t.id] = g;
    }
    gainValues = nextG;
    for (const t of withLufs) applyGain(t.id);
    for (const t of withLufs) onGainChange?.(t.id, nextG[t.id]);
  }

  let canNormalize = $derived(tracks.filter((t) => t.loudness_lufs != null).length >= 2);

  $effect(() => { onPlayingChange?.(playing); });

  // Auto-load buffers for tracks with unknown duration (WebM/MediaRecorder files often have duration_ms=0 in DB).
  // Only attempt once per set of track IDs to avoid infinite retry on decode failure.
  let autoLoadedForIds = $state("");
  $effect(() => {
    const ids = tracks.map((t) => t.id).join(",");
    if (ids === autoLoadedForIds) return;
    if (!loading && tracks.some((t) => t.duration_ms === 0 && !bufferCache.has(t.id))) {
      autoLoadedForIds = ids;
      loadMissing();
    }
  });

  $effect(() => {
    const currentIds = new Set(tracks.map((t) => t.id));
    for (const id of [...bufferCache.keys()]) {
      if (!currentIds.has(id)) { bufferCache.delete(id); peaksCache.delete(id); }
    }
  });

  onDestroy(() => {
    cancelAnimationFrame(rafId);
    stopSources();
    audioCtx?.close();
    window.removeEventListener("pointermove", onTrimMove);
  });

  let visibleTracks = $derived(hideSrc ? tracks.filter((t) => !t.isParent) : tracks);
  let srcTrack = $derived(tracks.find((t) => t.isParent));
</script>

<div class="border border-border bg-bg-surface {seamlessTop ? 'border-t-0' : ''}">
  <!-- Transport bar -->
  <div class="flex items-center gap-2 px-3 py-2 border-b border-border/50">
    <button
      onclick={playing ? pause : play}
      disabled={loading}
      class="w-7 h-7 flex items-center justify-center text-text-secondary hover:text-accent transition-colors disabled:opacity-40 shrink-0"
      title={playing ? "pause" : "play"}
    >
      {#if loading}
        <div class="w-3 h-3 border border-accent/60 border-t-accent animate-spin"></div>
      {:else if playing}
        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
          <rect x="5" y="3" width="4" height="18"/>
          <rect x="15" y="3" width="4" height="18"/>
        </svg>
      {:else}
        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
          <polygon points="6,3 20,12 6,21"/>
        </svg>
      {/if}
    </button>

    <button
      onclick={stop}
      disabled={!playing && seekMs === 0}
      class="w-7 h-7 flex items-center justify-center text-text-secondary hover:text-accent transition-colors disabled:opacity-20 shrink-0"
      title="stop"
    >
      <svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor">
        <rect x="4" y="4" width="16" height="16" rx="1"/>
      </svg>
    </button>

    <span class="label-sm font-mono text-text-muted tabular-nums shrink-0">
      {formatDuration(positionMs)} / {formatDuration(totalMs)}
    </span>

    {#if loadError}
      <span class="label-sm text-danger ml-1">{loadError}</span>
    {/if}

    {#if hideSrc && srcTrack}
      <span class="label-sm text-text-muted shrink-0 ml-2">src</span>
      <input
        type="range" min="0" max="8" step="0.05"
        value={gainValues[srcTrack.id] ?? 1}
        oninput={(e) => setGainVal(srcTrack!.id, parseFloat((e.target as HTMLInputElement).value))}
        onchange={(e) => onGainChange?.(srcTrack!.id, parseFloat((e.target as HTMLInputElement).value))}
        class="w-20 h-1 accent-accent cursor-pointer shrink-0"
      />
      <span class="w-10 label-sm font-mono text-text-muted text-right shrink-0">
        {gainToDb(gainValues[srcTrack.id] ?? 1)}
      </span>
    {/if}

    {#if canNormalize}
      <button
        onclick={normalize}
        class="hidden sm:block label-sm text-text-muted/60 hover:text-accent transition-colors shrink-0 ml-auto"
        title="Set gains so all tracks have matching loudness"
      >normalize</button>
    {/if}

    <!-- Zoom controls -->
    <div class="hidden sm:flex items-center gap-1 {canNormalize ? '' : 'ml-auto'} shrink-0">
      <button
        onclick={() => pxPerSec = Math.max(10, pxPerSec / 1.5)}
        class="w-6 h-6 flex items-center justify-center text-text-muted/50 hover:text-accent transition-colors font-mono text-base leading-none"
        title="zoom out"
      >−</button>
      <span class="label-sm font-mono text-text-muted/40 tabular-nums w-8 text-center">{Math.round(pxPerSec)}</span>
      <button
        onclick={() => pxPerSec = Math.min(800, pxPerSec * 1.5)}
        class="w-6 h-6 flex items-center justify-center text-text-muted/50 hover:text-accent transition-colors font-mono text-base leading-none"
        title="zoom in"
      >+</button>
    </div>
  </div>

  <!-- Mobile track list (< sm) -->
  <div class="sm:hidden">
    {#each visibleTracks as t (t.id)}
      {@const isMuted = muted.has(t.id)}
      {@const isSolo = soloId === t.id}
      {@const isDimmed = soloId !== null && soloId !== t.id}
      <div class="flex items-center gap-2 px-3 py-2.5 border-b border-border/20 last:border-0 transition-opacity {isDimmed ? 'opacity-35' : ''}">
        <button
          onclick={() => toggleMute(t.id)}
          title={isMuted ? "unmute" : "mute"}
          class="w-7 h-7 text-[10px] font-bold font-mono border shrink-0 transition-colors flex items-center justify-center {isMuted
            ? 'bg-bg-primary border-border text-text-muted/50'
            : 'border-accent/40 text-accent/70 hover:border-accent hover:text-accent'}"
        >M</button>
        <button
          onclick={() => toggleSolo(t.id)}
          title={isSolo ? "unsolo" : "solo"}
          class="w-7 h-7 text-[10px] font-bold font-mono border shrink-0 transition-colors flex items-center justify-center {isSolo
            ? 'border-amber-400 bg-amber-400/10 text-amber-400'
            : 'border-border text-text-muted/50 hover:border-amber-400/60 hover:text-amber-400/60'}"
        >S</button>

        <div class="flex-1 min-w-0 flex flex-col gap-0.5">
          <span class="truncate text-sm font-semibold font-display tracking-wide {isMuted ? 'text-text-muted/50' : t.isParent ? 'text-accent/70' : 'text-text-primary'}">{t.title}</span>
        </div>

        <input
          type="range" min="0" max="8" step="0.05"
          value={gainValues[t.id] ?? 1}
          oninput={(e) => setGainVal(t.id, parseFloat((e.target as HTMLInputElement).value))}
          onchange={(e) => onGainChange?.(t.id, parseFloat((e.target as HTMLInputElement).value))}
          class="w-20 h-1 accent-accent cursor-pointer shrink-0"
        />
        <span class="w-10 label-sm font-mono text-text-muted/60 text-right shrink-0">
          {gainToDb(gainValues[t.id] ?? 1)}
        </span>
      </div>
    {/each}
  </div>

  <!-- Desktop timeline (≥ sm) — horizontally scrollable DAW layout -->
  <div class="hidden sm:block overflow-x-auto" bind:this={scrollContainer}>
    <div style="min-width: {timelineWidthPx + 256}px">

      <!-- Time ruler -->
      <div class="flex h-6 border-b border-border/20 bg-bg-primary/30 select-none sticky top-0 z-20">
        <!-- Left panel stub (sticky) -->
        <div class="w-64 shrink-0 border-r border-border/20 sticky left-0 z-30 bg-bg-surface"></div>
        <!-- Ruler ticks at pixel positions -->
        <div class="relative" style="width: {timelineWidthPx}px; flex-shrink: 0;">
          {#each timeMarkers as ms}
            <div
              class="absolute top-0 h-full flex items-center gap-0.5"
              style="left: {(ms / 1000) * pxPerSec}px"
            >
              <div class="w-px h-3 bg-border/50 shrink-0"></div>
              <span class="font-mono text-text-muted/50 font-semibold whitespace-nowrap" style="font-size: 9px;">{formatDuration(ms)}</span>
            </div>
          {/each}
        </div>
      </div>

      <!-- Track rows -->
      {#each visibleTracks as t (t.id)}
        {@const isMuted = muted.has(t.id)}
        {@const isSolo = soloId === t.id}
        {@const isDimmed = soloId !== null && soloId !== t.id}
        {@const dur = trackDuration(t)}
        <div class="flex items-stretch border-b border-border/20 last:border-0 h-[80px] transition-opacity {isDimmed ? 'opacity-35' : ''}">

          <!-- Left: controls panel (sticky, 2-row layout) -->
          <div class="w-64 shrink-0 flex flex-col justify-center gap-1 px-2 py-2 border-r border-border/20 sticky left-0 z-10 bg-bg-surface">
            <!-- Row 1: M/S + gain + actions -->
            <div class="flex items-center gap-1.5">
              <button
                onclick={() => toggleMute(t.id)}
                title={isMuted ? "unmute" : "mute"}
                class="w-5 h-5 text-[9px] font-bold font-mono border shrink-0 transition-colors flex items-center justify-center {isMuted
                  ? 'bg-bg-primary border-border text-text-muted/50'
                  : 'border-accent/40 text-accent/70 hover:border-accent hover:text-accent'}"
              >M</button>
              <button
                onclick={() => toggleSolo(t.id)}
                title={isSolo ? "unsolo" : "solo"}
                class="w-5 h-5 text-[9px] font-bold font-mono border shrink-0 transition-colors flex items-center justify-center {isSolo
                  ? 'border-amber-400 bg-amber-400/10 text-amber-400'
                  : 'border-border text-text-muted/50 hover:border-amber-400/60 hover:text-amber-400/60'}"
              >S</button>

              <!-- Gain slider — wider now that name has its own row -->
              <input
                type="range" min="0" max="8" step="0.05"
                value={gainValues[t.id] ?? 1}
                oninput={(e) => setGainVal(t.id, parseFloat((e.target as HTMLInputElement).value))}
                onchange={(e) => onGainChange?.(t.id, parseFloat((e.target as HTMLInputElement).value))}
                class="flex-1 h-0.5 accent-accent cursor-pointer min-w-0"
              />
              <span class="w-9 font-mono text-text-muted/50 font-semibold tabular-nums text-right shrink-0" style="font-size: 9px;">
                {gainToDb(gainValues[t.id] ?? 1)}
              </span>

              <!-- Actions (horizontal) -->
              <div class="flex items-center gap-2 shrink-0 pl-0.5">
                {#if !t.isParent}
                  <button
                    onclick={() => onVote?.(t.user_voted ? null : t.id)}
                    title="vote"
                    class="transition-colors {t.user_voted ? 'text-accent' : 'text-text-muted/30 hover:text-accent/70'}"
                  >
                    <svg width="10" height="10" viewBox="0 0 24 24" fill={t.user_voted ? "currentColor" : "none"} stroke="currentColor" stroke-width="2.5">
                      <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3H14z"/>
                    </svg>
                  </button>
                {/if}
                <a
                  href={`/api/bands/${bandSlug}/tracks/${t.id}/stream?dl=1`}
                  title="download"
                  class="text-text-muted/30 hover:text-accent/70 transition-colors"
                >
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                    <polyline points="7 10 12 15 17 10"/>
                    <line x1="12" y1="15" x2="12" y2="3"/>
                  </svg>
                </a>
                {#if !t.isParent}
                  {#if confirmDeleteId === t.id}
                    <div class="flex items-center gap-1">
                      <button onclick={() => { onDelete?.(t.id); confirmDeleteId = null; }} class="label-sm text-danger hover:text-red-300 transition-colors px-0.5">y</button>
                      <button onclick={() => confirmDeleteId = null} class="label-sm text-text-muted hover:text-text-secondary transition-colors px-0.5">n</button>
                    </div>
                  {:else}
                    <button onclick={() => confirmDeleteId = t.id} title="delete" class="text-text-muted/30 hover:text-danger transition-colors">
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                        <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                      </svg>
                    </button>
                  {/if}
                {/if}
              </div>
            </div>

            <!-- Row 2: track name (full panel width, no truncation fighting with controls) -->
            <div class="min-w-0">
              {#if editingId === t.id}
                <input
                  type="text"
                  bind:value={editingTitle}
                  onblur={commitRename}
                  onkeydown={(e) => { if (e.key === "Enter") commitRename(); if (e.key === "Escape") cancelRename(); }}
                  autofocus
                  class="w-full bg-bg-primary border border-accent px-1 py-0 text-xs font-semibold text-text-primary focus:outline-none leading-tight"
                />
              {:else}
                <button
                  ondblclick={() => startRename(t)}
                  title="double-click to rename"
                  class="truncate w-full text-left text-xs font-semibold font-display tracking-wide leading-tight {isMuted ? 'text-text-muted/50' : t.isParent ? 'text-accent/70' : 'text-text-secondary'} hover:text-accent transition-colors"
                >{t.title}</button>
              {/if}
            </div>
          </div>

          <!-- Right: waveform lane at explicit pixel width -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            class="relative cursor-crosshair bg-bg-primary/10"
            style="width: {timelineWidthPx}px; flex-shrink: 0;"
            use:initLane={t.id}
            onclick={(e) => {
              const lane = laneMap.get(t.id);
              if (!lane || totalMs <= 0) return;
              const rect = lane.getBoundingClientRect();
              const scrollLeft = scrollContainer?.scrollLeft ?? 0;
              const xAbsolute = (e.clientX - rect.left) + scrollLeft;
              commitSeek(Math.max(0, Math.round((xAbsolute / pxPerSec) * 1000)));
            }}
          >
            {#if totalMs > 0 && dur > 0}
              <!-- Clip block -->
              <div
                class="absolute top-1 bottom-1 rounded-sm overflow-hidden border border-border/30 bg-bg-elevated/30"
                style="left: {fullClipStartPx(t)}px; width: {fullClipWidthPx(t)}px"
              >
                <canvas
                  use:initCanvas={t.id}
                  width="400"
                  height="40"
                  class="w-full h-full block"
                ></canvas>

                <!-- Left trim handle -->
                <div
                  class="absolute top-0 h-full w-3 cursor-ew-resize z-10 flex items-center justify-center group"
                  style="left: {trimStartPct(t)}%; transform: translateX(-50%)"
                  onpointerdown={(e) => startTrimDrag(e, t.id, "left")}
                >
                  <div class="w-0.5 h-4/5 bg-white/50 group-hover:bg-white/90 rounded-full transition-colors"></div>
                </div>

                <!-- Right trim handle -->
                <div
                  class="absolute top-0 h-full w-3 cursor-ew-resize z-10 flex items-center justify-center group"
                  style="left: {trimEndPct(t)}%; transform: translateX(-50%)"
                  onpointerdown={(e) => startTrimDrag(e, t.id, "right")}
                >
                  <div class="w-0.5 h-4/5 bg-white/50 group-hover:bg-white/90 rounded-full transition-colors"></div>
                </div>
              </div>
            {:else if dur > 0}
              <!-- Loading placeholder -->
              <div
                class="absolute top-1 bottom-1 border border-border/20 bg-bg-elevated/10 flex items-center px-2"
                style="left: {fullClipStartPx(t)}px; width: {fullClipWidthPx(t)}px"
              >
                <div class="w-full h-px bg-border/30"></div>
              </div>
            {/if}

            <!-- Playhead -->
            <div
              class="absolute top-0 bottom-0 w-px bg-white/50 pointer-events-none z-20"
              style="left: {playheadPx}px"
            ></div>
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Bounce controls (admin only) -->
  {#if isAdmin && tracks.filter((t) => !t.isParent).length > 0}
    <div class="flex items-center gap-4 px-3 py-2 border-t border-border/50">
      <button
        onclick={() => doBounce(true)}
        disabled={bouncing || getUnmutedOverdubIds().length === 0}
        class="label-sm text-text-muted hover:text-accent transition-colors disabled:opacity-40"
      >{bouncing ? "bouncing..." : "bounce unmuted → new"}</button>
      <button
        onclick={() => doBounce(false)}
        disabled={bouncing || getUnmutedOverdubIds().length === 0}
        class="label-sm text-text-muted hover:text-accent transition-colors disabled:opacity-40"
      >{bouncing ? "bouncing..." : "bounce unmuted → in-place"}</button>
    </div>
  {/if}
</div>
