<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, apiPost, apiPatch, apiPut, apiDelete } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, setTypeCode, setTypeLabel } from "../lib/utils/format";
  import WaveformPlayer from "../lib/components/WaveformPlayer.svelte";
  import { layoutWidth } from "../lib/stores/layoutWidth";

  let { slug, setId }: { slug: string; setId: string } = $props();

  interface Song {
    id: string;
    name: string;
  }

  interface SetItem {
    id: string;
    set_id: string;
    position: number;
    song_id?: string;
    custom_name?: string;
    start_ms?: number;
    end_ms?: number;
    notes?: string;
    song_name?: string;
  }

  interface Track {
    id: string;
    title: string;
    description?: string;
    waveform_data?: number[];
    duration_ms: number;
    format: string;
    file_size: number;
    status: string;
    set_id?: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
  }

  interface SetDetail {
    id: string;
    band_id: string;
    name: string;
    set_type: string;
    recorded_at?: string;
    notes?: string;
    created_at: string;
    items: SetItem[];
    tracks: Track[];
  }

  let set = $state<SetDetail | null>(null);
  let songs = $state<Song[]>([]);
  let allTracks = $state<Track[]>([]);
  let loading = $state(true);
  let error = $state("");
  let playerRef: ReturnType<typeof WaveformPlayer> | undefined = $state();

  // Editing
  let editingName = $state(false);
  let editName = $state("");
  let editType = $state("");
  let editDate = $state("");
  let editNotes = $state("");
  let savingMeta = $state(false);
  let savingItems = $state(false);
  let savedFlash = $state(false);
  let deleting = $state(false);

  // Add item
  let addSongInput = $state("");
  let addSongDropdown = $state(false);
  let addFilteredSongs = $derived(
    addSongInput.trim()
      ? songs.filter((s) => s.name.toLowerCase().includes(addSongInput.trim().toLowerCase()))
      : songs
  );

  // Tagging mode
  let tagging = $state(false);
  let taggingItemIndex = $state<number | null>(null);
  let taggingPhase = $state<"start" | "end">("start");

  // Track assignment
  let assigningTrack = $state(false);

  let primaryTrack = $derived(set?.tracks?.[0] ?? null);
  let streamUrl = $derived(
    primaryTrack ? `/api/bands/${slug}/tracks/${primaryTrack.id}/stream` : ""
  );
  let setRuntimeMs = $derived(
    set
      ? Math.max(
          primaryTrack?.duration_ms ?? 0,
          ...set.items.map((item) => item.end_ms ?? 0)
        )
      : 0
  );
  let setRuntimeLabel = $derived(setRuntimeMs > 0 ? formatDuration(setRuntimeMs) : "--:--");

  let regions = $derived(() => {
    if (!set?.items) return [];
    return set.items
      .filter((item) => item.start_ms != null && item.end_ms != null)
      .map((item, i) => ({
        start_ms: item.start_ms!,
        end_ms: item.end_ms!,
        label: item.song_name || item.custom_name || `#${item.position + 1}`,
        color: regionColors[i % regionColors.length],
      }));
  });

  const regionColors = [
    "rgba(99,102,241,0.25)",
    "rgba(236,72,153,0.25)",
    "rgba(34,197,94,0.25)",
    "rgba(234,179,8,0.25)",
    "rgba(168,85,247,0.25)",
    "rgba(249,115,22,0.25)",
  ];

  const setTypes = ["live", "rehearsal", "pre-production", "other"];

  onMount(async () => {
    layoutWidth.set('workspace');
    await loadData();
  });

  onDestroy(() => {
    layoutWidth.set('index');
  });

  async function loadData() {
    loading = true;
    error = "";
    try {
      [set, songs, allTracks] = await Promise.all([
        api<SetDetail>(`/api/bands/${slug}/sets/${setId}`),
        api<Song[]>(`/api/bands/${slug}/songs`),
        api<Track[]>(`/api/bands/${slug}/tracks`),
      ]);
    } catch (e: any) {
      error = e?.message || "failed to load set";
    } finally {
      loading = false;
    }
  }

  function startEditMeta() {
    if (!set) return;
    editName = set.name;
    editType = set.set_type;
    editDate = set.recorded_at ? set.recorded_at.slice(0, 10) : "";
    editNotes = set.notes || "";
    editingName = true;
  }

  async function saveMeta() {
    if (!set || !editName.trim()) return;
    savingMeta = true;
    try {
      const updated = await apiPatch<SetDetail>(`/api/bands/${slug}/sets/${setId}`, {
        name: editName.trim(),
        set_type: editType,
        recorded_at: editDate || "",
        notes: editNotes,
      });
      set.name = updated.name;
      set.set_type = updated.set_type;
      set.recorded_at = updated.recorded_at;
      set.notes = updated.notes;
      editingName = false;
    } finally {
      savingMeta = false;
    }
  }

  async function deleteSet() {
    if (!set || !confirm("Delete this set?")) return;
    deleting = true;
    try {
      await apiDelete(`/api/bands/${slug}/sets/${setId}`);
      navigate(`/band/${slug}`);
    } finally {
      deleting = false;
    }
  }

  async function saveItems() {
    if (!set) return;
    savingItems = true;
    try {
      await apiPut<SetItem[]>(`/api/bands/${slug}/sets/${setId}/items`, {
        items: set.items.map((item) => ({
          song_id: item.song_id || null,
          custom_name: item.custom_name || "",
          start_ms: item.start_ms ?? null,
          end_ms: item.end_ms ?? null,
          notes: item.notes || "",
        })),
      });
      // Re-fetch to get song_names back
      const updated = await api<SetDetail>(`/api/bands/${slug}/sets/${setId}`);
      set.items = updated.items;
      savedFlash = true;
      setTimeout(() => (savedFlash = false), 1500);
    } finally {
      savingItems = false;
    }
  }

  function addSong(song: Song) {
    if (!set) return;
    set.items = [
      ...set.items,
      {
        id: "",
        set_id: setId,
        position: set.items.length,
        song_id: song.id,
        custom_name: "",
        song_name: song.name,
        notes: "",
      },
    ];
    addSongInput = "";
    addSongDropdown = false;
    saveItems();
  }

  function addCustom() {
    if (!set || !addSongInput.trim()) return;
    set.items = [
      ...set.items,
      {
        id: "",
        set_id: setId,
        position: set.items.length,
        custom_name: addSongInput.trim(),
        song_name: "",
        notes: "",
      },
    ];
    addSongInput = "";
    addSongDropdown = false;
    saveItems();
  }

  function removeItem(index: number) {
    if (!set) return;
    set.items = set.items.filter((_, i) => i !== index);
    saveItems();
  }

  function moveItem(from: number, to: number) {
    if (!set || to < 0 || to >= set.items.length) return;
    const items = [...set.items];
    const [moved] = items.splice(from, 1);
    items.splice(to, 0, moved);
    set.items = items;
    saveItems();
  }

  async function assignTrackToSet(trackId: string) {
    if (!set) return;
    assigningTrack = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/set`, { set_id: setId });
      const updated = await api<SetDetail>(`/api/bands/${slug}/sets/${setId}`);
      set.tracks = updated.tracks;
      allTracks = await api<Track[]>(`/api/bands/${slug}/tracks`);
    } finally {
      assigningTrack = false;
    }
  }

  async function unassignTrack(trackId: string) {
    if (!set) return;
    assigningTrack = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/set`, { set_id: null });
      const updated = await api<SetDetail>(`/api/bands/${slug}/sets/${setId}`);
      set.tracks = updated.tracks;
      allTracks = await api<Track[]>(`/api/bands/${slug}/tracks`);
    } finally {
      assigningTrack = false;
    }
  }

  function enterTaggingMode() {
    tagging = true;
    taggingItemIndex = 0;
    taggingPhase = "start";
  }

  function exitTaggingMode() {
    tagging = false;
    taggingItemIndex = null;
  }

  function handleTaggingClick(ms: number) {
    if (!tagging || taggingItemIndex == null || !set) return;
    const item = set.items[taggingItemIndex];
    if (!item) return;

    if (taggingPhase === "start") {
      item.start_ms = ms;
      taggingPhase = "end";
      set.items = [...set.items];
    } else {
      item.end_ms = ms;
      set.items = [...set.items];
      // Advance to next item
      if (taggingItemIndex < set.items.length - 1) {
        taggingItemIndex = taggingItemIndex + 1;
        taggingPhase = "start";
      } else {
        // All tagged, save
        saveItems();
        exitTaggingMode();
      }
    }
  }

  function selectTaggingItem(index: number) {
    taggingItemIndex = index;
    taggingPhase = "start";
  }

  function itemDisplayName(item: SetItem): string {
    return item.song_name || item.custom_name || `Item #${item.position + 1}`;
  }

  function parseTimeToMs(value: string): number | null {
    // Accept "1:23" or "83" (seconds) or "1:23.45"
    const parts = value.trim().split(":");
    if (parts.length === 2) {
      const mins = parseInt(parts[0], 10);
      const secs = parseFloat(parts[1]);
      if (!isNaN(mins) && !isNaN(secs)) return Math.round((mins * 60 + secs) * 1000);
    } else if (parts.length === 1) {
      const secs = parseFloat(parts[0]);
      if (!isNaN(secs)) return Math.round(secs * 1000);
    }
    return null;
  }

  function msToTimeStr(ms: number): string {
    const totalSec = ms / 1000;
    const mins = Math.floor(totalSec / 60);
    const secs = (totalSec % 60).toFixed(0).padStart(2, "0");
    return `${mins}:${secs}`;
  }

  function setItemTime(index: number, field: "start_ms" | "end_ms", value: string) {
    if (!set) return;
    const ms = parseTimeToMs(value);
    if (ms == null) return;
    set.items[index][field] = ms;
    set.items = [...set.items];
  }

  let availableTracks = $derived(
    allTracks.filter((t) => t.status === "ready" && (!t.set_id || t.set_id === setId))
  );
  let assignableTracks = $derived(
    allTracks.filter((t) => t.status === "ready" && !t.set_id)
  );

  // Mobile: which rows have timestamp inputs expanded
  let expandedRows = $state(new Set<number>());

  function toggleRowExpand(index: number) {
    const next = new Set(expandedRows);
    next.has(index) ? next.delete(index) : next.add(index);
    expandedRows = next;
  }
