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
  // Only store pathname (strip query params) so route matching is clean
  const url = new URL(to, window.location.origin);
  path.set(url.pathname);
}

// Tabs inside BandView that are reflected in the URL as /band/:slug/:tab.
// "feed" is the default and stays at the bare /band/:slug.
export const BAND_TABS = ["feed", "songs", "sets", "tags", "files", "sync"] as const;
export type BandTab = (typeof BAND_TABS)[number];

export const route = derived(path, ($path) => {
  const parts = $path.split("/").filter(Boolean);
  const bandTab =
    parts[0] === "band" && parts.length === 3 && (BAND_TABS as readonly string[]).includes(parts[2])
      ? (parts[2] as BandTab)
      : null;
  return {
    path: $path,
    parts,
    // /band/:slug, with or without a tab segment
    isBand: parts[0] === "band" && (parts.length === 2 || bandTab !== null),
    // /band/:slug/track/:id
    isTrack: parts[0] === "band" && parts[2] === "track",
    // /band/:slug/song/:id
    isSong: parts[0] === "band" && parts[2] === "song",
    // /band/:slug/set/:id
    isSet: parts[0] === "band" && parts[2] === "set" && parts.length === 4,
    // /band/:slug/set/:id/perform OR /band/:slug/perform (freestyle)
    isPerform: parts[0] === "band" && ((parts[2] === "set" && parts[4] === "perform") || (parts[2] === "perform" && parts.length === 3)),
    // /band/:slug/tag/:tag
    isTag: parts[0] === "band" && parts[2] === "tag" && parts.length === 4,
    // /band/:slug/car
    isCar: parts[0] === "band" && parts[2] === "car",
    // /invite/:token
    isInvite: parts[0] === "invite",
    // /profile
    isProfile: parts[0] === "profile",
    // /admin
    isAdmin: parts[0] === "admin",
    // /changelog
    isChangelog: parts[0] === "changelog",
    // /no-access
    isNoAccess: parts[0] === "no-access",
    // /auth/open
    isAuthOpen: parts[0] === "auth" && parts[1] === "open",
    // /connect?code=… — Reaper device-pairing landing
    isConnect: parts[0] === "connect",
    bandSlug: parts[0] === "band" ? parts[1] : null,
    // /band/:slug/:tab
    bandTab,
    trackId: parts[0] === "band" && parts[2] === "track" ? parts[3] : null,
    songId: parts[0] === "band" && parts[2] === "song" ? parts[3] : null,
    setId: parts[0] === "band" && parts[2] === "set" ? parts[3] : null,
    tagName: parts[0] === "band" && parts[2] === "tag" ? decodeURIComponent(parts[3]) : null,
    inviteToken: parts[0] === "invite" ? parts[1] : null,
  };
});
