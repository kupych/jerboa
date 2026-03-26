export interface ChangelogEntry {
  version: string;
  date: string;
  summary: string[];
}

export const changelog: ChangelogEntry[] = [
  {
    version: "0.10.0",
    date: "2026-03-26",
    summary: [
      "Added production notes field to songs",
      "Redesigned Song page — takes first, notes/lyrics/tabs collapse into accordions",
      "Threaded comment replies",
      "Inline comment editing",
      "Clicking a timestamp marker on the waveform scrolls to the comment",
      "Clicking a timestamp in the comment list seeks the player",
      "BPM / tap tempo on songs with count-in clicks before recording",
      "Bounce to new track option for non-destructive overdub bouncing",
      "Restore original track from bounced versions",
      "Overdub punch-in recording from any position",
      "Overdub file upload with offset",
      "Massively improved overdub sync via Web Audio API",
      "Theme selector moved to profile — now supports dark, light, and system modes",
      "What's new modal + changelog page",
    ],
  },
];

export const CURRENT_VERSION = changelog[0].version;
