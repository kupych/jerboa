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
  } = $props();

  export function seekTo(ms: number) { commitSeek(ms); }
  export function playTrack() { play(); }
  export function pauseTrack() { pause(); }
  export function stopTrack() { stop(); }

  // === Audio engine ===
  let audioCtx: AudioContext | null = null;
  let bufferCache = new Map<string, AudioBuffer>();
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
  let userSeeking = false; // plain flag — NOT reactive, so no re-render during drag

  // === Per-track state ===
  let muted = $state(new Set<string>());
  let soloId = $state<string | null>(null);
  let gainValues = $state<Record<string, number>>({});

  // === UI state ===
  let editingId = $state<string | null>(null);
  let editingTitle = $state("");
  let confirmDeleteId = $state<string | null>(null);
  let bouncing = $state(false);

  // Init gains for new tracks
  $effect(() => {
    let changed = false;
    const next = { ...gainValues };
    for (const t of tracks) {
      if (!(t.id in next)) { next[t.id] = 1; changed = true; }
    }
    if (changed) gainValues = next;
  });

  // === Timeline math ===
  function timelineShift(ts: MixerTrack[]): number {
    const offsets = ts.filter((t) => !t.isParent).map((t) => t.offset_ms);
    if (offsets.length === 0) return 0;
    return Math.max(0, -Math.min(0, ...offsets));
  }

  function trackPos(t: MixerTrack, shift: number): number {
    return t.isParent ? shift : shift + t.offset_ms;
  }

  // Use buffer duration if track metadata not yet processed or suspiciously short
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
    }
    return audioCtx;
  }

  // === Loading ===
  async function loadMissing(): Promise<boolean> {
    const ctx = ensureCtx();
    const missing = tracks.filter((t) => !bufferCache.has(t.id));
    if (missing.length === 0) return true;

    loading = true;
    loadError = "";
    try {
      await Promise.all(
        missing.map(async (t) => {
          const ab = await fetch(t.streamUrl).then((r) => r.arrayBuffer());
          const buf = await ctx.decodeAudioData(ab);
          bufferCache.set(t.id, buf);
        })
      );
      return true;
    } catch {
      loadError = "failed to load one or more tracks";
      return false;
    } finally {
      loading = false;
    }
  }

  // === Playback ===
  async function play() {
    if (playing) return;
    if (!(await loadMissing())) return;

    const ctx = ensureCtx();
    await ctx.resume();

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
      if (dur <= 0 || from >= tpos + dur) continue;

      const ctxDelay = Math.max(0, tpos - from) / 1000;
      const bufOffset = Math.max(0, from - tpos) / 1000;

      const gainNode = ctx.createGain();
      gainNode.gain.value = effectiveGain(t.id);
      gainNode.connect(ctx.destination);
      activeGains.set(t.id, gainNode);

      const src = ctx.createBufferSource();
      src.buffer = buf;
      src.connect(gainNode);
      src.start(now + ctxDelay, bufOffset);
      activeSources.push(src);
    }

    playing = true;
    positionMs = from;
    startRaf();

    const remaining = Math.max(0, totalMs - from);
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

  // Seek committed by slider release or click
  function commitSeek(ms: number) {
    userSeeking = false;
    const to = Math.max(0, Math.min(ms, totalMs));
    seekMs = to;
    positionMs = to;
    if (playing) {
      stopAll();
      play();
    }
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

  // === Inline rename ===
  function startRename(t: MixerTrack) {
    editingId = t.id;
    editingTitle = t.title;
  }

  function commitRename() {
    if (editingId && editingTitle.trim() && onRename) {
      onRename(editingId, editingTitle.trim());
    }
    editingId = null;
  }

  function cancelRename() {
    editingId = null;
  }

  // === Bounce ===
  function getUnmutedOverdubIds(): string[] {
    return tracks
      .filter((t) => !t.isParent && !muted.has(t.id))
      .map((t) => t.id);
  }

  function doBounce(toNew: boolean) {
    const ids = getUnmutedOverdubIds();
    if (ids.length === 0) return;
    bouncing = true;
    onBounceMix?.(ids, toNew);
    // Reset after a short delay (the bounce runs async server-side)
    setTimeout(() => { bouncing = false; }, 2000);
  }

  $effect(() => { onPlayingChange?.(playing); });

  // Invalidate buffers when tracks change (new overdub, etc.)
  $effect(() => {
    const currentIds = new Set(tracks.map((t) => t.id));
    for (const id of bufferCache.keys()) {
      if (!currentIds.has(id)) bufferCache.delete(id);
    }
  });

  onDestroy(() => {
    cancelAnimationFrame(rafId);
    stopSources();
    audioCtx?.close();
  });
</script>

<div class="border border-border bg-bg-surface {seamlessTop ? 'border-t-0' : ''}">
  <!-- Source volume + transport row (when integrated with waveform) -->
  {#if hideSrc}
    {@const srcTrack = tracks.find((t) => t.isParent)}
    {#if srcTrack}
      <div class="flex items-center gap-3 px-4 py-2.5 border-b border-border/50">
        <!-- Play/Pause -->
        <button
          onclick={playing ? pause : play}
          disabled={loading}
          title={playing ? "pause" : "play"}
          class="w-7 h-7 flex items-center justify-center text-text-secondary hover:text-accent transition-colors disabled:opacity-40 shrink-0"
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
        <!-- Stop -->
        <button
          onclick={stop}
          disabled={!playing && seekMs === 0}
          title="stop"
          class="w-7 h-7 flex items-center justify-center text-text-secondary hover:text-accent transition-colors disabled:opacity-20 shrink-0"
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
            <rect x="4" y="4" width="16" height="16" rx="1"/>
          </svg>
        </button>
        <span class="label-sm text-text-muted shrink-0">src</span>
        {#if loadError}
          <span class="label-sm text-danger">{loadError}</span>
        {:else}
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={gainValues[srcTrack.id] ?? 1}
            oninput={(e) => setGainVal(srcTrack.id, parseFloat((e.target as HTMLInputElement).value))}
            class="flex-1 h-1 accent-accent cursor-pointer"
          />
          <span class="w-7 label-sm font-mono text-text-muted text-right shrink-0">
            {Math.round((gainValues[srcTrack.id] ?? 1) * 100)}
          </span>
        {/if}
      </div>
    {/if}
  {:else}
    <!-- Standalone transport bar (when not integrated with waveform) -->
    <div class="flex items-center gap-3 px-4 py-2.5 border-b border-border/50">
      <button
        onclick={playing ? pause : play}
        disabled={loading}
        title={playing ? "pause" : "play"}
        class="w-7 h-7 flex items-center justify-center text-text-secondary hover:text-accent transition-colors disabled:opacity-40 shrink-0"
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
        title="stop"
        class="w-7 h-7 flex items-center justify-center text-text-secondary hover:text-accent transition-colors disabled:opacity-20 shrink-0"
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
          <rect x="4" y="4" width="16" height="16" rx="1"/>
        </svg>
      </button>
      {#if loadError}
        <span class="label-sm text-danger">{loadError}</span>
      {:else}
        <span class="label-sm font-mono text-text-muted tabular-nums shrink-0">
          {formatDuration(positionMs)} / {formatDuration(totalMs)}
        </span>
        <input
          type="range"
          min="0"
          max={totalMs || 1}
          value={positionMs}
          step="100"
          oninput={(e) => { userSeeking = true; positionMs = parseFloat((e.target as HTMLInputElement).value); }}
          onchange={(e) => commitSeek(parseFloat((e.target as HTMLInputElement).value))}
          class="flex-1 h-1 accent-accent cursor-pointer"
        />
      {/if}
    </div>
  {/if}

  <!-- Track rows -->
  {#each tracks.filter((t) => !(hideSrc && t.isParent)) as t (t.id)}
    {@const isMuted = muted.has(t.id)}
    {@const isSolo = soloId === t.id}
    {@const isDimmed = soloId !== null && soloId !== t.id}
    <div
      class="flex flex-col px-4 py-2 border-b border-border/30 last:border-0 transition-opacity {isDimmed ? 'opacity-40' : ''}"
    >
      <!-- Top row: M S + track name -->
      <div class="flex items-center gap-2 min-w-0">
        <!-- Mute -->
        <button
          onclick={() => toggleMute(t.id)}
          title={isMuted ? "unmute" : "mute"}
          class="w-6 h-6 label-sm font-mono border shrink-0 transition-colors {isMuted
            ? 'bg-bg-primary border-border text-text-muted line-through'
            : 'border-accent/60 text-accent hover:border-accent'}"
        >M</button>

        <!-- Solo -->
        <button
          onclick={() => toggleSolo(t.id)}
          title={isSolo ? "unsolo" : "solo"}
          class="w-6 h-6 label-sm font-mono border shrink-0 transition-colors {isSolo
            ? 'border-amber-400 bg-amber-400/10 text-amber-400'
            : 'border-border text-text-muted hover:border-amber-400/60 hover:text-amber-400/60'}"
        >S</button>

        {#if t.isParent}
          <span class="label-sm bg-accent/10 text-accent px-1.5 shrink-0">src</span>
        {/if}

        {#if editingId === t.id}
          <input
            type="text"
            bind:value={editingTitle}
            onblur={commitRename}
            onkeydown={(e) => { if (e.key === "Enter") commitRename(); if (e.key === "Escape") cancelRename(); }}
            autofocus
            class="flex-1 min-w-0 bg-bg-primary border border-accent px-1.5 py-0.5 text-sm font-semibold text-text-primary font-display tracking-wide focus:outline-none"
          />
        {:else}
          <button
            onclick={() => startRename(t)}
            title="click to rename"
            class="flex-1 min-w-0 text-sm font-semibold font-display tracking-wide truncate text-left {isMuted ? 'text-text-muted' : 'text-text-primary'} hover:text-accent transition-colors"
          >{t.title}</button>
        {/if}
      </div>

      <!-- Bottom row: controls (indented past M+S) -->
      <div class="flex items-center gap-2 mt-1.5 pl-16">
        {#if !t.isParent}
          <input
            type="number"
            step="10"
            value={t.offset_ms}
            onchange={(e) => {
              const val = parseInt((e.target as HTMLInputElement).value);
              if (!isNaN(val)) onOffsetChange?.(t.id, val);
            }}
            class="w-16 bg-bg-primary border border-border px-1 py-0.5 label-sm font-mono text-text-secondary text-right focus:outline-none focus:border-accent transition-colors shrink-0"
          />
          <span class="label-sm text-text-muted shrink-0">ms</span>
        {/if}

        <!-- Gain slider -->
        <input
          type="range"
          min="0"
          max="1"
          step="0.05"
          value={gainValues[t.id] ?? 1}
          oninput={(e) => setGainVal(t.id, parseFloat((e.target as HTMLInputElement).value))}
          class="flex-1 h-1 accent-accent cursor-pointer"
        />
        <span class="w-7 label-sm font-mono text-text-muted text-right shrink-0">
          {Math.round((gainValues[t.id] ?? 1) * 100)}
        </span>

        <!-- Row actions -->
        <div class="flex items-center gap-2 shrink-0">
          {#if !t.isParent}
            <!-- Vote -->
            <button
              onclick={() => onVote?.(t.user_voted ? null : t.id)}
              title="vote"
              class="label-sm transition-colors flex items-center gap-0.5 {t.user_voted ? 'text-accent' : 'text-text-muted hover:text-accent'}"
            >
              <svg width="10" height="10" viewBox="0 0 24 24" fill={t.user_voted ? "currentColor" : "none"} stroke="currentColor" stroke-width="2">
                <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3H14z"/>
                <path d="M7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3"/>
              </svg>
              {t.vote_count || ""}
            </button>
          {/if}

          <!-- Download -->
          <a
            href={`/api/bands/${bandSlug}/tracks/${t.id}/stream?dl=1`}
            title="download"
            class="label-sm text-text-muted hover:text-accent transition-colors"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
              <polyline points="7 10 12 15 17 10"/>
              <line x1="12" y1="15" x2="12" y2="3"/>
            </svg>
          </a>

          {#if !t.isParent}
            <!-- Delete -->
            {#if confirmDeleteId === t.id}
              <button
                onclick={() => { onDelete?.(t.id); confirmDeleteId = null; }}
                class="label-sm text-danger hover:text-red-300 transition-colors"
              >yes</button>
              <button
                onclick={() => confirmDeleteId = null}
                class="label-sm text-text-muted hover:text-text-secondary transition-colors"
              >no</button>
            {:else}
              <button
                onclick={() => confirmDeleteId = t.id}
                title="delete"
                class="text-text-muted hover:text-danger transition-colors"
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
              </button>
            {/if}
          {/if}
        </div>
      </div>
    </div>
  {/each}

  <!-- Bounce controls (admin only) -->
  {#if isAdmin && tracks.filter((t) => !t.isParent).length > 0}
    <div class="flex items-center gap-4 px-4 py-2.5 border-t border-border/50">
      <button
        onclick={() => doBounce(true)}
        disabled={bouncing || getUnmutedOverdubIds().length === 0}
        class="label-sm text-text-muted hover:text-accent transition-colors disabled:opacity-40"
      >{bouncing ? "bouncing..." : "bounce unmuted → new track"}</button>
      <button
        onclick={() => doBounce(false)}
        disabled={bouncing || getUnmutedOverdubIds().length === 0}
        class="label-sm text-text-muted hover:text-accent transition-colors disabled:opacity-40"
      >{bouncing ? "bouncing..." : "bounce unmuted → in-place"}</button>
    </div>
  {/if}
</div>
