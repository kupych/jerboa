<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost, apiPatch, apiDelete } from "../lib/api";
  import { markBandSeen } from "../lib/stores/notifications";
  import { ws } from "../lib/ws";
  import { user as currentUser } from "../lib/stores/auth";
  import { navigate } from "../lib/stores/router";
  import { loadBands } from "../lib/stores/bands";
  import { colorSchemes, applyColorScheme } from "../lib/colorSchemes";
  import TrackCard from "../lib/components/TrackCard.svelte";
  import TrackUpload from "../lib/components/TrackUpload.svelte";
  import Recorder from "../lib/components/Recorder.svelte";
  import { setTypeCode, setTypeLabel } from "../lib/utils/format";

  let { slug }: { slug: string } = $props();

  interface BandMember {
    user: { id: string; display_name: string; email: string };
    role: string;
  }

  interface PendingInvite {
    id: string;
    email: string;
    created_at: string;
  }

  interface BandDetail {
    band: { id: string; name: string; slug: string; color_scheme: string };
    members: BandMember[];
    pending_invites?: PendingInvite[];
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
    overdub_of?: string;
    bounced_to?: string;
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


  let ungroupedTracks = $derived(tracks.filter((t) => !t.song_id && !t.set_id && !t.bounced_to && !t.overdub_of));

  // Reload when slug changes (band switching)
  $effect(() => {
    slug; // track dependency
    loadData();
    markBandSeen(slug);
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
    <div class="mb-2">
      <div class="flex items-center gap-3 mb-1">
        <h2 class="text-xl md:text-2xl font-bold tracking-wider font-display">{band.band.name}</h2>
        <div class="flex items-center gap-1">
          <button
            onclick={() => (showInviteForm = !showInviteForm)}
            class="w-7 h-7 flex items-center justify-center text-text-muted/40 hover:text-accent transition-colors"
            title="Invite member"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
              <circle cx="8.5" cy="7" r="4"/>
              <line x1="20" y1="8" x2="20" y2="14"/>
              <line x1="23" y1="11" x2="17" y2="11"/>
            </svg>
          </button>
          {#if isAdmin}
            <button
              onclick={() => { showSettings = !showSettings; if (showSettings && band) editBandName = band.band.name; }}
              class="w-7 h-7 flex items-center justify-center transition-colors {showSettings ? 'text-accent' : 'text-text-muted/40 hover:text-accent'}"
              title="Band settings"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="3"/>
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
              </svg>
            </button>
          {/if}
        </div>
      </div>

      <div class="flex items-center gap-2 md:gap-3 flex-wrap mb-2">
        {#each band.members as member}
          <span class="label-sm text-text-muted px-3 py-1.5 bg-bg-surface border border-border">
            {member.user.display_name || member.user.email}
            {#if member.user.id === $currentUser?.id}
              <span class="text-accent/60 ml-1">you</span>
            {/if}
          </span>
        {/each}
        {#each band.pending_invites ?? [] as invite}
          <span class="label-sm text-text-muted/60 px-3 py-1.5 border border-dashed border-border/40">
            {invite.email}
            <span class="text-text-muted/35 ml-1">invited</span>
          </span>
        {/each}
      </div>

      <div class="grid grid-cols-2 sm:flex sm:items-center gap-1 sm:gap-2">
        <button
          onclick={() => navigate(`/band/${slug}/car`)}
          class="label-sm sm:label text-bg-primary bg-accent hover:bg-accent-hover transition-colors flex items-center justify-center gap-1.5 py-2.5 sm:py-1.5 px-3 sm:px-4 border border-accent"
        >
          <svg class="-translate-y-px" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="5,3 19,12 5,21"/>
          </svg>
          play all
        </button>
        <button
          onclick={() => navigate(`/band/${slug}/perform`)}
          class="label-sm sm:label text-accent hover:bg-accent/10 transition-colors flex items-center justify-center gap-1.5 py-2.5 sm:py-1.5 px-3 sm:px-4 border border-accent/40 hover:border-accent"
        >
          <svg class="-translate-y-px" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M9 18V5l12-2v13"/>
            <circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>
          </svg>
          perform
        </button>
      </div>
    </div>

    <!-- Invite URL -->
    {#if showInviteUrl}
      <div class="mb-4 bg-bg-surface border border-accent/30 p-3 flex items-center gap-3 opacity-70 hover:opacity-100 transition-opacity">
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
        class="mb-4 bg-bg-surface border border-border p-3 md:p-4 flex flex-col sm:flex-row items-stretch sm:items-center gap-3"
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
      <div class="mb-4 bg-bg-surface border border-border p-3 md:p-4 space-y-4">
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
                class="w-7 h-7 md:w-8 md:h-8 border-2 transition-all flex items-center justify-center {band.band.color_scheme === key ? 'border-text-primary scale-110 ring-2 ring-text-primary/30' : 'border-transparent hover:border-text-muted/30'}"
                style="background: {scheme.accent}"
                title={scheme.name}
              >
                {#if band.band.color_scheme === key}
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
                {/if}
              </button>
            {/each}
          </div>
        </div>

        <div>
          <span class="label-sm md:label text-text-muted block mb-2">members</span>
          <div class="space-y-1.5">
            {#each band.members as member}
              <div class="group/member flex items-center justify-between gap-2 py-1.5 px-3 bg-bg-primary border border-border">
                <div class="flex items-center gap-2 min-w-0">
                  <span class="label-sm text-text-primary truncate">{member.user.display_name || member.user.email}</span>
                  <span class="label-sm text-text-muted shrink-0">{member.role}</span>
                </div>
                {#if member.user.id !== $currentUser?.id}
                  <div class="flex items-center gap-2 shrink-0 opacity-0 group-hover/member:opacity-100 transition-opacity">
                    <button
                      onclick={() => toggleRole(member)}
                      class="label-sm text-text-muted hover:text-accent transition-colors hidden sm:block"
                    >
                      {member.role === "admin" ? "demote" : "promote"}
                    </button>
                    <button
                      onclick={() => removeMember(member.user.id)}
                      class="label-sm text-red-400/70 hover:text-red-300 transition-colors text-[10px]"
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

    <!-- Add tracks: Upload / Record / Import -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-1 mb-4">
      <TrackUpload bandSlug={slug} onUploaded={loadTracks} />
      <Recorder bandSlug={slug} onRecorded={loadTracks} />

      <!-- Import URL -->
      {#if showImport}
        <form
          onsubmit={(e) => { e.preventDefault(); importFromUrl(); }}
          class="border border-accent/40 bg-accent/5 p-3 md:col-span-3"
        >
          <div class="grid grid-cols-1 sm:grid-cols-[1fr_1fr_auto_auto] gap-2 items-end">
            <input
              bind:value={importUrl}
              type="url"
              placeholder="youtube url..."
              class="w-full bg-bg-primary border border-border px-4 py-2.5 text-sm text-text-primary font-semibold placeholder:text-text-muted/40 focus:outline-none focus:border-accent transition-colors"
            />
            <input
              bind:value={importTitle}
              type="text"
              placeholder="title (optional)"
              class="w-full bg-bg-primary border border-border px-4 py-2.5 text-sm text-text-primary font-semibold placeholder:text-text-muted/40 focus:outline-none focus:border-accent transition-colors"
            />
            <button
              type="submit"
              disabled={importing || !importUrl.trim()}
              class="px-4 py-2.5 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors"
            >
              {importing ? "..." : "import"}
            </button>
            <button
              type="button"
              onclick={() => (showImport = false)}
              class="px-4 py-2.5 label-sm text-text-muted hover:text-text-secondary transition-colors"
            >
              cancel
            </button>
          </div>
        </form>
      {:else}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="border border-dashed border-border hover:border-accent/40 hover:bg-accent/[0.02] p-3 md:py-2 text-center transition-colors cursor-pointer flex items-center justify-center gap-2 whitespace-nowrap"
          onclick={() => (showImport = true)}
        >
          <svg class="text-text-muted shrink-0" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
            <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
          </svg>
          <span class="text-sm text-text-muted font-semibold">import url</span>
          <span class="label-sm text-text-muted/30 font-mono hidden md:inline translate-y-px">youtube, etc</span>
        </div>
      {/if}
    </div>

    <!-- Tab toggle -->
    <div class="flex items-center gap-6 mb-2">
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
            class="bg-transparent border border-accent/30 px-3 py-1 label-sm text-text-secondary placeholder:text-accent/70 focus:outline-none focus:border-accent focus:bg-accent/[0.03] transition-colors w-40"
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
        <div class="space-y-1 mb-4">
          {#each songs as song}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="bg-bg-surface border border-border border-l-2 border-l-transparent hover:border-l-accent hover:border-accent/40 px-4 py-3 transition-all cursor-pointer group flex items-center justify-between"
              onclick={() => navigate(`/band/${slug}/song/${song.id}`)}
            >
              <h4 class="text-base font-semibold tracking-wider text-text-primary font-display group-hover:text-accent transition-colors">{song.name}</h4>
              <div class="flex items-center gap-3">
                <span class="label-sm text-text-muted group-hover:text-accent transition-colors">{song.take_count} {song.take_count === 1 ? 'take' : 'takes'}</span>
                <span class="label-sm text-text-muted/0 group-hover:text-accent transition-colors">&rsaquo;</span>
              </div>
            </div>
          {/each}
        </div>
      {:else if tracks.length === 0}
        <div class="text-center py-12 label text-text-muted mb-4">
          no songs yet
        </div>
      {/if}
    {:else}
      {#if sets.length > 0}
        <div class="space-y-1 mb-4">
          {#each sets as set}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="bg-bg-surface border border-border px-4 py-3 hover:border-accent/40 transition-colors cursor-pointer group flex items-center justify-between"
              onclick={() => navigate(`/band/${slug}/set/${set.id}`)}
            >
              <div class="flex items-center gap-3 min-w-0">
                <h4 class="text-base font-semibold tracking-wider text-text-primary font-display group-hover:text-accent transition-colors truncate">{set.name}</h4>
                <span class="label-sm text-accent bg-accent/10 px-2 py-0.5 shrink-0" title={setTypeLabel(set.set_type)}>{setTypeLabel(set.set_type)}</span>
              </div>
              <div class="flex items-center gap-4 label-sm text-text-muted/60 shrink-0">
                {#if set.item_count > 0}
                  <span>{set.item_count} {set.item_count === 1 ? 'song' : 'songs'}</span>
                {/if}
                {#if set.recorded_at}
                  <span>{new Date(set.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-center py-12 label text-text-muted mb-4">
          no sets yet
        </div>
      {/if}
    {/if}

    <!-- Ungrouped tracks -->
    {#if ungroupedTracks.length > 0}
      <div class="mt-6 pt-4 border-t border-border/30">
        <h3 class="label text-text-muted/50 mb-2">unassigned takes</h3>
        <div class="space-y-1">
          {#each ungroupedTracks as track}
            <TrackCard {track} bandSlug={slug} onDelete={loadTracks} />
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}
