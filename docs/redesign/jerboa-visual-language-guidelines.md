# Jerboa Visual Language Guidelines

## Purpose
This document defines the visual language for Jerboa after the first live style pass. It is meant for implementation agents and designers who need concrete rules, not vague inspiration.

Use this alongside:

- `docs/redesign/jerboa-redesign-agent-spec.yaml`
- `docs/redesign/jerboa-redesign-philosophy.md`

Use the current implementation in these files as the reference baseline:

- `web/src/app.css`
- `web/src/lib/components/Header.svelte`
- `web/src/routes/BandView.svelte`
- `web/src/routes/TrackView.svelte`
- `web/src/routes/SetView.svelte`

## Design Thesis
Jerboa should feel like a technical music instrument, not a styled app shell.

The aesthetic reference is adjacent to The Designers Republic and Warp Records, but the goal is not nostalgia and not fake Y2K. The useful part of that reference is:

- severe hierarchy
- coded chrome
- hard geometry
- decisive state
- strong visual roles
- high readability through contrast and proportion

The correct move is not "add more style." The correct move is "make the system harsher, clearer, and more disciplined."

## Non-Negotiable Rules
### 1. Readability Improves Because the UI Gets Harder
Go harder by making hierarchy more brutal:

- bigger anchors
- calmer body text
- smaller but clearer metadata
- more obvious section breaks
- more binary active states

Do not go harder by making everything tiny, washed out, or over-labeled.

### 2. One Dominant Anchor Per Screen
Each page gets one thing that wins immediately:

- Dashboard: the band list
- Band: the activity feed
- Song: the document surfaces
- Track: the waveform and mix surface
- Set: the setlist and timestamp workspace
- Perform: the live reference content

If two areas feel equally important above the fold, the hierarchy is wrong.

### 3. Content Is Larger Than Chrome
Lyrics, notes, comments, titles, take names, and song names are content.

Labels, codes, counters, mode tags, timestamps, and admin controls are chrome.

Chrome can be small. Content cannot.

### 4. Accent Means Something
Accent color is reserved for:

- active
- current
- primary
- armed

The working rule is:

> if everything glows, nothing does

Accent is not decoration. It is a signal. If a state is not important enough to deserve immediate attention, it should not use accent.

### 5. Geometry Carries the Attitude
Jerboa should feel sharp because of:

- slab panels
- hard edges
- technical dividers
- asymmetry
- controlled whitespace
- strong title blocks

Do not rely on decorative gradients, ornamental iconography, or novelty components to create attitude.

## Theme Philosophy
### Dark Mode
Dark mode is the canonical emotional baseline.

It should feel:

- dense
- controlled
- machine-like
- high-contrast
- deliberate

Use darker backgrounds and brighter text rather than stacking multiple accent effects.

### Light Mode
Light mode is not an inversion of dark mode.

It should feel like printed technical matter:

- cool field
- darker rules
- clearer borders
- white panels
- restrained accent moments

The light theme should not drift into generic admin UI. It should stay technical and slightly severe.

### Color Discipline
These semantic rules must survive both themes:

- `accent` = active, current, primary, armed
- `danger` = destructive or error
- `success` = confirmed or complete
- `text-muted` = metadata and chrome, never primary reading text
- `border` = structure, never mood

Band color may personalize the interface, but it must never weaken semantic meaning. `danger` cannot become ambiguous because a band accent is similar.

## Typography Roles
### Primary Typeface Roles
- Display: `Chakra Petch`
- Body: `Rajdhani`
- Code/system layer: existing mono stack

### Title Rules
- Page titles should be large, uppercase, and unmistakable.
- Desktop titles should feel close to poster scale on key pages.
- Titles should usually sit inside a title block with a coded kicker above them.

### Chrome Rules
- Microtype is uppercase, tracked, and weighted.
- System labels use code-like language and should feel instrument-grade, not decorative.
- Microtype should never carry the core reading load.

### Content Rules
- Body copy stays calm and readable.
- Notes, lyrics, comments, and prose should not be forced into the coded system voice.
- The wider and louder the chrome becomes, the calmer content must become in response.

## Surface and Layout Rules
### Surfaces
Use panels as structural slabs, not floating cards.

Preferred characteristics:

- square corners
- explicit border
- restrained inner shadow
- thin but visible dividers
- grouped content with one clear role

Avoid:

- pill-heavy layouts
- soft glassy surfaces
- floating card stacks
- ornamental nested boxes

### Borders
- `1px` borders are structural default.
- Accent borders should appear only for active or selected states.
- `2px` accent treatments are acceptable when marking current items or active regions.

### Asymmetry
Desktop layouts should lean into asymmetry.

The main canvas should dominate. Side rails should read like instrument sidecars, not equal peers. This is especially important on:

