<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, apiPost } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import ChordProLyrics from "../lib/components/ChordProLyrics.svelte";

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
    }
  }

  function next() {
    if (currentIndex < totalItems - 1) {
      currentIndex++;
      scrollTop();
    } else if (freestyle) {
      openPicker();
    }
  }

  function prev() {
    if (currentIndex > 0) {
      currentIndex--;
      scrollTop();
    }
  }

  function goTo(index: number) {
    currentIndex = index;
    showList = false;
    scrollTop();
  }

  function scrollTop() {
    document.getElementById("perform-lyrics")?.scrollTo(0, 0);
  }

  function exit() {
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
    scrollTop();
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
        {#if totalItems > 0}
          <div class="text-[10px] font-mono font-semibold tracking-[0.3em] text-text-muted/40 uppercase">
            {currentIndex + 1} / {totalItems}
          </div>
        {/if}
        <div class="label text-text-primary">{title}</div>
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
      </div>
    </div>

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
      <div class="px-6 pt-5 pb-3 shrink-0">
        <h1 class="text-xl md:text-2xl font-display font-bold tracking-wider text-text-primary">{itemName}</h1>
        {#if currentItem.notes}
          <p class="text-xs font-semibold text-text-muted mt-1">{currentItem.notes}</p>
        {/if}
      </div>

      <!-- Lyrics area -->
      <div id="perform-lyrics" class="flex-1 overflow-y-auto px-6 pb-24 text-base md:text-lg">
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
            class="pointer-events-auto label-sm text-accent hover:text-accent-hover transition-colors"
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
