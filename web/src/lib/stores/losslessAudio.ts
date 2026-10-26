import { writable, get } from "svelte/store";

// Stream the original WAV/FLAC instead of the Opus transcode. Remembered per browser.
const stored = localStorage.getItem("losslessAudio") === "1";

export const losslessAudio = writable<boolean>(stored);

losslessAudio.subscribe((on) => {
  localStorage.setItem("losslessAudio", on ? "1" : "0");
});

/** Stream URL for a track, honouring the lossless preference. */
export function trackStreamUrl(bandSlug: string, trackId: string, lossless = get(losslessAudio)): string {
  const url = `/api/bands/${bandSlug}/tracks/${trackId}/stream`;
  return lossless ? `${url}?hq=1` : url;
}

/** Whether the server can stream this track's original losslessly (see browserLosslessType). */
export function hasLosslessOriginal(format: string | undefined): boolean {
  // Big-endian PCM is AIFF, which browsers can't decode.
  return !!format && (format === "flac" || (format.startsWith("pcm_") && !format.endsWith("be")));
}
