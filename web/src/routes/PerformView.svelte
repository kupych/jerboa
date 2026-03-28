<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, apiPost, apiPatch, apiPut } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import ChordProLyrics from "../lib/components/ChordProLyrics.svelte";
  import PerformRecorder from "../lib/components/PerformRecorder.svelte";
  import type { SongMarker } from "../lib/components/PerformRecorder.svelte";

  let { slug, setId = undefined }: { slug: string; setId?: string } = $props();

  interface PerformItem {
    position: number;
    song_name: string;
    custom_name: string;
    lyrics: string;
    tabs: string;
    notes?: string;
  }

  interface PerformData {
    set_name: string;
    set_type: string;
    items: PerformItem[];
  }

  interface Song {
    id: string;
    name: string;
    lyrics: string;
    tabs: string;
  }

  // Set mode
  let data = $state<PerformData | null>(null);

  // Freestyle mode
  let freestyle = $derived(!setId);
  let allSongs = $state<Song[]>([]);
  let playedItems = $state<PerformItem[]>([]);
  let showPicker = $state(false);
  let pickerSearch = $state("");
  let saving = $state(false);

  let filteredSongs = $derived(
    pickerSearch.trim()
      ? allSongs.filter((s) => s.name.toLowerCase().includes(pickerSearch.trim().toLowerCase()))
      : allSongs
  );

  let loading = $state(true);
  let currentIndex = $state(0);
  let showChords = $state(true);
  let showList = $state(false);

  // Recording
  let recorder = $state<PerformRecorder | undefined>(undefined);
  let isRec = $state(false);
  let recordedTrackId: string | null = null;
  let recordedMarkers: SongMarker[] = [];
  let recordedDurationMs = 0;
  let savingRecording = $state(false);

  let items = $derived(freestyle ? playedItems : (data?.items ?? []));
  let currentItem = $derived(items[currentIndex] ?? null);
  let itemName = $derived(
    currentItem ? (currentItem.song_name || currentItem.custom_name || `Song ${currentItem.position + 1}`) : ""
  );
  let totalItems = $derived(items.length);
  let title = $derived(freestyle ? "freestyle" : (data?.set_name ?? ""));

  // Touch handling
  let touchStartX = 0;
  let touchStartY = 0;

  // Wake lock
  let wakeLock: WakeLockSentinel | null = null;

  onMount(async () => {
    try {
      if (setId) {
        data = await api<PerformData>(`/api/bands/${slug}/sets/${setId}/perform`);
      } else {
        allSongs = await api<Song[]>(`/api/bands/${slug}/songs`);
        showPicker = true;
      }
    } finally {
      loading = false;
    }

    try {
      if ("wakeLock" in navigator) {
        wakeLock = await navigator.wakeLock.request("screen");
      }
    } catch {}

    window.addEventListener("keydown", handleKey);
  });

  onDestroy(() => {
    wakeLock?.release();
    window.removeEventListener("keydown", handleKey);
  });

  function handleKey(e: KeyboardEvent) {
    if (showPicker) return;
    if (e.key === "ArrowRight" || e.key === "ArrowDown" || e.key === " ") {
      e.preventDefault();
      next();
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      prev();
    } else if (e.key === "Escape") {
      exit();
    } else if (e.key === "c") {
      showChords = !showChords;
    } else if (e.key === "r") {
      toggleRecording();
    }
  }

  // Helper: display name of item at a given index (for recording markers)
  function itemNameAt(index: number): string {
    const item = items[index];
    if (!item) return "";
    return item.song_name || item.custom_name || `Song ${item.position + 1}`;
  }

  // Helper: song_id of item at index (only available in freestyle where allSongs is loaded)
  function itemSongId(index: number): string | undefined {
    const item = items[index];
    if (!item || !item.song_name) return undefined;
    return allSongs.find((s) => s.name === item.song_name)?.id;
  }

  function next() {
    if (currentIndex < totalItems - 1) {
      currentIndex++;
      recorder?.markSong(currentIndex, itemNameAt(currentIndex), itemSongId(currentIndex));
      scrollTop();
    } else if (freestyle) {
      openPicker();
    }
  }

  function prev() {
    if (currentIndex > 0) {
      currentIndex--;
      recorder?.markSong(currentIndex, itemNameAt(currentIndex), itemSongId(currentIndex));
      scrollTop();
    }
  }

  function goTo(index: number) {
    currentIndex = index;
    recorder?.markSong(currentIndex, itemNameAt(currentIndex), itemSongId(currentIndex));
    showList = false;
    scrollTop();
  }

  function scrollTop() {
    document.getElementById("perform-lyrics")?.scrollTo(0, 0);
  }

  async function exit() {
    if (recorder?.isRecording()) {
      recorder.stopRecording();
      // Give the upload a moment to start before navigating
      await new Promise((r) => setTimeout(r, 300));
    }
    if (setId) {
      navigate(`/band/${slug}/set/${setId}`);
    } else {
      navigate(`/band/${slug}`);
    }
  }

  function openPicker() {
    pickerSearch = "";
    showPicker = true;
  }

  function pickSong(song: Song) {
    playedItems = [
      ...playedItems,
      {
        position: playedItems.length,
        song_name: song.name,
        custom_name: "",
        lyrics: song.lyrics,
        tabs: song.tabs,
      },
    ];
    currentIndex = playedItems.length - 1;
    showPicker = false;
    recorder?.markSong(currentIndex, song.name, song.id);
    scrollTop();
  }

  // Build timestamp map from markers: position -> { startMs, endMs }
  // First occurrence of each position wins (handles revisited songs)
  function markersToTimestamps(markers: SongMarker[], durationMs: number): Map<number, { startMs: number; endMs: number }> {
    const seen = new Map<number, { startMs: number; endMs: number }>();
    for (let i = 0; i < markers.length; i++) {
      const m = markers[i];
      if (!seen.has(m.index)) {
        const endMs = i + 1 < markers.length ? markers[i + 1].startMs : durationMs;
        seen.set(m.index, { startMs: m.startMs, endMs });
      }
    }
    return seen;
  }

  function handleRecordingStopped({ trackId, markers: m, durationMs }: { trackId: string; markers: SongMarker[]; durationMs: number }) {
    recordedTrackId = trackId;
    recordedMarkers = m;
    recordedDurationMs = durationMs;
    isRec = false;

    // Set mode: immediately assign track + timestamps to existing set
    if (setId) {
      saveRecordingToSet(setId, trackId, m, durationMs);
    }
  }

  async function saveRecordingToSet(sid: string, trackId: string, markers: SongMarker[], durationMs: number) {
    savingRecording = true;
    try {
      // 1. Assign track to set
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/set`, { set_id: sid });

      // 2. Fetch current set items (need song_ids and positions)
      interface SetDetail { items: Array<{ position: number; song_id?: string; custom_name: string; notes: string; start_ms?: number; end_ms?: number }> }
      const setDetail = await api<SetDetail>(`/api/bands/${slug}/sets/${sid}`);
      const timestamps = markersToTimestamps(markers, durationMs);

      // 3. Merge timestamps into items
      const updatedItems = setDetail.items.map((item) => {
        const ts = timestamps.get(item.position);
        return {
          song_id: item.song_id || null,
          custom_name: item.custom_name || "",
          notes: item.notes || "",
          start_ms: ts?.startMs ?? item.start_ms ?? null,
          end_ms: ts?.endMs ?? item.end_ms ?? null,
        };
      });

      await apiPut(`/api/bands/${slug}/sets/${sid}/items`, { items: updatedItems });
    } catch {
      // Non-fatal: track is uploaded, just couldn't write timestamps
    } finally {
      savingRecording = false;
    }
  }

  async function toggleRecording() {
    if (!recorder) return;
    if (recorder.isRecording()) {
      recorder.stopRecording();
    } else {
      isRec = true;
      await recorder.startRecording();
      // Mark the currently visible song as the first marker
      if (currentItem) {
        recorder.markSong(currentIndex, itemNameAt(currentIndex), itemSongId(currentIndex));
      }
    }
  }

  async function saveAsSet() {
    if (playedItems.length === 0 || saving) return;
    saving = true;
    try {
      const now = new Date();
      const dateStr = `${now.getFullYear()}-${(now.getMonth() + 1).toString().padStart(2, "0")}-${now.getDate().toString().padStart(2, "0")}`;
      const set = await apiPost<{ id: string }>(`/api/bands/${slug}/sets`, {
        name: `Rehearsal ${dateStr}`,
        set_type: "rehearsal",
        items: playedItems.map((item, i) => {
          const song = allSongs.find((s) => s.name === item.song_name);
          return {
            song_id: song?.id || null,
            custom_name: song ? "" : item.song_name,
            position: i,
          };
        }),
      });

      // If a recording exists, assign it and write timestamps
      if (recordedTrackId) {
        const timestamps = markersToTimestamps(recordedMarkers, recordedDurationMs);
        await apiPatch(`/api/bands/${slug}/tracks/${recordedTrackId}/set`, { set_id: set.id });
        const itemsWithTs = playedItems.map((item, i) => {
          const song = allSongs.find((s) => s.name === item.song_name);
          const ts = timestamps.get(i);
          return {
            song_id: song?.id || null,
            custom_name: song ? "" : item.song_name,
            notes: "",
            start_ms: ts?.startMs ?? null,
            end_ms: ts?.endMs ?? null,
          };
        });
        await apiPut(`/api/bands/${slug}/sets/${set.id}/items`, { items: itemsWithTs });
      }

      navigate(`/band/${slug}/set/${set.id}`);
    } finally {
      saving = false;
    }
  }

  function handleTouchStart(e: TouchEvent) {
    touchStartX = e.touches[0].clientX;
    touchStartY = e.touches[0].clientY;
  }

  function handleTouchEnd(e: TouchEvent) {
    const dx = e.changedTouches[0].clientX - touchStartX;
    const dy = e.changedTouches[0].clientY - touchStartY;
    const absDx = Math.abs(dx);
    const absDy = Math.abs(dy);

    if (absDx > 60 && absDx > absDy * 1.5) {
      if (dx < 0) next();
      else prev();
    }
  }
</script>

{#if loading}
  <div class="fixed inset-0 bg-bg-primary flex items-center justify-center z-50">
    <div class="flex items-center gap-2 label text-text-muted">
      <span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span>
      <span class="tracking-[0.2em] font-mono">SYS.LOAD</span>
    </div>
  </div>
{:else if !freestyle && (!data || data.items.length === 0)}
  <div class="fixed inset-0 bg-bg-primary flex flex-col items-center justify-center gap-4 z-50">
    <div class="label text-text-muted">no songs in setlist</div>
    <button onclick={exit} class="label text-accent hover:text-accent-hover transition-colors">back to set</button>
  </div>
{:else}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 bg-bg-primary z-50 flex flex-col select-none"
    ontouchstart={handleTouchStart}
    ontouchend={handleTouchEnd}
  >
    <!-- Top bar -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
      <button onclick={exit} class="label-sm text-text-muted hover:text-accent transition-colors">exit</button>

      <button
        onclick={() => showList = !showList}
        class="text-center"
      >
        <div class="label text-text-primary">
          {title}
          {#if totalItems > 0}
            <span class="text-text-muted/40 font-mono ml-2">{currentIndex + 1}/{totalItems}</span>
          {/if}
        </div>
      </button>

      <div class="flex items-center gap-3">
        {#if freestyle && playedItems.length > 0}
          <button
            onclick={saveAsSet}
            disabled={saving}
            class="label-sm text-text-muted hover:text-accent transition-colors"
          >{saving ? "..." : "save"}</button>
        {/if}
        <button
          onclick={() => showChords = !showChords}
          class="label-sm transition-colors {showChords ? 'text-accent' : 'text-text-muted/40 hover:text-text-muted'}"
        >chords</button>
        <button
          onclick={toggleRecording}
          class="label-sm transition-colors flex items-center gap-1.5 {isRec ? 'text-red-400' : 'text-text-muted/40 hover:text-text-muted'}"
          title="{isRec ? 'stop recording' : 'start recording'} (r)"
        >
          <span class="w-1.5 h-1.5 rounded-full {isRec ? 'bg-red-400 animate-pulse' : 'bg-current'}"></span>
          rec
        </button>
        {#if savingRecording}
          <span class="label-sm text-text-muted/40 animate-pulse">saving...</span>
        {/if}
      </div>
    </div>

    <PerformRecorder
      bind:this={recorder}
      bandSlug={slug}
      onStopped={handleRecordingStopped}
    />

    {#if totalItems === 0 && !showPicker}
      <!-- Freestyle: no songs picked yet -->
      <div class="flex-1 flex flex-col items-center justify-center gap-4">
        <div class="label text-text-muted/30">pick a song to start</div>
        <button
          onclick={openPicker}
          class="px-6 py-3 bg-accent hover:bg-accent-hover text-bg-primary label transition-colors"
        >choose song</button>
      </div>
    {:else if currentItem}
      <!-- Song title -->
      <div class="px-6 pt-5 pb-4 shrink-0 border-b border-border/30">
        <h1 class="text-xl md:text-2xl font-display font-bold tracking-wider text-text-primary">{itemName}</h1>
        {#if currentItem.notes}
          <p class="text-xs font-semibold text-text-muted/50 mt-1">{currentItem.notes}</p>
        {/if}
      </div>

      <!-- Lyrics area -->
      <div id="perform-lyrics" class="flex-1 overflow-y-auto px-6 pt-4 pb-24 text-base md:text-lg">
        {#if currentItem.lyrics}
          <ChordProLyrics text={currentItem.lyrics} {showChords} />
        {:else}
          <div class="flex items-center justify-center h-full">
            <span class="label text-text-muted/30">no lyrics</span>
          </div>
        {/if}
      </div>

      <!-- Bottom nav -->
      <div class="absolute bottom-0 left-0 right-0 flex justify-between items-center px-6 py-4 bg-gradient-to-t from-bg-primary via-bg-primary/90 to-transparent pointer-events-none">
        <button
          onclick={prev}
          disabled={currentIndex === 0}
          class="pointer-events-auto label-sm text-text-muted hover:text-accent disabled:opacity-0 transition-all"
        >
          {#if currentIndex > 0}
            {items[currentIndex - 1].song_name || items[currentIndex - 1].custom_name}
          {/if}
        </button>

        {#if freestyle && currentIndex === totalItems - 1}
          <button
            onclick={openPicker}
            class="pointer-events-auto label text-accent hover:text-accent-hover transition-colors px-4 py-3 border border-accent/40 hover:border-accent min-h-[44px] min-w-[44px] flex items-center"
          >+ next song</button>
        {:else}
          <button
            onclick={next}
            disabled={currentIndex === totalItems - 1}
            class="pointer-events-auto label-sm text-text-muted hover:text-accent disabled:opacity-0 transition-all"
          >
            {#if currentIndex < totalItems - 1}
              {items[currentIndex + 1].song_name || items[currentIndex + 1].custom_name}
            {/if}
          </button>
        {/if}
      </div>
    {/if}

    <!-- Song picker overlay (freestyle) -->
    {#if showPicker}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="absolute inset-0 bg-bg-primary/95 z-10 flex flex-col">
        <div class="px-6 py-4 border-b border-border shrink-0 space-y-3">
          <div class="flex items-center justify-between">
            <h2 class="label text-text-muted">choose song</h2>
            {#if playedItems.length > 0}
              <button onclick={() => showPicker = false} class="label-sm text-text-muted hover:text-accent transition-colors">cancel</button>
            {/if}
          </div>
          <input
            type="text"
            bind:value={pickerSearch}
            placeholder="search..."
            autofocus
            class="w-full bg-transparent border border-border text-text-primary text-sm font-semibold px-4 py-3 placeholder:text-text-muted/30 focus:border-accent focus:outline-none transition-colors"
          />
        </div>
        <div class="flex-1 overflow-y-auto">
          {#each filteredSongs as song}
            <button
              onclick={() => pickSong(song)}
              class="w-full text-left px-6 py-4 flex items-center gap-4 text-text-secondary hover:bg-bg-surface transition-colors"
            >
              <span class="flex-1 font-display font-semibold tracking-wider truncate">{song.name}</span>
              {#if !song.lyrics}
                <span class="label-sm text-text-muted/30">no lyrics</span>
              {/if}
            </button>
          {/each}
          {#if filteredSongs.length === 0}
            <div class="text-center py-10 label text-text-muted/30">no songs found</div>
          {/if}
        </div>
      </div>
    {/if}

    <!-- Song list overlay -->
    {#if showList && totalItems > 0}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="absolute inset-0 bg-bg-primary/95 z-10 flex flex-col"
        onclick={() => showList = false}
      >
        <div class="px-6 py-5 border-b border-border shrink-0">
          <h2 class="label text-text-muted">{freestyle ? "played" : "setlist"}</h2>
        </div>
        <div class="flex-1 overflow-y-auto">
          {#each items as item, i}
            <button
              onclick={(e) => { e.stopPropagation(); goTo(i); }}
              class="w-full text-left px-6 py-4 flex items-center gap-4 transition-colors {i === currentIndex ? 'bg-accent/10 text-accent' : 'text-text-secondary hover:bg-bg-surface'}"
            >
              <span class="text-[10px] font-mono font-semibold w-6 text-right opacity-50">{i + 1}</span>
              <span class="flex-1 font-display font-semibold tracking-wider truncate">
                {item.song_name || item.custom_name || `Song ${item.position + 1}`}
              </span>
              {#if !item.lyrics}
                <span class="label-sm text-text-muted/30">no lyrics</span>
              {/if}
            </button>
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}
