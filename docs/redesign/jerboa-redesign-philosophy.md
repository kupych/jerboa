# Jerboa Redesign Philosophy

## Goal
Jerboa should look more extreme, more intentional, and more memorable without becoming harder to use.

The design target is not a nostalgic imitation of late-90s and early-2000s graphics. It is a modern product that uses the same strengths that made that work readable in the first place:

- aggressive hierarchy
- strong contrast
- hard module boundaries
- obvious active states
- clear separation between content and chrome

This redesign should make the product feel more kickass and more readable as the same move, not as a tradeoff.

## What Must Stay Familiar
Pilot users already know the product. Redesign should improve recognition, not replace it.

Keep these stable:

- page identities: Dashboard, Band, Song, Track, Set, Perform
- route structure and major navigation patterns
- core verbs: play, perform, record, upload, import, comment, save
- section order inside major pages unless there is a strong reason not to
- primary CTA placement in the same general zones users already learned

If a pilot user can say "this is still the band page" within a second, the redesign is on track.

## The Core Shift
The current UI already has the right product model. The main weakness is not conceptual structure. The weakness is that too many different page types are being asked to share one visual language and one layout density.

Jerboa is really several tools:

- Dashboard is a switchboard.
- Band is an inbox and launchpad.
- Song is a document with attached takes.
- Track is a workstation.
- Set is a planner and timestamp editor.
- Perform is a stage tool.

The redesign should make each page feel more like its job.

## Design Doctrine

### 1. Content Is Readable First, Chrome Is Stylized Second
Lyrics, notes, comments, take names, and song titles are content. They should be larger, calmer, and easier to read than labels, counters, and system metadata.

Microtype belongs to chrome. It should not carry the main reading workload.

### 2. Go Harder by Using Bigger Anchors
The page title, the current state, and the primary action should hit harder.

That means:

- larger titles
- more obvious section breaks
- stronger active states
- cleaner hierarchy

It does **not** mean shrinking everything and adding more labels.

### 3. One Dominant Anchor Per Screen
Every screen should answer one question instantly.

- Dashboard: where do I go?
- Band: what is happening now?
- Song: what is this song and what material belongs to it?
- Track: what am I listening to and how do I work on it?
- Set: what is the running order and where do the timestamps land?
- Perform: what do I need to see right now on stage?

### 4. Use Geometry and Proportion to Create Attitude
The aggressive aesthetic should come from proportion and zoning more than decoration.

Use:

- slab-like modules
- hard edges
- strong alignment
- disciplined accent usage
- obvious state changes

Avoid:

- soft dashboard mush
- too many equally loud text styles
- accent color sprayed across secondary information

### 5. Accent Means Something
Accent color should mean one of four things:

- current
- active
- primary
- armed

If accent is used for everything, it stops helping.

Put more bluntly:

> if everything glows, nothing does

Accent is not decoration. It is a signal. It should call attention to:

- the thing that is active
- the thing that is current
- the thing that is primary
- the thing that is armed

Everything else should rely on structure, contrast, spacing, and typography instead.

### 5a. Color Is a Discipline, Not a Skin
Color should not be applied as a mood pass after layout is finished. It is part of hierarchy.

That means:

- content gets readability first
- chrome gets restraint
- accent stays selective
- danger and success stay semantic
- band color must not erase universal meaning

The UI should feel sharp because color is disciplined, not because everything is saturated.

### 5b. Light Mode Is Not an Inversion
Light mode should not be derived by simply flipping dark tokens.

If dark mode is the canonical emotional baseline, light mode still needs its own logic:

- clearer borders
- cleaner panel separation
- more obvious content-vs-chrome contrast
- reduced washiness in neutral surfaces
- restrained accent usage so active states still pop

The goal is not to make light mode soft or generic. It should still feel technical, hard-edged, and intentional.

Think of light mode less like "white version of the same screen" and more like printed technical matter:

- brighter field
- darker rules
- stronger typographic contrast
- more controlled accent moments

In both themes, the same rule applies:

> the UI should not glow everywhere just because the palette allows it

Color emphasis must be earned by state and importance.

### 6. Desktop and Mobile Should Diverge More
Jerboa is mobile-first, but some pages are fundamentally workstation pages.

Mobile should optimize for:

- fast scan
- fast reference
- obvious next action
- thumb reach

Desktop should optimize for:

- editing
- arrangement
- comparison
- complex control surfaces

This is not inconsistency. It is good responsive design.

## Page Roles

### Dashboard
Should feel like a switchboard. Minimal, fast, and confident.

### Band
Should feel like a band's living inbox. The activity feed is the heart of the page.

### Song
Should feel like a document with attached evidence. Takes and sets support the song document; they are not the song itself.

### Track
Should feel like the most technical page in the product. On desktop it should border on DAW-like without becoming hostile.

### Set
Should feel like a planning surface. On desktop it should behave like a setlist editor plus a timestamp workbench.

### Perform
Should feel like the least designed page in the best possible sense: direct, dependable, and stage-safe.

## Readability Through Aggression
The key design belief behind this redesign is simple:

> harder hierarchy produces better readability

That means:

- bigger titles
- calmer body text
- smaller but cleaner metadata
- clearer active states
- fewer in-between styles
- more obvious grouping

The UI should feel more decisive, not busier.

## Rollout Philosophy
This should ship as a sequence of recognizable upgrades, not a relaunch.

Good rollout order:

1. shared visual system
2. band page clarity
3. song page desktop editing
4. set page planner layout
5. track page workstation layout
6. perform page polish

Do not redesign Track and Set in the same release. They are both high-complexity pages and will create too much pilot-user cognitive churn if changed together.

## Success Criteria
The redesign is successful if all of the following are true:

- old users still orient instantly
- page identity is clearer than before
- body content is easier to read
- active state is easier to detect
- desktop pages feel more powerful
- mobile pages feel less crowded
- the product looks more distinctive and more serious

## Failure Modes To Avoid

- fake Y2K: tiny text, low contrast, lots of labels, weak hierarchy
- over-rebranding: changing too many interaction patterns at once
- flattening page roles: making every page feel like the same app shell
- decorative aggression: more attitude, less usefulness
- mobile simplification that removes power-user desktop workflows

## Short Version
Make Jerboa look harder by making it clearer.

Bigger anchors.
Sharper states.
Calmer content.
Stronger modules.
Less mush.
More intent.