- Track
- Set
- Song desktop editor layouts

### Negative Space
Use space to separate modules, not to make the interface feel luxury-soft. Jerboa should feel taut, not fluffy.

## Motion Rules
Motion should feel dry and mechanical.

Preferred behavior:

- fast fades
- short slides
- crisp dropdowns
- minimal easing

Avoid:

- floaty entrances
- springy interactions
- ornamental parallax
- soft consumer-app motion

A panel should feel like it opens or reveals. It should not feel like it drifts.

## TDR/Warp Extensions
These are sanctioned ways to push the visual language harder without breaking usability.

Use them selectively. They should sharpen hierarchy and identity, not turn every page into a poster.

### 1. Oversized Numeric Anchors
Use giant numerals or counters when a number is central to the page's meaning.

Good candidates:

- `7 SONGS`
- `3 TAKES`
- `2 MARKERS`
- `120 BPM`
- `36:38`

Rules:

- the number must reflect the page's dominant anchor or primary task
- it should sit near the title or section header, not deep inside metadata
- it should feel like a structural heading, not decorative wallpaper
- use at most one oversized numeric anchor per major page region

Best uses:

- Dashboard counts
- Band overview counts
- Song BPM or take totals
- Track marker counts or duration
- Set song count or runtime

Avoid:

- scattering giant numbers everywhere
- using oversized numbers for low-value metadata
- letting the number outshout the page title unless it is the page title

### 2. Edge Labeling
Use short coded labels on the edge of rails, modules, or page regions to make the interface feel cataloged and instrument-like.

Examples:

- `OVR`
- `META`
- `MIX`
- `LIVE`
- `REV`

Rules:

- edge labels must be short, coded, and quiet
- they belong on side rails, module edges, drawer edges, or narrow vertical surfaces
- they should support orientation, not replace the main heading
- they should be aligned to the geometry of the layout, not floating freely

Best uses:

- Track side rails
- Set planner side modules
- Band overview rails
- narrow utility modules in desktop layouts

Avoid:

- putting edge labels on every panel
- using long words where a short code works better
- making them brighter than the main content

### 3. Mode Bars
Whenever the product enters a special mode, use a dedicated bar or banner to make that state unmistakable.

Good candidates:

- tagging
- armed record
- queue active
- perform live
- review focus

Rules:

- mode bars should be immediate and hard to miss
- they should span the local work surface, not hide inside a small chip
- they may use accent or high-contrast reversal because mode changes are high-value state
- their copy should be blunt and operational

Examples:

- `TAGGING MODE`
- `RECORD ARMED`
- `QUEUE ACTIVE`
- `LIVE MODE`

Best uses:

- Set timestamp tagging
- Track overdub and record states
- Perform mode variants
- any temporary state that changes how clicks or taps behave

Avoid:

- using a mode bar for routine passive states
- stacking multiple mode bars on one screen
- burying mode state inside metadata rows

### 4. Poster-Like Empty States
Empty states should feel designed, not apologetic.

The correct mood is not "nothing here." The correct mood is "this slot is ready."

Rules:

- one bold anchor
- one line of utility instruction
- one obvious next action
- optional code or count treatment

Good structure:

- big code or title
- one short supporting line
- one CTA or next step

Examples:

- `NO SONGS`
- `0 TAKES`
- `QUEUE EMPTY`
- `NO RECORDINGS ASSIGNED`

Best uses:

- Dashboard when there are no bands
- Band tabs with no songs, sets, or activity
- Song pages with no takes or empty document sections
- Set pages with no recordings

Avoid:

- multi-paragraph empty states
- playful filler language
- generic illustration-driven empties
- more than one CTA in an empty state

## Shared Primitives In Code
Prefer reusing the existing system utilities before inventing new visual patterns.

### `sys-kicker`
Use for page-level coded kickers above major titles.

Examples:

- `band channel // live workspace`
- `track console // review + overdub`
- `set planner // running order + timestamps`

### `sys-title-block`
Use to give titles a strong base and a clean separation from the next zone.

Best for:

- main page titles
- high-importance editor headers

### `sys-chip`
Use for neutral coded chips and system tags.

Good uses:

- member identity
- system labels
- mode markers
- neutral state tags

Do not use `sys-chip` for primary call-to-action buttons.

### `sys-chip-accent`
Use for meaningful, active, or linked states.

Good uses:

- `song linked`
- current mode
- armed state

Do not use this for passive metadata.

### `sys-panel`
Use for the main slab treatment.

Good uses:

- activity rows
- setlist rows
- grouped list items
- content modules that need structural presence

### `sys-panel-muted`
Use for quieter metadata blocks that still need to read as designed.

Good uses:

- track metadata strip
- secondary info bands

