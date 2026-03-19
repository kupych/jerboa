import { writable, derived } from "svelte/store";

export const hash = writable(window.location.hash.slice(1) || "/");

window.addEventListener("hashchange", () => {
  hash.set(window.location.hash.slice(1) || "/");
});

export function navigate(path: string) {
  window.location.hash = path;
}

export const route = derived(hash, ($hash) => {
  const parts = $hash.split("/").filter(Boolean);
  return {
    path: $hash,
    parts,
    // /band/:slug
    isBand: parts[0] === "band" && parts.length === 1,
    // /band/:slug/track/:id
    isTrack: parts[0] === "band" && parts[2] === "track",
    // /invite/:token
    isInvite: parts[0] === "invite",
    bandSlug: parts[0] === "band" ? parts[1] : null,
    trackId: parts[0] === "band" && parts[2] === "track" ? parts[3] : null,
    inviteToken: parts[0] === "invite" ? parts[1] : null,
  };
});
