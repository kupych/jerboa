# Jerboa Iconography Spec

## Purpose
This document defines the first icon system for Jerboa.

It is written for implementation agents and designers who need to know:

- which icons to make first
- what visual language they must follow
- where icons should appear
- where icons should **not** replace the existing coded system language

Use this alongside:

- `docs/redesign/jerboa-redesign-philosophy.md`
- `docs/redesign/jerboa-visual-language-guidelines.md`
- `docs/redesign/jerboa-redesign-agent-spec.yaml`

## Core Rule
Jerboa should use both icons and codes, but for different jobs.

### Use icons for:
- objects
- actions
- navigation
- familiar controls

Examples:

- song
- track
- comment
- set
- tag
- file
- play
- record
- upload
- download

### Keep codes for:
- system language
- edge labels
- mode language
- instrumentation
- compact structural labels

Examples:

- `OVR`
- `META`
- `TIM`
- `REC`
- `TGR`
- `ACT`

Do not flatten the whole interface into icons. The coded layer is part of the TDR/Warp flavor and should stay.

## Design Thesis
The icon set should feel like technical signage for a music tool, not a generic SaaS library.

The closest useful references are:

- hard-edged pictograms
- techno-signage glyphs
- Vectorheart-like geometry
- engineered symbols with attitude

The icons should feel:

- severe
- compact
- legible
- one-color first
- schematic rather than illustrative

They should not feel:

- friendly
- rounded
- app-store generic
- soft
- cute
- ornamental

## System Rules
### 1. One-Color First
Every icon must work as a single-color mark.

No gradients.
No multi-tone shading.
No built-in glow.

State should come from UI color, not from the icon asset.

### 2. Clear Silhouette First
The icon must read at a glance before any internal detail is noticed.

If the silhouette fails at `12px` or `16px`, the icon is too detailed.

### 3. Geometric Discipline
Preferred geometry:

- square or near-square proportions
- straight segments with a few deliberate curves
- bevels or sharp terminals where useful
- minimal internal breaks

Avoid:

- bubbly circles
- playful curves
- hand-drawn looseness
- tiny decorative cuts that disappear at small sizes

### 4. Stroke Logic Must Be Consistent
Choose one family logic and stay there.

Recommended default:

- outline-first
- consistent stroke weight
- occasional solid-fill exceptions for very universal symbols

Good fill exceptions:

- play
- pause
- record dot
- chevrons

Do not mix thin-outline icons with chunky filled icons unless the distinction is intentional and systematic.

### 5. Optical Weight Matters More Than Literal Geometry
Icons must feel equally weighted at the same rendered size.

That means:

- some icons will need larger internal shapes
- some icons will need less detail
- some icons will need tighter framing

Perfect geometry is less important than balanced visual weight.

## Master Build Specs
### Source Grid
Draw on a `24x24` master grid.

Recommended safe area:

- keep major strokes inside a `20x20` live area
- reserve outer edges for optical breathing room

### Rendered Sizes
Design for these output sizes first:

- `12px`
- `16px`
- `20px`
- `24px`

These should cover almost all Jerboa use.

### Stroke Weight
Recommended starting point on the `24x24` master:

- `1.75px` or `2px`

Then visually tune, not mathematically tune.

If you use a slightly heavier stroke, make sure the set still breathes at `12px`.

### Corner Treatment
Pick one of these and keep it consistent:

- squared
- slightly chamfered
- tight-radius but not friendly-round

Recommended:

- squared or subtly chamfered

### Terminal Treatment
Be deliberate about line endings.

Recommended:

- square terminals
- occasional angled cuts where the icon benefits from a more mechanical feel

Avoid casual rounded terminals unless the whole family is built around them.

## State Rules
Icons should not encode state by themselves beyond simple shape changes like play/pause.

State comes from:

- color
- surrounding panel
- border treatment
- selection treatment
- mode bars

Preferred state behavior:

- default = muted
- hover = brighter
- active/current = accent
- danger = semantic danger color
- disabled = lower opacity, no special shape change

## What To Replace And What To Keep
### Replace With Icons
These are the best candidates for icon treatment:

- playback controls
- comment/chat buttons
- upload/download
- link/import
- add/remove
- edit/delete
- tag/file
- song/track/set indicators
- search
- settings/admin
- invite/member

### Keep As Codes
These should stay text-coded:

- edge labels
- mode bars
- section instrumentation labels
- compact system stamps
- tiny rail markers

Examples:

- `OVR`
- `META`
- `TIM`
- `REC`
- `ACT`
- `TGR`

### Mixed Strategy
In several places, the best solution is `icon + label`, not icon-only.

Use `icon + label` in:

- buttons
- action strips
- empty states
- forms
- lower-frequency actions

