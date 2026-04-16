export interface ColorScheme {
  name: string;
  accent: string;
  accentHover: string;
  accentMuted: string;
  waveform: string;
  waveformProgress: string;
  marker: string;
  markerHover: string;
}

export const colorSchemes: Record<string, ColorScheme> = {
  teal: {
    name: "Teal",
    accent: "#2ec4b6",
    accentHover: "#4ad4c8",
    accentMuted: "#2ec4b622",
    waveform: "#2ec4b6",
    waveformProgress: "#6ee0d8",
    marker: "#f0a030",
    markerHover: "#f4b454",
  },
  violet: {
    name: "Violet",
    accent: "#8b5cf6",
    accentHover: "#a78bfa",
    accentMuted: "#8b5cf622",
    waveform: "#8b5cf6",
    waveformProgress: "#b49ffa",
    marker: "#d4e157",
    markerHover: "#e6ee9c",
  },
  rose: {
    name: "Pink",
    accent: "#ec4899",
    accentHover: "#f472b6",
    accentMuted: "#ec489922",
    waveform: "#ec4899",
    waveformProgress: "#f9a8d4",
    marker: "#34d399",
    markerHover: "#6ee7b7",
  },
  amber: {
    name: "Amber",
    accent: "#f59e0b",
    accentHover: "#fbbf24",
    accentMuted: "#f59e0b22",
    waveform: "#f59e0b",
    waveformProgress: "#fcd34d",
    marker: "#818cf8",
    markerHover: "#a5b4fc",
  },
  lime: {
    name: "Lime",
    accent: "#84cc16",
    accentHover: "#a3e635",
    accentMuted: "#84cc1622",
    waveform: "#84cc16",
    waveformProgress: "#bef264",
    marker: "#f472b6",
    markerHover: "#f9a8d4",
  },
  cyan: {
    name: "Cyan",
    accent: "#06b6d4",
    accentHover: "#22d3ee",
    accentMuted: "#06b6d422",
    waveform: "#06b6d4",
    waveformProgress: "#67e8f9",
    marker: "#fb923c",
    markerHover: "#fdba74",
  },
  fuchsia: {
    name: "Fuchsia",
    accent: "#d946ef",
    accentHover: "#e879f9",
    accentMuted: "#d946ef22",
    waveform: "#d946ef",
    waveformProgress: "#f0abfc",
    marker: "#4ade80",
    markerHover: "#86efac",
  },
  orange: {
    name: "Orange",
    accent: "#f97316",
    accentHover: "#fb923c",
    accentMuted: "#f9731622",
    waveform: "#f97316",
    waveformProgress: "#fdba74",
    marker: "#38bdf8",
    markerHover: "#7dd3fc",
  },
};

export function applyColorScheme(scheme: string) {
  const colors = colorSchemes[scheme] || colorSchemes.teal;
  const root = document.documentElement;
  root.style.setProperty("--color-accent", colors.accent);
  root.style.setProperty("--color-accent-hover", colors.accentHover);
  root.style.setProperty("--color-accent-muted", colors.accentMuted);
  root.style.setProperty("--color-waveform", colors.waveform);
  root.style.setProperty("--color-waveform-progress", colors.waveformProgress);
  root.style.setProperty("--color-marker", colors.marker);
  root.style.setProperty("--color-marker-hover", colors.markerHover);
}

export function resetColorScheme() {
  const root = document.documentElement;
  const props = [
    "--color-accent", "--color-accent-hover", "--color-accent-muted",
    "--color-waveform", "--color-waveform-progress",
    "--color-marker", "--color-marker-hover",
  ];
  for (const prop of props) {
    root.style.removeProperty(prop);
  }
}
