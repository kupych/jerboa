<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost, apiPatch, apiPut, apiDelete } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, setTypeCode } from "../lib/utils/format";
  import WaveformPlayer from "../lib/components/WaveformPlayer.svelte";

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
    await loadData();
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
</script>

{#if loading}
  <div class="flex items-center justify-center gap-2 py-20 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
{:else if error}
  <div class="text-center py-20 label text-danger">{error}</div>
{:else if set}
  <div>
    <!-- Breadcrumb -->
    <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5 mb-8">
      <button onclick={() => navigate(`/band/${slug}`)} class="hover:text-accent/60 transition-colors py-1">{slug.toUpperCase()}</button>
      <span>/</span>
      <span class="text-text-muted/50">SET:{set.name.replace(/\s+/g, "").toUpperCase()}</span>
    </div>

    <!-- Set header -->
    <div class="mb-8">
      {#if editingName}
        <div class="bg-bg-surface border border-border p-6 space-y-5">
          <div>
            <span class="label text-text-muted block mb-2">name</span>
            <input
              bind:value={editName}
              class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors"
            />
          </div>
          <div class="flex gap-4">
            <div class="flex-1">
              <span class="label text-text-muted block mb-2">type</span>
              <select
                bind:value={editType}
                class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors"
              >
                {#each setTypes as t}
                  <option value={t}>{t}</option>
                {/each}
              </select>
            </div>
            <div class="flex-1">
              <span class="label text-text-muted block mb-2">date</span>
              <input
                bind:value={editDate}
                type="date"
                class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors"
              />
            </div>
          </div>
          <div>
            <span class="label text-text-muted block mb-2">notes</span>
            <textarea
              bind:value={editNotes}
              rows="3"
              placeholder="Session notes, venue, etc..."
              class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y"
            ></textarea>
          </div>
          <div class="flex gap-4">
            <button
              onclick={saveMeta}
              disabled={savingMeta || !editName.trim()}
              class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
            >{savingMeta ? "..." : "save"}</button>
            <button
              onclick={() => (editingName = false)}
              class="px-6 py-3 label text-text-muted hover:text-text-secondary transition-colors"
            >cancel</button>
            <button
              onclick={deleteSet}
              disabled={deleting}
              class="ml-auto px-6 py-3 label text-danger hover:text-red-300 transition-colors"
            >{deleting ? "..." : "delete set"}</button>
          </div>
        </div>
      {:else}
        <div class="flex items-start justify-between">
          <div>
            <div class="flex items-center gap-3">
              <h2 class="text-2xl font-bold tracking-wider font-display">{set.name}</h2>
              <span class="label-sm text-accent bg-accent/10 px-2 py-0.5 font-mono">{setTypeCode(set.set_type)}</span>
            </div>
            <div class="flex items-center gap-3 mt-3 label-sm text-text-muted">
              {#if set.recorded_at}
                <span>{new Date(set.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
                <span class="vr-divider">/</span>
              {/if}
              <span>{set.items.length} {set.items.length === 1 ? 'song' : 'songs'}</span>
              {#if set.tracks.length > 0}
                <span class="vr-divider">/</span>
                <span>{set.tracks.length} {set.tracks.length === 1 ? 'recording' : 'recordings'}</span>
              {/if}
            </div>
            {#if set.notes}
              <p class="text-sm font-medium text-text-secondary mt-3">{set.notes}</p>
            {/if}
          </div>
          <div class="flex items-center gap-4">
          {#if set.items.length > 0}
            <button
              onclick={() => navigate(`/band/${slug}/set/${setId}/perform`)}
              class="px-4 py-2 bg-accent hover:bg-accent-hover text-bg-primary label-sm transition-colors"
            >perform</button>
          {/if}
          <button
            onclick={startEditMeta}
            class="label text-text-muted hover:text-accent transition-colors"
          >edit</button>
        </div>
        </div>
      {/if}
    </div>

    <!-- Setlist -->
    <div class="mb-8">
      <div class="flex items-center justify-between mb-4">
        <h3 class="label text-text-muted">setlist</h3>
        <div class="flex items-center gap-4">
          {#if tagging}
            <button
              onclick={() => { saveItems(); exitTaggingMode(); }}
              class="label text-accent hover:text-accent-hover transition-colors"
            >save &amp; done</button>
            <button
              onclick={exitTaggingMode}
              class="label text-text-muted hover:text-text-secondary transition-colors"
            >cancel</button>
          {:else}
            {#if primaryTrack && set.items.length > 0}
              <button
                onclick={enterTaggingMode}
                class="label text-text-muted hover:text-accent transition-colors"
              >tag timestamps</button>
            {/if}
            <button
              onclick={saveItems}
              disabled={savingItems}
              class="label transition-colors {savedFlash ? 'text-green-400' : 'text-text-muted hover:text-accent'}"
            >{savingItems ? "saving..." : savedFlash ? "saved" : "save"}</button>
          {/if}
        </div>
      </div>

      {#if set.items.length > 0}
        <div class="space-y-1">
          {#each set.items as item, index}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="bg-bg-surface border p-4 flex items-center gap-4 group transition-colors {tagging && taggingItemIndex === index ? 'border-accent bg-accent/5' : 'border-border'}"
              onclick={() => { if (tagging) selectTaggingItem(index); }}
            >
              <!-- Position & reorder -->
              <div class="flex flex-col gap-1 shrink-0">
                <button
                  onclick={(e) => { e.stopPropagation(); moveItem(index, index - 1); }}
                  disabled={index === 0}
                  class="label-sm text-text-muted hover:text-accent disabled:opacity-20 transition-colors"
                >&uarr;</button>
                <button
                  onclick={(e) => { e.stopPropagation(); moveItem(index, index + 1); }}
                  disabled={index === set.items.length - 1}
                  class="label-sm text-text-muted hover:text-accent disabled:opacity-20 transition-colors"
                >&darr;</button>
              </div>

              <!-- Number -->
              <span class="label-sm text-text-muted w-6 text-center shrink-0">{index + 1}</span>

              <!-- Name -->
              <button
                class="text-base font-semibold tracking-wider text-text-primary font-display flex-1 min-w-0 truncate text-left {item.start_ms != null ? 'hover:text-accent transition-colors' : ''}"
                onclick={(e) => {
                  e.stopPropagation();
                  if (item.start_ms != null && playerRef) playerRef.seekTo(item.start_ms);
                }}
              >
                {itemDisplayName(item)}
              </button>

              <!-- Tagging indicator -->
              {#if tagging && taggingItemIndex === index}
                <span class="label-sm text-accent shrink-0 animate-pulse">
                  set {taggingPhase}
                </span>
              {/if}

              <!-- Timestamps: manual entry -->
              <div class="flex items-center gap-1 shrink-0 label-sm text-text-muted font-mono">
                <input
                  type="text"
                  value={item.start_ms != null ? msToTimeStr(item.start_ms) : ""}
                  placeholder="start"
                  onclick={(e) => e.stopPropagation()}
                  onchange={(e) => { setItemTime(index, "start_ms", (e.target as HTMLInputElement).value); }}
                  class="w-14 bg-transparent border border-border px-1.5 py-0.5 text-center text-text-secondary placeholder:text-text-muted/40 focus:outline-none focus:border-accent transition-colors"
                />
                <span class="text-text-muted/40">&mdash;</span>
                <input
                  type="text"
                  value={item.end_ms != null ? msToTimeStr(item.end_ms) : ""}
                  placeholder="end"
                  onclick={(e) => e.stopPropagation()}
                  onchange={(e) => { setItemTime(index, "end_ms", (e.target as HTMLInputElement).value); }}
                  class="w-14 bg-transparent border border-border px-1.5 py-0.5 text-center text-text-secondary placeholder:text-text-muted/40 focus:outline-none focus:border-accent transition-colors"
                />
              </div>

              <!-- Delete -->
              <button
                onclick={(e) => { e.stopPropagation(); removeItem(index); }}
                class="label-sm text-text-muted hover:text-danger transition-colors opacity-0 group-hover:opacity-100 shrink-0"
              >x</button>
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-center py-10 label text-text-muted bg-bg-surface border border-border">
          no songs in setlist
        </div>
      {/if}

      <!-- Add song -->
      <div class="mt-3 relative">
        <input
          type="text"
          bind:value={addSongInput}
          onfocus={() => (addSongDropdown = true)}
          onblur={() => setTimeout(() => (addSongDropdown = false), 150)}
          placeholder="+ add song (type to search or enter cover name)"
          class="w-full bg-bg-surface border border-border px-4 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
        />
        {#if addSongDropdown && (addFilteredSongs.length > 0 || addSongInput.trim())}
          <div class="absolute top-full left-0 mt-1 w-full bg-bg-surface border border-border z-10 max-h-48 overflow-y-auto">
            {#each addFilteredSongs as song}
              <button
                type="button"
                onmousedown={() => addSong(song)}
                class="block w-full text-left px-4 py-2 label-sm text-text-secondary hover:bg-accent/10 hover:text-accent transition-colors"
              >{song.name}</button>
            {/each}
            {#if addSongInput.trim() && !songs.some((s) => s.name.toLowerCase() === addSongInput.trim().toLowerCase())}
              <button
                type="button"
                onmousedown={addCustom}
                class="block w-full text-left px-4 py-2 label-sm text-accent hover:bg-accent/10 transition-colors border-t border-border"
              >+ add "{addSongInput.trim()}" (cover/custom)</button>
            {/if}
          </div>
        {/if}
      </div>
    </div>

    <!-- Recordings -->
    <div class="mb-8">
      <h3 class="label text-text-muted mb-4">recordings</h3>

      {#if set.tracks.length > 0}
        <div class="space-y-3 mb-4">
          {#each set.tracks as track}
            <div class="bg-bg-surface border border-border p-4 flex items-center justify-between">
              <div class="flex items-center gap-3 min-w-0">
                <button
                  onclick={() => navigate(`/band/${slug}/track/${track.id}`)}
                  class="text-base font-semibold tracking-wider text-text-primary font-display hover:text-accent transition-colors truncate"
                >{track.title}</button>
                {#if track.status === "ready"}
                  <span class="label-sm text-text-muted font-mono">{formatDuration(track.duration_ms)}</span>
                {:else if track.status === "processing"}
                  <span class="label-sm text-accent">processing</span>
                {/if}
              </div>
              <button
                onclick={() => unassignTrack(track.id)}
                disabled={assigningTrack}
                class="label-sm text-text-muted hover:text-danger transition-colors"
              >remove</button>
            </div>
          {/each}
        </div>
      {/if}

      {#if assignableTracks.length > 0}
        <select
          onchange={(e) => {
            const val = (e.target as HTMLSelectElement).value;
            if (val) { assignTrackToSet(val); (e.target as HTMLSelectElement).value = ""; }
          }}
          disabled={assigningTrack}
          class="bg-bg-surface border border-border px-4 py-3 text-base text-text-secondary focus:outline-none focus:border-accent transition-colors w-full"
        >
          <option value="">+ assign recording...</option>
          {#each assignableTracks as track}
            <option value={track.id}>{track.title} ({formatDuration(track.duration_ms)})</option>
          {/each}
        </select>
      {:else if set.tracks.length === 0}
        <div class="label-sm text-text-muted py-4">no recordings available to assign</div>
      {/if}
    </div>

    <!-- Player with regions -->
    {#if primaryTrack && primaryTrack.status === "ready"}
      <div class="mb-10">
        <WaveformPlayer
          bind:this={playerRef}
          src={streamUrl}
          peaks={primaryTrack.waveform_data}
          duration={primaryTrack.duration_ms}
          regions={regions()}
          clickToTag={tagging}
          onTimestampClick={tagging ? handleTaggingClick : undefined}
          minPxPerMin={120}
        />
      </div>
    {/if}
  </div>
{/if}
