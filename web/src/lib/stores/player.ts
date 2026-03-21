// Global player controls.
// Views register their media element directly — no DOM querying.

let _el: HTMLMediaElement | null = null;
let _onNext: (() => void) | null = null;
let _onPrev: (() => void) | null = null;

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];

export const player = {
  setMediaElement(el: HTMLMediaElement | null) {
    _el = el;
  },

  registerPlaylist(opts: { onNext?: () => void; onPrev?: () => void }) {
    _onNext = opts.onNext ?? null;
    _onPrev = opts.onPrev ?? null;
  },

  unregisterPlaylist() {
    _onNext = null;
    _onPrev = null;
  },

  toggle() {
    if (!_el) return;
    if (_el.paused) _el.play();
    else _el.pause();
  },

  seekRelative(seconds: number) {
    if (!_el) return;
    _el.currentTime = Math.max(
      0,
      Math.min(_el.duration || 0, _el.currentTime + seconds),
    );
  },

  restart() {
    if (!_el) return;
    _el.currentTime = 0;
    _el.play();
  },

  next() {
    _onNext?.();
  },

  prev() {
    _onPrev?.();
  },

  toggleMute(): boolean | null {
    if (!_el) return null;
    _el.muted = !_el.muted;
    return _el.muted;
  },

  speedUp(): number | null {
    if (!_el) return null;
    const idx = SPEEDS.findIndex((s) => Math.abs(s - _el!.playbackRate) < 0.01);
    if (idx >= 0 && idx < SPEEDS.length - 1) {
      _el.playbackRate = SPEEDS[idx + 1];
    } else if (idx === -1) {
      _el.playbackRate = 1;
    }
    return _el.playbackRate;
  },

  speedDown(): number | null {
    if (!_el) return null;
    const idx = SPEEDS.findIndex((s) => Math.abs(s - _el!.playbackRate) < 0.01);
    if (idx > 0) {
      _el.playbackRate = SPEEDS[idx - 1];
    } else if (idx === -1) {
      _el.playbackRate = 1;
    }
    return _el.playbackRate;
  },
};
