import { writable, derived } from "svelte/store";

function getPath() {
  return window.location.pathname || "/";
}

export const path = writable(getPath());

window.addEventListener("popstate", () => {
  path.set(getPath());
});

export function navigate(to: string) {
  window.history.pushState(null, "", to);
  path.set(to);
}

export const route = derived(path, ($path) => {
  const parts = $path.split("/").filter(Boolean);
  return {
    path: $path,
    parts,
    // /band/:slug
    isBand: parts[0] === "band" && parts.length === 2,
    // /band/:slug/track/:id
    isTrack: parts[0] === "band" && parts[2] === "track",
    // /band/:slug/song/:id
    isSong: parts[0] === "band" && parts[2] === "song",
    // /band/:slug/set/:id
    isSet: parts[0] === "band" && parts[2] === "set",
    // /band/:slug/car
    isCar: parts[0] === "band" && parts[2] === "car",
    // /invite/:token
    isInvite: parts[0] === "invite",
    // /profile
    isProfile: parts[0] === "profile",
    // /no-access
    isNoAccess: parts[0] === "no-access",
    bandSlug: parts[0] === "band" ? parts[1] : null,
    trackId: parts[0] === "band" && parts[2] === "track" ? parts[3] : null,
    songId: parts[0] === "band" && parts[2] === "song" ? parts[3] : null,
    setId: parts[0] === "band" && parts[2] === "set" ? parts[3] : null,
    inviteToken: parts[0] === "invite" ? parts[1] : null,
  };
});
