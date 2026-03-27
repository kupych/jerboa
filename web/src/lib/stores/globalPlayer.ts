// Persistent audio player store.
// Survives navigation — the <audio> element lives in Layout.

import { writable, derived, get } from "svelte/store";

export interface TrackInfo {
  id: string;
  title: string;
  bandSlug: string;
  bandName?: string;
  songName?: string;
  durationMs?: number;
}

export type LoopMode = "off" | "all" | "one";

interface PlayerState {
  track: TrackInfo | null;
  playing: boolean;
  currentTime: number;
  duration: number;
  loading: boolean;
  queue: TrackInfo[];
  queueIndex: number;
  loop: LoopMode;
  shuffle: boolean;
  // Shuffle order: indices into queue[]
  shuffleOrder: number[];
}

const state = writable<PlayerState>({
  track: null,
  playing: false,
  currentTime: 0,
  duration: 0,
  loading: false,
  queue: [],
  queueIndex: -1,
  loop: "off",
  shuffle: false,
  shuffleOrder: [],
});

let audioEl: HTMLAudioElement | null = null;
let rafId: number | null = null;

function startRAF() {
  if (rafId != null) return;
  function tick() {
    if (audioEl && !audioEl.paused) {
      state.update((s) => ({ ...s, currentTime: audioEl!.currentTime }));
    }
    rafId = requestAnimationFrame(tick);
  }
  rafId = requestAnimationFrame(tick);
}

function stopRAF() {
  if (rafId != null) {
    cancelAnimationFrame(rafId);
    rafId = null;
  }
}

/** Fisher-Yates shuffle, returns new array of indices */
function generateShuffleOrder(length: number, startIndex?: number): number[] {
  const order = Array.from({ length }, (_, i) => i);
  for (let i = order.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [order[i], order[j]] = [order[j], order[i]];
  }
  // If we have a startIndex, move it to front so current track plays first
  if (startIndex != null && startIndex >= 0) {
    const pos = order.indexOf(startIndex);
    if (pos > 0) {
      [order[0], order[pos]] = [order[pos], order[0]];
    }
  }
  return order;
}

/** Get the effective queue index accounting for shuffle */
function effectiveIndex(s: PlayerState): number {
  if (s.shuffle && s.shuffleOrder.length > 0) {
    const shufflePos = s.shuffleOrder.indexOf(s.queueIndex);
    return shufflePos >= 0 ? shufflePos : 0;
  }
  return s.queueIndex;
}

/** Resolve a shuffle position to a real queue index */
function resolveIndex(s: PlayerState, pos: number): number {
  if (s.shuffle && s.shuffleOrder.length > 0) {
    return s.shuffleOrder[pos] ?? 0;
  }
  return pos;
}

function loadTrackAtIndex(idx: number) {
  const s = get(state);
  const track = s.queue[idx];
  if (!track || !audioEl) return;

  state.update((st) => ({
    ...st,
    track,
    queueIndex: idx,
    currentTime: 0,
    duration: 0,
    loading: true,
    playing: true,
  }));
  audioEl.src = `/api/bands/${track.bandSlug}/tracks/${track.id}/stream`;
  audioEl.play();
}

export const playerState = { subscribe: state.subscribe };
export const currentTrack = derived(state, (s) => s.track);
export const isPlaying = derived(state, (s) => s.playing);

