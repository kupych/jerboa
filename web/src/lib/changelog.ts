export interface ChangelogEntry {
  version: string;
  date: string;
  summary: string[];
}

export const changelog: ChangelogEntry[] = [
  {
    version: "0.24.0",
    date: "2026-05-23",
    summary: [
      "Big files allowed now. Just don't upload anything illegal pls"
    ]
  },
  {
    version: "0.20.0",
    date: "2026-04-15",
    summary: [
      "Went even harder on the look. Hope it's not too much of a shift."
    ],
  },
  {
    version: "0.19.0",
    date: "2026-04-14",
    summary: [
      "Link existing tracks as overdubs — no re-upload needed. Pick any track in the band, clone it into the mixer, and the original stays exactly where it was.",
      "Fixed a sneaky bug where MediaRecorder recordings would decode correctly but play for exactly 1 millisecond. What is this, Napalm Death's 'You Suffer'?"
    ],
  },
  {
    version: "0.18.0",
    date: "2026-04-12",
    summary: [
      "Fader positions are saved so you can keep that tiger tamed",
      "Auto-normalization! Not perfect but it'll get you partway there",
    ],
  },
  {
    version: "0.17.0",
    date: "2026-04-11",
    summary: [
      "Timed comments on the waveform now actually work the way they should",
      "Fixed a dumb bug where if a take was a multiple of 3 in duration it wouldn't play. Fuck float math",
    ],
  },
  {
    version: "0.16.0",
    date: "2026-04-08",
    summary: [
      "DAW-style multitrack mixer: pixel-based timeline, horizontal scrolling, zoom controls — it actually looks like a real DAW now",
      "Reaper session tracks get their own dedicated view: skip the waveform player, go straight to the mixer with all your takes laid out",
      "Fixed the WebM/Opus pipeline for good — recordings now encode as Ogg/Opus and stream synchronously, so the mixer can decode them reliably every time",
      "RPP session sync enhancements and improved cachebuster handling",
    ],
  },
  {
    version: "0.15.0",
    date: "2026-04-05",
    summary: [
      "Reaper session sync! Download a pre-baked utility from band settings, drop it in your project folder, run it. Your takes teleport to Jerboa. The .rpp gets versioned too — time machine included.",
      "Recording now holds a wake lock so your phone screen won't bail mid-rehearsal and nuke your take",
      "Recorded takes are now backed up to IndexedDB as they happen — if the page crashes, the recovery banner will be waiting for you when you come back",
      "Files tab: upload anything (Reaper sessions, stems, your drummer's grocery list) — backed by object storage so your server's SSD can breathe",
    ]
  },
  {
    version: "0.14.0",
    date: "2026-04-04",
    summary: [
      "Check out that new 'Files' tab - upload any file you desire. But not too much. SSDs don't grow on trees."
    ]
  },
  { 
    version: "0.13.0",
    date: "2026-04-04",
    summary: [
      "Activity feed - see what your band's been up to at a glance",
      "Some UX enhancements to hopefully make navigation a little easier"
    ]
  },
  {
    version: "0.12.3",
    date: "2026-03-30",
    summary: [
      "Tags, baby! Now you can sort by tags."
    ],
  },
  {
    version: "0.12.0",
    date: "2026-03-29",
    summary: [
      "Multi-track mixer! Record overdubs and play them back with mute/solo/gain controls — great for vocal harmony practice ;)",
      "Selective bounce: bounce only unmuted tracks to a new track or in-place",
      "Song view: download individual takes from set recordings with proper song names in the filename",
      "Unfucked magic link for PWA"
    ],
  },
  {
    version: "0.11.0",
    date: "2026-03-27",
    summary: [
      "Persistent global player — play and queue tracks from anywhere, playback continues across pages",
      "Play and enqueue buttons on every track card",
      "Added recording and track tagging to Perform mode, to save those sweet sweet jams",
      "Collapsible player bar with queue management, shuffle, loop modes",
      "New car mode (Play All): it really whips the llama's ass",
      "Pending invites now shown in band roster",
      "Song view: takes section redesigned with inline playback",
      "Desktop layout and spacing improvements across BandView",
    ],
  },
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