### `sys-section-head`
Use for coded section headers that separate one work zone from another.

This should be the default pattern for:

- waveform sections
- setlist sections
- recordings sections
- review/comment zones

It should make the page feel like a console made of named work areas.

### `sys-code`
Use for tiny coded text where the UI needs instrumentation language rather than prose.

Good uses:

- counters
- short state codes
- tiny system markers
- version/build strings

## Page Motifs
### Header
The header should feel like the product's instrument rail.

Rules:

- keep the logo and band switcher where users expect
- use coded system language sparingly
- do not let header chrome compete with page titles
- version/build strings should feel like instrumentation, not a joke

### Dashboard
The dashboard is a switchboard.

It should feel:

- sparse
- direct
- easy to scan

Do not overbuild it with decorative modules. The loudest thing should be the band identity, not a dashboard framework.

### Band
The band page is an event console and launchpad.

Key motif:

- feed items as coded readouts

Rules:

- activity feed gets the strongest visual weight
- add-material should read as one slab, not three unrelated buttons
- tabs stay familiar but should feel more exact and less mushy
- member context should read as status, not decoration

The user should feel that the band page shows what is happening now.

### Song
The song page is a document surface.

Rules:

- notes, lyrics, and tabs should feel editorial and readable
- takes and sets support the document, but do not visually overpower it
- desktop should use more editorial asymmetry
- mobile should keep the familiar accordion model

The right mood is "working document with attached evidence."

### Track
The track page is the deepest technical surface in the product.

Key motif:

- console + canvas + sidecar

Rules:

- waveform and mixer are the hero
- metadata and assignment live in the rail
- section headers should make the surface feel like instrument zones
- the page should feel closer to a console than a generic detail page

The user should feel they are inside a serious working surface.

### Set
The set page is a planner and timestamp workbench.

Key motif:

- setlist on one side, timing canvas on the other

Rules:

- timestamp workspace must feel like a real canvas
- setlist rows should be precise and systematic
- tagging mode must be unmistakable
- recording assignment stays structured and secondary to the main timing task

The user should feel they are preparing a live document, not browsing a record.

### Perform
Perform is the most disciplined page.

Rules:

- keep it direct
- keep it calm
- keep it trustworthy
- improve legibility, not style density

Perform should look like a stage tool, not a design exercise.

## State Language
States should be obvious and mostly binary.

### Active
Use accent or strong border treatment. The user should see it instantly.

### Current
The current item should feel anchored and unmistakable. Use accent only if it truly matters to the present task.

### Armed
Armed states can use the loudest accent treatment because they imply risk or immediacy.

### Selected
Selection should be clear through border, surface shift, or code tag. Do not rely on tiny text-color changes alone.

### Passive
Passive controls should stay quiet. Muted text and structural borders should do most of the work.

## Copy and Label Language
Jerboa's chrome copy should read like utility language, not lifestyle branding.

Good characteristics:

- short
- coded
- concrete
- operational

Examples:

- `REC // recordings`
- `TIM // timestamp workspace`
- `PLAYBACK // waveform + markers`

Avoid:

- cute labels
- clever jokes in repeated chrome
- vague marketing language inside product surfaces

## Implementation Rules For Agents
### Before Styling a Screen
Answer these questions:

1. What is the dominant anchor?
2. What is the primary task?
3. What belongs to content and what belongs to chrome?
4. What state must be obvious at a glance?
5. What can be quieter?

If you cannot answer those cleanly, do not start styling yet.

### When Adding New UI
- Reuse an existing primitive if it gets you 80 percent there.
- Only add a new primitive when the existing ones cannot express the role cleanly.
- Match the coded system voice for chrome.
- Keep readable content out of that coded voice.
- Preserve route structure, verbs, and familiar action zones.

### When a Design Feels Weak
Try these in order:

1. make the title larger
2. strengthen the section break
3. quiet the passive elements
4. reduce accent usage
5. give the main task more physical space

Do not start by adding more decorative detail.

### When a Design Feels Overdone
Try these in order:

1. remove accent from secondary elements
2. reduce the number of coded labels
3. calm the body text
4. remove one surface treatment
5. check whether two modules are fighting for dominance

## Review Checklist
Use this after each ticket:

- Is the page identity obvious above the fold?
- Is there one dominant anchor?
- Is content easier to read than chrome?
- Is accent visibly scarce?
- Does the UI feel more technical rather than more decorative?
- Would an existing pilot user still recognize the page instantly?
- On mobile, is the primary task reachable without visual clutter?
- On desktop, does the main canvas dominate the support rail?

## Short Version
Jerboa should feel like a hard-edged music tool.

Make it kickass by making it more decisive:

- louder titles
- quieter chrome
- sharper modules
- stricter accent discipline
- more obvious state
- better readability as a result
