<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost, apiPatch, apiDelete } from "../lib/api";
  import { ws } from "../lib/ws";
  import { user as currentUser } from "../lib/stores/auth";
  import { navigate } from "../lib/stores/router";
  import { loadBands } from "../lib/stores/bands";
  import { colorSchemes, applyColorScheme } from "../lib/colorSchemes";
  import TrackCard from "../lib/components/TrackCard.svelte";
  import TrackUpload from "../lib/components/TrackUpload.svelte";
  import Recorder from "../lib/components/Recorder.svelte";
  import { setTypeCode } from "../lib/utils/format";

  let { slug }: { slug: string } = $props();

  interface BandMember {
    user: { id: string; display_name: string; email: string };
    role: string;
  }

  interface BandDetail {
    band: { id: string; name: string; slug: string; color_scheme: string };
    members: BandMember[];
  }

  interface Song {
    id: string;
    name: string;
    take_count: number;
  }

  interface SetSummary {
    id: string;
    name: string;
    set_type: string;
    recorded_at?: string;
    notes?: string;
    item_count: number;
    created_at: string;
  }

  interface Track {
    id: string;
    title: string;
    description?: string;
    duration_ms: number;
    format: string;
    file_size: number;
    status: string;
    tags: string[];
    song_id?: string;
    set_id?: string;
    song?: { id: string; name: string };
    source_url?: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
  }

  let band = $state<BandDetail | null>(null);
  let tracks = $state<Track[]>([]);
  let songs = $state<Song[]>([]);
  let sets = $state<SetSummary[]>([]);
  let loading = $state(true);
  let viewTab = $state<"songs" | "sets">("songs");
  let showInviteUrl = $state("");
  let generatingInvite = $state(false);
  let inviteEmail = $state("");
  let showInviteForm = $state(false);
  let newSongName = $state("");
  let creatingSong = $state(false);
  let newSetName = $state("");
  let newSetType = $state("rehearsal");
  let creatingSet = $state(false);
  let showImport = $state(false);
  let importUrl = $state("");
  let importTitle = $state("");
  let importing = $state(false);

  // Band management
  let showSettings = $state(false);
  let editBandName = $state("");
  let savingBandName = $state(false);
  let addMemberEmail = $state("");
  let addingMember = $state(false);
  let addMemberError = $state("");
  let isAdmin = $derived(band?.members.some((m) => m.user.id === $currentUser?.id && m.role === "admin") ?? false);


  let ungroupedTracks = $derived(tracks.filter((t) => !t.song_id && !t.set_id));

  // Reload when slug changes (band switching)
  $effect(() => {
    slug; // track dependency
    loadData();
  });

  $effect(() => {
    if (band) {
      ws.subscribe(`band:${band.band.id}`);
      const off = ws.on("track.ready", () => loadTracks());
      return () => {
        ws.unsubscribe(`band:${band!.band.id}`);
        off();
      };
    }
  });

  async function loadData() {
    loading = true;
    try {
      [band, tracks, songs, sets] = await Promise.all([
        api<BandDetail>(`/api/bands/${slug}`),
        api<Track[]>(`/api/bands/${slug}/tracks`),
        api<Song[]>(`/api/bands/${slug}/songs`),
        api<SetSummary[]>(`/api/bands/${slug}/sets`),
      ]);
    } finally {
      loading = false;
    }
  }

  async function loadTracks() {
    tracks = await api<Track[]>(`/api/bands/${slug}/tracks`);
  }

  async function loadSongs() {
    songs = await api<Song[]>(`/api/bands/${slug}/songs`);
  }

  async function generateInvite() {
    if (!inviteEmail.trim()) return;
    generatingInvite = true;
    try {
      const res = await apiPost<{ invite_url: string }>(`/api/bands/${slug}/invite`, {
        email: inviteEmail.trim(),
      });
      showInviteUrl = res.invite_url;
      inviteEmail = "";
      showInviteForm = false;
    } finally {
      generatingInvite = false;
    }
  }

  async function copyInvite() {
    await navigator.clipboard.writeText(showInviteUrl);
  }

  async function createSong() {
    if (!newSongName.trim()) return;
    creatingSong = true;
    try {
      await apiPost(`/api/bands/${slug}/songs`, { name: newSongName.trim() });
      newSongName = "";
      await loadSongs();
    } finally {
      creatingSong = false;
    }
  }

  async function loadSets() {
    sets = await api<SetSummary[]>(`/api/bands/${slug}/sets`);
  }

  async function createSet() {
    if (!newSetName.trim()) return;
    creatingSet = true;
    try {
      const set = await apiPost<SetSummary>(`/api/bands/${slug}/sets`, {
        name: newSetName.trim(),
        set_type: newSetType,
      });
      newSetName = "";
      newSetType = "rehearsal";
      navigate(`/band/${slug}/set/${set.id}`);
    } finally {
      creatingSet = false;
    }
  }

  async function setColorScheme(scheme: string) {
    if (!band) return;
    await apiPatch(`/api/bands/${slug}`, { color_scheme: scheme });
    band.band.color_scheme = scheme;
    applyColorScheme(scheme);
    await loadBands();
  }

  async function updateBandName() {
    if (!editBandName.trim() || !band) return;
    savingBandName = true;
    try {
      const updated = await apiPatch<{ name: string; slug: string }>(`/api/bands/${slug}`, {
        name: editBandName.trim(),
      });
      band.band.name = updated.name;
      if (updated.slug !== slug) {
        navigate(`/band/${updated.slug}`);
      }
    } finally {
      savingBandName = false;
    }
  }

  async function removeMember(userId: string) {
    if (!band) return;
    await apiDelete(`/api/bands/${slug}/members/${userId}`);
    band.members = band.members.filter((m) => m.user.id !== userId);
  }

  async function toggleRole(member: BandMember) {
    if (!band) return;
    const newRole = member.role === "admin" ? "member" : "admin";
    await apiPatch(`/api/bands/${slug}/members/${member.user.id}`, { role: newRole });
    member.role = newRole;
    band.members = [...band.members];
  }

  async function addMember() {
    if (!addMemberEmail.trim() || !band) return;
    addingMember = true;
    addMemberError = "";
    try {
      await apiPost(`/api/bands/${slug}/members`, { email: addMemberEmail.trim() });
      addMemberEmail = "";
      await loadData();
    } catch (e: any) {
      const msg = e?.message || "";
      if (msg.includes("not found")) {
        addMemberError = "no user with that email";
      } else {
        addMemberError = "failed to add member";
      }
    } finally {
      addingMember = false;
    }
  }

  async function importFromUrl() {
    if (!importUrl.trim()) return;
    importing = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/import`, {
        url: importUrl.trim(),
        title: importTitle.trim() || undefined,
      });
      importUrl = "";
      importTitle = "";
      showImport = false;
      await loadTracks();
    } finally {
      importing = false;
    }
  }
</script>

{#if loading}
  <div class="flex items-center justify-center gap-2 py-20 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
{:else if band}
  <div>
    <!-- Band header -->
    <div class="flex flex-col md:flex-row md:items-start justify-between gap-4 mb-8 md:mb-10">
      <div>
        <h2 class="text-xl md:text-2xl font-bold tracking-wider font-display">{band.band.name}</h2>
        <div class="flex items-center gap-2 md:gap-3 mt-3 md:mt-4 flex-wrap">
          {#each band.members as member}
            <span class="label-sm text-text-muted px-3 py-1.5 bg-bg-surface border border-border">
              {member.user.display_name || member.user.email}
              {#if member.role === "admin"}
                <span class="text-accent">*</span>
              {/if}
            </span>
          {/each}
        </div>
      </div>

      <div class="grid grid-cols-4 sm:flex sm:items-center gap-1 sm:gap-4 w-full sm:w-auto">
        <button
          onclick={() => navigate(`/band/${slug}/car`)}
          class="label-sm sm:label text-text-muted hover:text-accent transition-colors flex items-center justify-center sm:justify-start gap-1.5 py-2 sm:py-0 bg-bg-surface sm:bg-transparent border border-border sm:border-0"
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="5,3 19,12 5,21"/>
          </svg>
          play all
        </button>
        <button
          onclick={() => (showImport = !showImport)}
          class="label-sm sm:label text-text-muted hover:text-accent transition-colors flex items-center justify-center sm:justify-start gap-1.5 py-2 sm:py-0 bg-bg-surface sm:bg-transparent border border-border sm:border-0"
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
            <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
          </svg>
          import
        </button>
        <button
          onclick={() => (showInviteForm = !showInviteForm)}
          class="label-sm sm:label text-text-muted hover:text-accent transition-colors flex items-center justify-center sm:justify-start gap-1.5 py-2 sm:py-0 bg-bg-surface sm:bg-transparent border border-border sm:border-0"
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
            <circle cx="8.5" cy="7" r="4"/>
            <line x1="20" y1="8" x2="20" y2="14"/>
            <line x1="23" y1="11" x2="17" y2="11"/>
          </svg>
          invite
        </button>
        {#if isAdmin}
          <button
            onclick={() => { showSettings = !showSettings; if (showSettings && band) editBandName = band.band.name; }}
            class="label-sm sm:label transition-colors flex items-center justify-center sm:justify-start gap-1.5 py-2 sm:py-0 bg-bg-surface sm:bg-transparent border border-border sm:border-0 {showSettings ? 'text-accent' : 'text-text-muted hover:text-accent'}"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="3"/>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
            settings
          </button>
        {/if}
      </div>
    </div>

    <!-- Invite URL -->
    {#if showInviteUrl}
      <div class="mb-8 bg-bg-surface border border-accent/30 p-4 flex items-center gap-3">
        <input
          type="text"
          value={showInviteUrl}
          readonly
          class="flex-1 min-w-0 bg-transparent label-sm text-text-secondary font-mono outline-none truncate"
        />
        <button
          onclick={copyInvite}
          class="label-sm text-accent hover:text-accent-hover shrink-0 transition-colors"
        >
          copy
        </button>
        <button
          onclick={() => (showInviteUrl = "")}
          class="label-sm text-text-muted hover:text-text-secondary shrink-0 transition-colors"
        >
          dismiss
        </button>
      </div>
    {/if}

    <!-- Invite form -->
    {#if showInviteForm}
      <form
        onsubmit={(e) => { e.preventDefault(); generateInvite(); }}
        class="mb-8 bg-bg-surface border border-border p-4 md:p-6 flex flex-col sm:flex-row items-stretch sm:items-center gap-3"
      >
        <input
          bind:value={inviteEmail}
          type="email"
          placeholder="email@example.com"
          class="flex-1 bg-bg-primary border border-border px-4 py-2.5 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
        />
        <div class="flex items-center gap-3">
          <button
            type="submit"
            disabled={generatingInvite || !inviteEmail.trim()}
            class="px-5 py-2.5 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors whitespace-nowrap"
          >
            {generatingInvite ? "..." : "send invite"}
          </button>
          <button
            type="button"
            onclick={() => (showInviteForm = false)}
            class="label-sm text-text-muted hover:text-text-secondary transition-colors whitespace-nowrap"
          >cancel</button>
        </div>
      </form>
    {/if}

    <!-- Settings panel -->
    {#if showSettings && isAdmin && band}
      <div class="mb-8 bg-bg-surface border border-border p-4 md:p-6 space-y-5">
        <div>
          <span class="label-sm md:label text-text-muted block mb-2">band name</span>
          <form onsubmit={(e) => { e.preventDefault(); updateBandName(); }} class="flex gap-2">
            <input
              bind:value={editBandName}
              type="text"
              class="flex-1 min-w-0 bg-bg-primary border border-border px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
            />
            <button
              type="submit"
              disabled={savingBandName || !editBandName.trim() || editBandName.trim() === band.band.name}
              class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors shrink-0"
            >
              {savingBandName ? "..." : "save"}
            </button>
          </form>
        </div>

        <div>
          <span class="label-sm md:label text-text-muted block mb-2">color scheme</span>
          <div class="flex flex-wrap gap-2">
            {#each Object.entries(colorSchemes) as [key, scheme]}
              <button
                onclick={() => setColorScheme(key)}
                class="w-7 h-7 md:w-8 md:h-8 border-2 transition-all {band.band.color_scheme === key ? 'border-text-primary scale-110' : 'border-transparent hover:border-text-muted/30'}"
                style="background: {scheme.accent}"
                title={scheme.name}
              ></button>
            {/each}
          </div>
        </div>

        <div>
          <span class="label-sm md:label text-text-muted block mb-2">members</span>
          <div class="space-y-1.5">
            {#each band.members as member}
              <div class="flex items-center justify-between gap-2 py-1.5 px-3 bg-bg-primary border border-border">
                <div class="flex items-center gap-2 min-w-0">
                  <span class="label-sm text-text-primary truncate">{member.user.display_name || member.user.email}</span>
                  <span class="label-sm text-text-muted shrink-0">{member.role}</span>
                </div>
                {#if member.user.id !== $currentUser?.id}
                  <div class="flex items-center gap-2 shrink-0">
                    <button
                      onclick={() => toggleRole(member)}
                      class="label-sm text-text-muted hover:text-accent transition-colors hidden sm:block"
                    >
                      {member.role === "admin" ? "demote" : "promote"}
                    </button>
                    <button
                      onclick={() => removeMember(member.user.id)}
                      class="label-sm text-red-400 hover:text-red-300 transition-colors"
                    >
                      remove
                    </button>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
          <form onsubmit={(e) => { e.preventDefault(); addMember(); }} class="flex gap-2 mt-2">
            <input
              bind:value={addMemberEmail}
              type="email"
              placeholder="add by email"
              class="flex-1 min-w-0 bg-bg-primary border border-border px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
            />
            <button
              type="submit"
              disabled={addingMember || !addMemberEmail.trim()}
              class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors shrink-0"
            >
              {addingMember ? "..." : "add"}
            </button>
          </form>
          {#if addMemberError}
            <div class="label-sm text-danger mt-2">{addMemberError}</div>
          {/if}
        </div>

        <button
          onclick={() => (showSettings = false)}
          class="label text-text-muted hover:text-text-secondary transition-colors"
        >close</button>
      </div>
    {/if}

    <!-- YouTube Import -->
    {#if showImport}
      <form
        onsubmit={(e) => { e.preventDefault(); importFromUrl(); }}
        class="mb-8 bg-bg-surface border border-border p-6 space-y-4"
      >
        <div>
          <span class="label text-text-muted block mb-2">youtube url</span>
          <input
            bind:value={importUrl}
            type="url"
            placeholder="https://www.youtube.com/watch?v=..."
            class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
          />
        </div>
        <div>
          <span class="label text-text-muted block mb-2">title (optional, auto-detected)</span>
          <input
            bind:value={importTitle}
            type="text"
            placeholder="Leave blank to use video title"
            class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
          />
        </div>
        <div class="flex gap-4">
          <button
            type="submit"
            disabled={importing || !importUrl.trim()}
            class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
          >
            {importing ? "importing..." : "import"}
          </button>
          <button
            type="button"
            onclick={() => (showImport = false)}
            class="px-6 py-3 label text-text-muted hover:text-text-secondary transition-colors"
          >
            cancel
          </button>
        </div>
      </form>
    {/if}

    <!-- Upload & Record -->
    <div class="mb-8 space-y-2">
      <TrackUpload bandSlug={slug} onUploaded={loadTracks} />
      <Recorder bandSlug={slug} onRecorded={loadTracks} />
    </div>

    <!-- Tab toggle -->
    <div class="flex items-center gap-6 mb-6">
      <button
        onclick={() => (viewTab = "songs")}
        class="label transition-colors {viewTab === 'songs' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
      >songs</button>
      <button
        onclick={() => (viewTab = "sets")}
        class="label transition-colors {viewTab === 'sets' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
      >sets</button>

      {#if viewTab === "songs"}
        <form onsubmit={(e) => { e.preventDefault(); createSong(); }} class="ml-auto flex gap-2">
          <input
            bind:value={newSongName}
            type="text"
            placeholder="+ new song"
            disabled={creatingSong}
            class="bg-transparent border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-40"
          />
        </form>
      {:else}
        <form onsubmit={(e) => { e.preventDefault(); createSet(); }} class="ml-auto flex gap-2">
          <select
            bind:value={newSetType}
            class="bg-transparent border border-border px-2 py-1 label-sm text-text-secondary focus:outline-none focus:border-accent transition-colors"
          >
            <option value="rehearsal">rehearsal</option>
            <option value="live">live</option>
            <option value="pre-production">pre-production</option>
            <option value="other">other</option>
          </select>
          <input
            bind:value={newSetName}
            type="text"
            placeholder="+ new set"
            disabled={creatingSet}
            class="bg-transparent border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-40"
          />
        </form>
      {/if}
    </div>

    {#if viewTab === "songs"}
      {#if songs.length > 0}
        <div class="space-y-2 mb-10">
          {#each songs as song}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="bg-bg-surface border border-border p-5 hover:border-accent/40 transition-colors cursor-pointer group flex items-center justify-between"
              onclick={() => navigate(`/band/${slug}/song/${song.id}`)}
            >
              <h4 class="text-base font-semibold tracking-wider text-text-primary font-display group-hover:text-accent transition-colors">{song.name}</h4>
              <span class="label-sm text-text-muted">{song.take_count} {song.take_count === 1 ? 'take' : 'takes'}</span>
            </div>
          {/each}
        </div>
      {:else if tracks.length === 0}
        <div class="text-center py-20 label text-text-muted mb-10">
          no songs yet
        </div>
      {/if}
    {:else}
      {#if sets.length > 0}
        <div class="space-y-2 mb-10">
          {#each sets as set}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="bg-bg-surface border border-border p-5 hover:border-accent/40 transition-colors cursor-pointer group flex items-center justify-between"
              onclick={() => navigate(`/band/${slug}/set/${set.id}`)}
            >
              <div class="flex items-center gap-3 min-w-0">
                <h4 class="text-base font-semibold tracking-wider text-text-primary font-display group-hover:text-accent transition-colors truncate">{set.name}</h4>
                <span class="label-sm text-accent bg-accent/10 px-2 py-0.5 shrink-0 font-mono">{setTypeCode(set.set_type)}</span>
              </div>
              <div class="flex items-center gap-4 label-sm text-text-muted shrink-0">
                <span>{set.item_count} {set.item_count === 1 ? 'song' : 'songs'}</span>
                {#if set.recorded_at}
                  <span>{new Date(set.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-center py-20 label text-text-muted mb-10">
          no sets yet
        </div>
      {/if}
    {/if}

    <!-- Ungrouped tracks -->
    {#if ungroupedTracks.length > 0}
      <div>
        <h3 class="label text-text-muted mb-4">unassigned takes</h3>
        <div class="space-y-3">
          {#each ungroupedTracks as track}
            <TrackCard {track} bandSlug={slug} onDelete={loadTracks} />
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}
