import { writable } from "svelte/store";

type Theme = "dark" | "light";

const stored = localStorage.getItem("theme") as Theme | null;
const preferred = window.matchMedia("(prefers-color-scheme: light)").matches
  ? "light"
  : "dark";

export const theme = writable<Theme>(stored ?? preferred);

theme.subscribe((t) => {
  localStorage.setItem("theme", t);
  document.documentElement.classList.toggle("light", t === "light");
});

export function toggleTheme() {
  theme.update((t) => (t === "dark" ? "light" : "dark"));
}