</script>

{#if loading}
  <div class="flex items-center justify-center gap-2 py-20 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
{:else if error}
  <div class="text-center py-20 label text-danger">{error}</div>
{:else if set}
  <div>
    <!-- Breadcrumb -->
    <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5 mb-4">
      <button onclick={() => navigate(`/band/${slug}`)} class="hover:text-accent/60 transition-colors py-1">{slug.toUpperCase()}</button>
      <span class="text-text-muted/20">&rsaquo;</span>
      <span class="text-text-muted/50">{set.name.toUpperCase()}</span>
    </div>

    <!-- Set header -->
    <div class="mb-6">
      {#if editingName}
        <div class="bg-bg-surface border border-border p-6 space-y-5">
          <div>
            <span class="label text-text-muted block mb-2">name</span>
            <input bind:value={editName} class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors" />
          </div>
          <div class="flex gap-4">
            <div class="flex-1">
              <span class="label text-text-muted block mb-2">type</span>
              <select bind:value={editType} class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors">
                {#each setTypes as t}<option value={t}>{t}</option>{/each}
              </select>
            </div>
            <div class="flex-1">
              <span class="label text-text-muted block mb-2">date</span>
              <input bind:value={editDate} type="date" class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors" />
            </div>
          </div>
          <div>
            <span class="label text-text-muted block mb-2">notes</span>
            <textarea bind:value={editNotes} rows="3" placeholder="Session notes, venue, etc..." class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y"></textarea>
          </div>
          <div class="flex gap-4">
            <button onclick={saveMeta} disabled={savingMeta || !editName.trim()} class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors">{savingMeta ? "..." : "save"}</button>
            <button onclick={() => (editingName = false)} class="px-6 py-3 label text-text-muted hover:text-text-secondary transition-colors">cancel</button>
            <button onclick={deleteSet} disabled={deleting} class="ml-auto px-6 py-3 label text-danger hover:text-red-300 transition-colors">{deleting ? "..." : "delete set"}</button>
          </div>
        </div>
      {:else}
        <!-- Title row -->
        <div class="flex items-start gap-4 mb-4">
          <div class="flex-1 min-w-0">
            <div class="sys-kicker mb-3">set planner // running order + timestamps</div>
            <div class="flex items-center gap-3 mb-1 flex-wrap">
              <h1 class="text-4xl md:text-[3.55rem] leading-none font-bold tracking-[0.05em] font-display uppercase">{set.name}</h1>
              <span class="label-sm text-accent bg-accent/10 px-2 py-0.5 font-mono" title={setTypeLabel(set.set_type)}>{setTypeCode(set.set_type)}</span>
            </div>
            <div class="flex items-center gap-2 label-sm text-text-muted flex-wrap">
              {#if set.recorded_at}
                <span>{new Date(set.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
                <span class="text-text-muted/30">/</span>
              {/if}
              <span>{set.items.length} {set.items.length === 1 ? 'song' : 'songs'}</span>
              {#if set.tracks.length > 0}
                <span class="text-text-muted/30">/</span>
                <span>{set.tracks.length} {set.tracks.length === 1 ? 'recording' : 'recordings'}</span>
              {/if}
            </div>
            {#if set.notes}
              <p class="text-sm font-medium text-text-secondary mt-2">{set.notes}</p>
            {/if}
            <div class="sys-stat-grid mt-4 grid-cols-1 sm:grid-cols-2">
              <div class="sys-stat">
                <div class="sys-code text-text-muted/45">SLT // songs</div>
                <div class="sys-stat-value text-accent mt-2">{String(set.items.length).padStart(2, "0")}</div>
              </div>
              <div class="sys-stat">
                <div class="sys-code text-text-muted/45">RUN // runtime</div>
                <div class="sys-stat-value mt-2">{setRuntimeLabel}</div>
              </div>
            </div>
          </div>
          <button onclick={startEditMeta} class="label text-text-muted hover:text-accent transition-colors shrink-0 mt-1">edit</button>
        </div>

        <!-- Primary action group: save · tag timestamps · perform -->
        <div class="flex items-center gap-2 flex-wrap">
          {#if tagging}
            <button
              onclick={() => { saveItems(); exitTaggingMode(); }}
              class="px-4 py-2 bg-accent hover:bg-accent-hover text-bg-primary label-sm transition-colors"
            >save &amp; done</button>
            <button
              onclick={exitTaggingMode}
              class="px-4 py-2 border border-border label-sm text-text-muted hover:text-text-secondary transition-colors"
            >cancel tagging</button>
          {:else}
            {#if primaryTrack && set.items.length > 0}
              <button
                onclick={enterTaggingMode}
                class="px-4 py-2 border border-border hover:border-accent/60 label-sm text-text-muted hover:text-accent transition-colors"
              >tag timestamps</button>
            {/if}
            <button
              onclick={saveItems}
              disabled={savingItems}
              class="px-4 py-2 border label-sm transition-colors {savedFlash ? 'border-success/40 text-success' : 'border-border text-text-muted hover:border-accent/60 hover:text-accent'}"
            >{savingItems ? "saving..." : savedFlash ? "saved" : "save"}</button>
            {#if set.items.length > 0}
              <button
                onclick={() => navigate(`/band/${slug}/set/${setId}/perform`)}
                class="px-4 py-2 bg-accent hover:bg-accent-hover text-bg-primary label-sm transition-colors"
              >perform</button>
            {/if}
          {/if}
        </div>
      {/if}
    </div>

    <!-- Tagging mode banner — unmissable -->
    {#if tagging}
      <div class="sys-mode-bar mb-4">
        <div class="min-w-0">
          <div class="flex items-center gap-3 flex-wrap">
            <span class="sys-code text-accent">TGR</span>
            <span class="label-sm text-accent">interactive state</span>
          </div>
          <div class="sys-stat-value text-accent mt-2">Tagging Mode</div>
          {#if taggingItemIndex != null && set.items[taggingItemIndex]}
            <p class="label-sm text-text-secondary mt-2 truncate">
              {itemDisplayName(set.items[taggingItemIndex])}
              <span class="text-text-muted/60 ml-1">/ click waveform to set {taggingPhase}</span>
            </p>
          {/if}
        </div>
        <div class="shrink-0 text-right">
          <div class="sys-code text-text-muted/50">{String((taggingItemIndex ?? 0) + 1).padStart(2, "0")} / {String(set.items.length).padStart(2, "0")}</div>
          <div class="label-sm text-accent mt-2">{taggingPhase} point</div>
          <button onclick={exitTaggingMode} class="label-sm text-text-muted hover:text-text-primary transition-colors mt-3">exit mode</button>
        </div>
      </div>
    {/if}

    <!-- Desktop planner: sticky setlist left / recordings + workspace right -->
    <div class="md:grid md:grid-cols-[5fr_7fr] md:gap-8 md:items-start">

      <!-- Left column: sticky setlist -->
      <div class="min-w-0 md:sticky md:top-4 md:max-h-[calc(100svh-8rem)] md:overflow-y-auto">
        <div class="sys-section-head mb-3">
          <h3 class="sys-kicker">SLT // setlist</h3>
          {#if set.items.length > 0}
            <span class="sys-code text-text-muted/50">{set.items.length} songs</span>
          {/if}
        </div>

        {#if set.items.length > 0}
          <div class="space-y-px">
            {#each set.items as item, index}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div
                class="sys-panel px-3 py-2.5 flex items-center gap-3 group transition-all {tagging && taggingItemIndex === index ? 'border-accent bg-accent/5' : 'hover:border-border/80'}"
                onclick={() => { if (tagging) selectTaggingItem(index); }}
              >
                <!-- Reorder -->
                <div class="flex flex-col shrink-0 gap-0.5">
                  <button onclick={(e) => { e.stopPropagation(); moveItem(index, index - 1); }} disabled={index === 0} class="text-text-muted hover:text-accent disabled:opacity-20 transition-colors">
                    <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="18 15 12 9 6 15"/></svg>
                  </button>
                  <button onclick={(e) => { e.stopPropagation(); moveItem(index, index + 1); }} disabled={index === set.items.length - 1} class="text-text-muted hover:text-accent disabled:opacity-20 transition-colors">
                    <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="6 9 12 15 18 9"/></svg>
                  </button>
                </div>

                <!-- Number -->
                <span class="label-sm text-text-muted/50 w-5 text-center shrink-0 tabular-nums">{index + 1}</span>

                <!-- Name -->
                <button
                  class="text-sm font-semibold tracking-wide text-text-primary font-display flex-1 min-w-0 truncate text-left {item.start_ms != null ? 'hover:text-accent transition-colors' : ''}"
                  onclick={(e) => { e.stopPropagation(); if (item.start_ms != null && playerRef) playerRef.seekTo(item.start_ms); }}
                >{itemDisplayName(item)}</button>

                <!-- Tagging active indicator -->
                {#if tagging && taggingItemIndex === index}
                  <span class="label-sm text-bg-primary bg-accent/80 px-1.5 py-0.5 shrink-0 animate-pulse">
                    {taggingPhase}
                  </span>
                {/if}

                <!-- Timestamps: inputs on desktop, compact badge on mobile -->
                <!-- Mobile: show compact "1:23" badge if tagged, or nothing -->
                {#if item.start_ms != null && !expandedRows.has(index)}
                  <button
                    class="md:hidden label-sm text-text-muted/60 font-mono shrink-0 hover:text-accent transition-colors"
                    onclick={(e) => { e.stopPropagation(); toggleRowExpand(index); }}
                    title="expand timestamps"
                  >{msToTimeStr(item.start_ms)}</button>
                {/if}

                <!-- Inputs: always on desktop, shown when expanded on mobile -->
                <div class="{expandedRows.has(index) ? 'flex' : 'hidden'} md:flex items-center gap-1 shrink-0 label-sm text-text-muted font-mono">
                  <input
                    type="text"
                    value={item.start_ms != null ? msToTimeStr(item.start_ms) : ""}
                    placeholder="0:00"
                    onclick={(e) => e.stopPropagation()}
                    onchange={(e) => { setItemTime(index, "start_ms", (e.target as HTMLInputElement).value); }}
                    class="w-12 bg-transparent border border-border px-1 py-0.5 text-center text-text-secondary placeholder:text-text-muted/30 focus:outline-none focus:border-accent transition-colors"
                  />
                  <span class="text-text-muted/30">&ndash;</span>
                  <input
                    type="text"
                    value={item.end_ms != null ? msToTimeStr(item.end_ms) : ""}
                    placeholder="0:00"
                    onclick={(e) => e.stopPropagation()}
                    onchange={(e) => { setItemTime(index, "end_ms", (e.target as HTMLInputElement).value); }}
                    class="w-12 bg-transparent border border-border px-1 py-0.5 text-center text-text-secondary placeholder:text-text-muted/30 focus:outline-none focus:border-accent transition-colors"
                  />
                </div>

                <!-- Delete -->
                <button
                  onclick={(e) => { e.stopPropagation(); removeItem(index); }}
                  class="label-sm text-text-muted hover:text-danger transition-colors opacity-0 group-hover:opacity-100 shrink-0 ml-1"
                >×</button>
              </div>
            {/each}
          </div>
        {:else}
          <div class="sys-empty">
            <div class="sys-empty-code">SLT // 00</div>
            <div class="sys-empty-title text-text-primary mt-3">No Setlist</div>
            <p class="sys-empty-copy mt-3">Add songs or covers here first. This column is the running order that everything else keys off.</p>
          </div>
        {/if}

        <!-- Add song -->
        <div class="mt-2 relative">
          <input
            type="text"
            bind:value={addSongInput}
            onfocus={() => (addSongDropdown = true)}
            onblur={() => setTimeout(() => (addSongDropdown = false), 150)}
            placeholder="+ add song or cover"
            class="w-full bg-bg-surface border border-border px-4 py-2.5 text-sm text-text-primary placeholder:text-text-muted/60 focus:outline-none focus:border-accent transition-colors"
          />
          {#if addSongDropdown && (addFilteredSongs.length > 0 || addSongInput.trim())}
            <div class="absolute top-full left-0 mt-1 w-full bg-bg-elevated border border-border z-10 max-h-48 overflow-y-auto animate-dropdown">
              {#each addFilteredSongs as song}
                <button type="button" onmousedown={() => addSong(song)} class="block w-full text-left px-4 py-2 label-sm text-text-secondary hover:bg-accent/10 hover:text-accent transition-colors">{song.name}</button>
              {/each}
              {#if addSongInput.trim() && !songs.some((s) => s.name.toLowerCase() === addSongInput.trim().toLowerCase())}
                <button type="button" onmousedown={addCustom} class="block w-full text-left px-4 py-2 label-sm text-accent hover:bg-accent/10 transition-colors border-t border-border">+ "{addSongInput.trim()}" (cover/custom)</button>
              {/if}
            </div>
          {/if}
        </div>
      </div><!-- /left column -->

      <!-- Right column: recordings + waveform workspace -->
      <div class="min-w-0 mt-8 md:mt-0 space-y-6">

        <!-- Recordings -->
        <div>
          <div class="sys-section-head mb-3">
            <h3 class="sys-kicker">REC // recordings</h3>
            <span class="sys-code text-text-muted/50">{set.tracks.length} assigned</span>
          </div>
          {#if set.tracks.length > 0}
            <div class="space-y-1 mb-3">
              {#each set.tracks as track}
                <div class="bg-bg-surface border border-border px-4 py-3 flex items-center justify-between gap-4">
                  <div class="flex items-center gap-3 min-w-0">
                    <button onclick={() => navigate(`/band/${slug}/track/${track.id}`)} class="text-sm font-semibold tracking-wide text-text-primary font-display hover:text-accent transition-colors truncate">{track.title}</button>
                    {#if track.status === "ready"}
                      <span class="label-sm text-text-muted font-mono shrink-0">{formatDuration(track.duration_ms)}</span>
                    {:else if track.status === "processing"}
                      <span class="label-sm text-accent shrink-0">processing</span>
                    {/if}
                  </div>
                  <button onclick={() => unassignTrack(track.id)} disabled={assigningTrack} class="label-sm text-text-muted hover:text-danger transition-colors shrink-0">remove</button>
                </div>
              {/each}
            </div>
          {/if}
          {#if assignableTracks.length > 0}
            <select
              onchange={(e) => { const val = (e.target as HTMLSelectElement).value; if (val) { assignTrackToSet(val); (e.target as HTMLSelectElement).value = ""; } }}
              disabled={assigningTrack}
              class="bg-bg-surface border border-border px-4 py-2.5 text-sm text-text-secondary focus:outline-none focus:border-accent transition-colors w-full"
            >
              <option value="">+ assign recording...</option>
              {#each assignableTracks as track}
                <option value={track.id}>{track.title} ({formatDuration(track.duration_ms)})</option>
              {/each}
            </select>
          {:else if set.tracks.length === 0}
            <div class="sys-empty">
              <div class="sys-empty-code">REC // 00</div>
              <div class="sys-empty-title text-text-primary mt-3">No Recordings</div>
              <p class="sys-empty-copy mt-3">Assign one take to this set so the timestamp workspace has a source track to tag against.</p>
            </div>
          {/if}
        </div>

        <!-- Waveform workspace -->
        {#if primaryTrack && primaryTrack.status === "ready"}
          <div class="sys-section-head border-b-0 mt-4">
            <span class="sys-kicker">TIM // timestamp workspace</span>
            <span class="sys-code text-text-muted/45 truncate">{primaryTrack.title}</span>
          </div>
          <div class="border border-border bg-bg-surface {tagging ? 'ring-2 ring-accent ring-offset-2 ring-offset-bg-primary' : ''}">
            <div class="overflow-x-auto">
              <WaveformPlayer
                bind:this={playerRef}
                src={streamUrl}
                peaks={primaryTrack.waveform_data}
                duration={primaryTrack.duration_ms}
                regions={regions()}
                clickToTag={tagging}
                onTimestampClick={tagging ? handleTaggingClick : undefined}
                minPxPerMin={120}
                height={300}
              />
            </div>
          </div>
        {:else}
          <div class="sys-empty">
            <div class="sys-empty-code">TIM // ---</div>
            <div class="sys-empty-title text-text-primary mt-3">Workspace Idle</div>
            <p class="sys-empty-copy mt-3">Assign a ready recording to unlock the timestamp canvas. This area becomes the main tagging surface once audio is attached.</p>
          </div>
        {/if}

      </div><!-- /right column -->
    </div><!-- /planner grid -->
  </div>
{/if}