Use `icon-only` in:

- compact headers
- transport controls
- familiar utility buttons
- repeated toolbar actions

## First Production Set
This is the first icon inventory worth building. It is intentionally small.

### Tier 1: Essential Actions
- play
- pause
- stop
- record
- next
- previous
- upload
- download
- add
- remove
- edit
- delete
- save
- search
- close
- chevron-down
- chevron-up
- chevron-right

### Tier 2: Core Product Objects
- song
- track
- comment
- chat
- setlist
- tag
- file
- member
- invite-member
- admin
- settings

### Tier 3: Jerboa-Specific Objects
- overdub
- queue-add
- lyrics
- chords-tabs
- perform-live
- mixer
- marker
- revision-version

If the custom workload needs to stay small, start with Tier 1 and Tier 2 only.

## Recommended Meanings
These are the intended semantic reads for the Jerboa-specific icons.

### Song
Should read like a document/song object, not merely a music note.

Possible approach:

- document sheet with one music or lyric cue
- compact score-like glyph

### Track
Should read like recorded audio, not a playlist item.

Possible approach:

- waveform strip
- reel/take object
- compact audio lane glyph

### Comment
Should read as review/annotation, not chat-only.

Possible approach:

- speech mark with a marker cue
- comment balloon with a timing notch

### Setlist
Should read as ordered running sequence.

Possible approach:

- numbered stacked lines
- vertical sequence bars

### Overdub
Should read as layer-on-layer, not just another track.

Possible approach:

- stacked waveform
- offset double-lane glyph

### Lyrics
Should read as readable text content.

Possible approach:

- document with line pattern
- page with verse bars

### Chords/Tabs
Should feel more schematic than lyrical.

Possible approach:

- grid or fret-like structure
- chord block with line segmentation

### Perform-Live
Should read as stage mode, not just music.

Possible approach:

- stand/mic/stage glyph
- directional “live” cue with note marker

### Mixer
Should read as lanes and adjustment, not generic settings.

Possible approach:

- vertical channel strips
- rigid sliders with meter logic

### Marker
Should read as timeline/timestamp annotation.

Possible approach:

- flag or locator
- pin adapted for waveform/timeline use

## Placement Map
### Header
Use icons for:

- chat
- notifications
- admin
- user/profile

Keep coded text for:

- version/build string
- band/system chips

### Band Page
Use icons for:

- play all
- perform
- upload
- record
- import
- invite
- settings

Do **not** rush to replace the activity feed codes yet. `TRK / CMT / ODB / SNG` are currently useful and on-brand in that dense context.

### Song Page
Use icons for:

- takes
- sets
- notes
- lyrics
- tabs/chords
- edit/save

Best starting use:

- section headers
- action buttons

### Track Page
Use icons for:

- playback
- comment
- markers
- upload overdub
- record overdub
- link existing
- tags
- edit/delete

Keep coded labels for:

- `PLAYBACK`
- `MIX`
- `REVIEW`
- `META`

### Set Page
Use icons for:

- perform
- tag timestamps
- add song
- assign recording
- reorder
- delete item

Keep coded labels for:

- `SLT`
- `REC`
- `TIM`
- `TGR`

### Perform Page
Use icons for:

- next
- previous
- record
- chords toggle if useful
- exit

Keep the page otherwise very restrained.

## Feed-Specific Rule
The band activity feed is the hardest place to swap codes for icons.

Rules:

- do not replace feed codes with icons unless the icons are brutally legible at very small sizes
- if tested icons feel mushy at `12-14px`, keep the codes
- if icons are introduced, test them against `TRK / CMT / ODB / SNG` before committing

The feed is dense. Legibility beats novelty there.

## Search Criteria For Off-The-Shelf Sets
If sourcing instead of drawing, look for:

- sharp geometry
- squared or engineered feel
- one-color compatibility
- clean small-size rendering
- minimal softness
- a complete enough family to cover the first production set

Reject sets that feel:

- rounded
- generic
- startup-friendly
- too thin
- too decorative
- too detailed at small sizes

## If You Draw Them Yourself
Keep the first custom family small.

Recommended first pass:

- play
- pause
- record
- upload
- download
- add
- close
- comment
- song
- track
- setlist
- tag
- file
- settings

This is enough to establish the grammar without turning icon work into its own project.

## Acceptance Criteria
The icon set is ready when:

- every icon reads at `16px`
- the important icons still read at `12px`
- the whole family feels like one system
- icons do not fight the existing code language
- object icons and action icons are visually consistent
- the set looks specific to Jerboa rather than pasted in from generic product UI

## Short Version
Make icons for objects and actions.
Keep codes for system language.
Draw them like hard-edged music-tool glyphs, not friendly app symbols.
