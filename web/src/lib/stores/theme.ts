import { writable, derived } from "svelte/store";

export type ThemePreference = "dark" | "light" | "system";

const stored = localStorage.getItem("theme") as ThemePreference | null;

export const themePreference = writable<ThemePreference>(stored ?? "system");

function getSystemTheme(): "dark" | "light" {
  return window.matchMedia("(prefers-color-scheme: light)").matches
    ? "light"
    : "dark";
}

export const theme = derived(themePreference, ($pref) =>
  $pref === "system" ? getSystemTheme() : $pref
);

// Listen for system theme changes
window
  .matchMedia("(prefers-color-scheme: light)")
  .addEventListener("change", () => {
    // Re-derive by triggering a no-op update
    themePreference.update((p) => p);
  });

themePreference.subscribe((pref) => {
  localStorage.setItem("theme", pref);
});

theme.subscribe((t) => {
  document.documentElement.classList.toggle("light", t === "light");
});

export function setTheme(pref: ThemePreference) {
  themePreference.set(pref);
}

// Keep toggleTheme for Login page
export function toggleTheme() {
  themePreference.update((p) => {
    const resolved = p === "system" ? getSystemTheme() : p;
    return resolved === "dark" ? "light" : "dark";
  });
}
