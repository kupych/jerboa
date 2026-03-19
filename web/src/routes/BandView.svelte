<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost, apiPatch, apiDelete } from "../lib/api";
  import { ws } from "../lib/ws";
  import { user as currentUser } from "../lib/stores/auth";
  import { navigate } from "../lib/stores/router";
  import TrackCard from "../lib/components/TrackCard.svelte";
  import TrackUpload from "../lib/components/TrackUpload.svelte";

  let { slug }: { slug: string } = $props();

  interface BandMember {
    user: { id: string; display_name: string; email: string };
    role: string;
  }

  interface BandDetail {
    band: { id: string; name: string; slug: string };
    members: BandMember[];
  }

  interface Song {
    id: string;
    name: string;
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
    song?: { id: string; name: string };
    source_url?: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
  }

  let band = $state<BandDetail | null>(null);
  let tracks = $state<Track[]>([]);
  let songs = $state<Song[]>([]);
  let loading = $state(true);
  let showInviteUrl = $state("");
  let generatingInvite = $state(false);
  let inviteEmail = $state("");
  let showInviteForm = $state(false);
  let viewMode = $state<"all" | "songs">("all");
  let newSongName = $state("");
  let creatingSong = $state(false);
  let showImport = $state(false);
  let importUrl = $state("");
  let importTitle = $state("");
  let importing = $state(false);

  // Band management
  let showSettings = $state(false);
  let editBandName = $state("");
  let savingBandName = $state(false);
  let isAdmin = $derived(band?.members.some((m) => m.user.id === $currentUser?.id && m.role === "admin") ?? false);

  // Group tracks by song
  let tracksBySong = $derived(() => {
    const grouped = new Map<string, { song: Song; tracks: Track[] }>();
    const ungrouped: Track[] = [];

    for (const track of tracks) {
      if (track.song_id && track.song) {
        const existing = grouped.get(track.song_id);
        if (existing) {
          existing.tracks.push(track);
        } else {
          grouped.set(track.song_id, { song: track.song, tracks: [track] });
        }
      } else {
        ungrouped.push(track);
      }
    }

    return { grouped: Array.from(grouped.values()), ungrouped };
  });

  onMount(() => {
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
      [band, tracks, songs] = await Promise.all([
        api<BandDetail>(`/api/bands/${slug}`),
        api<Track[]>(`/api/bands/${slug}/tracks`),
        api<Song[]>(`/api/bands/${slug}/songs`),
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
  <div class="text-center py-20 label text-text-muted">loading</div>
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

      <div class="flex items-center gap-4">
        <button
          onclick={() => (showImport = !showImport)}
          class="label text-text-muted hover:text-accent transition-colors flex items-center gap-2"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
            <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
          </svg>
          import url
        </button>
        <button
          onclick={() => (showInviteForm = !showInviteForm)}
          class="label text-text-muted hover:text-accent transition-colors flex items-center gap-2"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
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
            class="label transition-colors flex items-center gap-2 {showSettings ? 'text-accent' : 'text-text-muted hover:text-accent'}"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
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
      <div class="mb-8 bg-bg-surface border border-accent/30 p-5 flex items-center gap-4">
        <input
          type="text"
          value={showInviteUrl}
          readonly
          class="flex-1 bg-transparent label-sm text-text-secondary font-mono outline-none"
        />
        <button
          onclick={copyInvite}
          class="label text-accent hover:text-accent-hover shrink-0 transition-colors"
        >
          copy
        </button>
        <button
          onclick={() => (showInviteUrl = "")}
          class="label text-text-muted hover:text-text-secondary shrink-0 transition-colors"
        >
          dismiss
        </button>
      </div>
    {/if}

    <!-- Invite form -->
    {#if showInviteForm}
      <form
        onsubmit={(e) => { e.preventDefault(); generateInvite(); }}
        class="mb-8 bg-bg-surface border border-border p-6 flex items-center gap-4"
      >
        <input
          bind:value={inviteEmail}
          type="email"
          placeholder="email@example.com"
          class="flex-1 bg-bg-primary border border-border px-4 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
        />
        <button
          type="submit"
          disabled={generatingInvite || !inviteEmail.trim()}
          class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
        >
          {generatingInvite ? "..." : "send invite"}
        </button>
        <button
          type="button"
          onclick={() => (showInviteForm = false)}
          class="label text-text-muted hover:text-text-secondary transition-colors"
        >cancel</button>
      </form>
    {/if}

    <!-- Settings panel -->
    {#if showSettings && isAdmin && band}
      <div class="mb-8 bg-bg-surface border border-border p-6 space-y-6">
        <div>
          <span class="label text-text-muted block mb-2">band name</span>
          <form onsubmit={(e) => { e.preventDefault(); updateBandName(); }} class="flex gap-3">
            <input
              bind:value={editBandName}
              type="text"
              class="flex-1 bg-bg-primary border border-border px-4 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
            />
            <button
              type="submit"
              disabled={savingBandName || !editBandName.trim() || editBandName.trim() === band.band.name}
              class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
            >
              {savingBandName ? "..." : "save"}
            </button>
          </form>
        </div>

        <div>
          <span class="label text-text-muted block mb-3">members</span>
          <div class="space-y-2">
            {#each band.members as member}
              <div class="flex items-center justify-between gap-4 py-2 px-3 bg-bg-primary border border-border">
                <div class="flex items-center gap-3 min-w-0">
                  <span class="label-sm text-text-primary truncate">{member.user.display_name || member.user.email}</span>
                  <span class="label-sm text-text-muted shrink-0">{member.role}</span>
                </div>
                {#if member.user.id !== $currentUser?.id}
                  <div class="flex items-center gap-3 shrink-0">
                    <button
                      onclick={() => toggleRole(member)}
                      class="label-sm text-text-muted hover:text-accent transition-colors"
                    >
                      {member.role === "admin" ? "make member" : "make admin"}
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

    <!-- Upload -->
    <div class="mb-8">
      <TrackUpload bandSlug={slug} onUploaded={loadTracks} />
    </div>

    <!-- View toggle + Songs -->
    <div class="flex items-center justify-between mb-6">
      <div class="flex gap-4">
        <button
          onclick={() => (viewMode = "all")}
          class="label transition-colors {viewMode === 'all' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
        >
          all tracks
        </button>
        <button
          onclick={() => (viewMode = "songs")}
          class="label transition-colors {viewMode === 'songs' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
        >
          by song
        </button>
      </div>

      {#if viewMode === "songs"}
        <form onsubmit={(e) => { e.preventDefault(); createSong(); }} class="flex gap-2">
          <input
            bind:value={newSongName}
            type="text"
            placeholder="+ new song"
            disabled={creatingSong}
            class="bg-transparent border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-40"
          />
        </form>
      {/if}
    </div>

    <!-- Track list -->
    {#if viewMode === "all"}
      <div class="space-y-3">
        {#each tracks as track}
          <TrackCard {track} bandSlug={slug} />
        {/each}

        {#if tracks.length === 0}
          <div class="text-center py-20 label text-text-muted">
            no tracks yet
          </div>
        {/if}
      </div>
    {:else}
      <!-- Grouped by song -->
      {#each tracksBySong().grouped as { song, tracks: songTracks }}
        <div class="mb-8">
          <h3 class="label text-text-secondary mb-4 font-display">{song.name}</h3>
          <div class="space-y-3">
            {#each songTracks as track}
              <TrackCard {track} bandSlug={slug} />
            {/each}
          </div>
        </div>
      {/each}

      {#if tracksBySong().ungrouped.length > 0}
        <div class="mb-8">
          <h3 class="label text-text-muted mb-4">ungrouped</h3>
          <div class="space-y-3">
            {#each tracksBySong().ungrouped as track}
              <TrackCard {track} bandSlug={slug} />
            {/each}
          </div>
        </div>
      {/if}

      {#if tracks.length === 0}
        <div class="text-center py-20 label text-text-muted">
          no tracks yet
        </div>
      {/if}
    {/if}
  </div>
{/if}