export const globalPlayer = {
  /** Called once from Layout to bind the persistent <audio> element */
  setAudioElement(el: HTMLAudioElement) {
    audioEl = el;

    el.addEventListener("loadedmetadata", () => {
      state.update((s) => ({
        ...s,
        duration: el.duration,
        loading: false,
      }));
    });

    el.addEventListener("ended", () => {
      stopRAF();
      const s = get(state);

      // Loop one: restart same track
      if (s.loop === "one") {
        el.currentTime = 0;
        el.play();
        return;
      }

      // Try advance
      const eIdx = effectiveIndex(s);
      const nextPos = eIdx + 1;
      const qLen = s.queue.length;

      if (nextPos < qLen) {
        loadTrackAtIndex(resolveIndex(s, nextPos));
      } else if (s.loop === "all" && qLen > 0) {
        // Wrap around
        if (s.shuffle) {
          // Re-shuffle for next pass
          const newOrder = generateShuffleOrder(qLen);
          state.update((st) => ({ ...st, shuffleOrder: newOrder }));
          loadTrackAtIndex(newOrder[0]);
        } else {
          loadTrackAtIndex(0);
        }
      } else {
        state.update((st) => ({ ...st, playing: false }));
      }
    });

    el.addEventListener("play", () => {
      startRAF();
      state.update((s) => ({ ...s, playing: true }));
    });

    el.addEventListener("pause", () => {
      stopRAF();
      state.update((s) => ({ ...s, playing: false, currentTime: el.currentTime }));
    });

    el.addEventListener("waiting", () => {
      state.update((s) => ({ ...s, loading: true }));
    });

    el.addEventListener("canplay", () => {
      state.update((s) => ({ ...s, loading: false }));
    });
  },

  /** Play a track. If same track, toggle. Replaces queue with just this track. */
  play(track: TrackInfo) {
    if (!audioEl) return;

    const current = get(state).track;
    if (current?.id === track.id) {
      if (audioEl.paused) audioEl.play();
      else audioEl.pause();
      return;
    }

    const s = get(state);
    // Check if track is already in queue
    const existingIdx = s.queue.findIndex((t) => t.id === track.id);
    if (existingIdx >= 0) {
      loadTrackAtIndex(existingIdx);
      return;
    }

    // New track, replace queue
    const shuffleOrder = s.shuffle ? generateShuffleOrder(1, 0) : [];
    state.update((st) => ({
      ...st,
      queue: [track],
      queueIndex: 0,
      shuffleOrder,
    }));
    loadTrackAtIndex(0);
  },

  /** Play a list of tracks starting from a given index */
  playAll(tracks: TrackInfo[], startIndex = 0) {
    if (!audioEl || tracks.length === 0) return;

    const s = get(state);
    const shuffleOrder = s.shuffle
      ? generateShuffleOrder(tracks.length, startIndex)
      : [];

    state.update((st) => ({
      ...st,
      queue: tracks,
      queueIndex: startIndex,
      shuffleOrder,
    }));
    loadTrackAtIndex(startIndex);
  },

  /** Add track(s) to the end of the queue */
  enqueue(...tracks: TrackInfo[]) {
    state.update((s) => {
      const newQueue = [...s.queue, ...tracks];
      const shuffleOrder = s.shuffle
        ? generateShuffleOrder(newQueue.length, s.queueIndex)
        : s.shuffleOrder;
      return { ...s, queue: newQueue, shuffleOrder };
    });
    // If nothing is playing, start
    const s = get(state);
    if (!s.track && s.queue.length > 0) {
      loadTrackAtIndex(0);
    }
  },

  /** Remove a track from the queue by index */
  removeFromQueue(idx: number) {
    state.update((s) => {
      const newQueue = s.queue.filter((_, i) => i !== idx);
      let newIndex = s.queueIndex;
      if (idx < s.queueIndex) newIndex--;
      else if (idx === s.queueIndex && newIndex >= newQueue.length) {
        newIndex = Math.max(0, newQueue.length - 1);
      }
      const shuffleOrder = s.shuffle
        ? generateShuffleOrder(newQueue.length, newIndex)
        : [];
      return {
        ...s,
        queue: newQueue,
        queueIndex: newIndex,
        shuffleOrder,
        track: newQueue[newIndex] ?? null,
      };
    });
  },

  /** Clear the queue (keeps current track playing) */
  clearQueue() {
    state.update((s) => {
      if (s.track) {
        return { ...s, queue: [s.track], queueIndex: 0, shuffleOrder: [] };
      }
      return { ...s, queue: [], queueIndex: -1, shuffleOrder: [] };
    });
  },

  toggle() {
    if (!audioEl) return;
    if (audioEl.paused) audioEl.play();
    else audioEl.pause();
  },

  next() {
    const s = get(state);
    if (s.queue.length === 0) return;
    const eIdx = effectiveIndex(s);
    const nextPos = eIdx + 1;

    if (nextPos < s.queue.length) {
      loadTrackAtIndex(resolveIndex(s, nextPos));
    } else if (s.loop === "all") {
      if (s.shuffle) {
        const newOrder = generateShuffleOrder(s.queue.length);
        state.update((st) => ({ ...st, shuffleOrder: newOrder }));
        loadTrackAtIndex(newOrder[0]);
      } else {
        loadTrackAtIndex(0);
      }
    }
  },

  prev() {
    if (!audioEl) return;
    // If more than 3s in, restart current track
    if (audioEl.currentTime > 3) {
      audioEl.currentTime = 0;
      return;
    }
    const s = get(state);
    if (s.queue.length === 0) return;
    const eIdx = effectiveIndex(s);
    const prevPos = eIdx - 1;

    if (prevPos >= 0) {
      loadTrackAtIndex(resolveIndex(s, prevPos));
    } else if (s.loop === "all") {
      const lastPos = s.queue.length - 1;
      loadTrackAtIndex(resolveIndex(s, lastPos));
    } else {
      audioEl.currentTime = 0;
    }
  },

  cycleLoop() {
    state.update((s) => {
      const modes: LoopMode[] = ["off", "all", "one"];
      const next = modes[(modes.indexOf(s.loop) + 1) % modes.length];
      return { ...s, loop: next };
    });
  },

  toggleShuffle() {
    state.update((s) => {
      const shuffle = !s.shuffle;
      const shuffleOrder = shuffle
        ? generateShuffleOrder(s.queue.length, s.queueIndex)
        : [];
      return { ...s, shuffle, shuffleOrder };
    });
  },

  seek(time: number) {
    if (!audioEl) return;
    audioEl.currentTime = time;
  },

  seekFraction(frac: number) {
    if (!audioEl) return;
    audioEl.currentTime = frac * audioEl.duration;
  },

  stop() {
    if (!audioEl) return;
    stopRAF();
    audioEl.pause();
    audioEl.src = "";
    state.set({
      track: null,
      playing: false,
      currentTime: 0,
      duration: 0,
      loading: false,
      queue: [],
      queueIndex: -1,
      loop: "off",
      shuffle: false,
      shuffleOrder: [],
    });
  },

  getAudioElement(): HTMLAudioElement | null {
    return audioEl;
  },
};
