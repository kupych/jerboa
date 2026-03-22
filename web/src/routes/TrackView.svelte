<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost, apiPatch, apiDelete } from "../lib/api";
  import { ws } from "../lib/ws";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, formatFileSize, formatRelativeTime } from "../lib/utils/format";
  import WaveformPlayer from "../lib/components/WaveformPlayer.svelte";
  import CommentList from "../lib/components/CommentList.svelte";
  import Recorder from "../lib/components/Recorder.svelte";

  let { slug, trackId }: { slug: string; trackId: string } = $props();

  interface Song {
    id: string;
    name: string;
  }

  interface SetSummary {
    id: string;
    name: string;
    set_type: string;
  }

  interface Track {
    id: string;
    band_id: string;
    title: string;
    description?: string;
    notes?: string;
    waveform_data?: number[];
    duration_ms: number;
    format: string;
    sample_rate: number;
    file_size: number;
    status: string;
    tags: string[];
    song_id?: string;
    song?: Song;
    source_url?: string;
    recorded_at?: string;
    set_id?: string;
    overdub_of?: string;
    offset_ms: number;
    created_at: string;
    uploader?: { display_name: string; email: string };
  }

  interface Comment {
    id: string;
    body: string;
    timestamp_ms: number | null;
    created_at: string;
    user: { id: string; display_name: string; email: string; avatar_url?: string };
    replies?: Comment[];
  }

  interface Member {
    user: { id: string; display_name: string; email: string };
    role: string;
  }

  interface Personnel {
    user_id: string;
    role: string;
    user: { id: string; display_name: string; email: string; avatar_url?: string };
  }

  let track = $state<Track | null>(null);
  let comments = $state<Comment[]>([]);
  let loading = $state(true);
  let newComment = $state("");
  let commentTimestamp = $state<number | null>(null);
  let posting = $state(false);
  let newTag = $state("");
  let savingTags = $state(false);

  let songs = $state<Song[]>([]);
  let allSets = $state<SetSummary[]>([]);
  let members = $state<Member[]>([]);
  let personnel = $state<Personnel[]>([]);
  let addPersonnelId = $state("");
  let addPersonnelRole = $state("");

  // Overdubs
  type Overdub = {
    id: string;
    title: string;
    waveform_data?: number[];
    duration_ms: number;
    format: string;
    file_size: number;
    status: string;
    offset_ms: number;
    created_at: string;
    vote_count: number;
    user_voted: boolean;
    uploader?: { display_name: string; email: string };
  };

  let overdubs = $state<Overdub[]>([]);
  let showOverdubRecord = $state(false);
  let voting = $state(false);
  let bouncing = $state(false);
  let scrubbing = $state(false);
  let isAdmin = $derived(
    members.some((m) => m.user.id === $currentUser?.id && m.role === "admin")
  );

  import { user as currentUser } from "../lib/stores/auth";

  // Editable meta
  let editing = $state(false);
  let editTitle = $state("");
  let editDesc = $state("");
  let editNotes = $state("");
  let editRecordedAt = $state("");
  let savingMeta = $state(false);
  let assigningSet = $state(false);

  // Song combobox
  let songInput = $state("");
  let songDropdownOpen = $state(false);
  let filteredSongs = $derived(
    songInput.trim()
      ? songs.filter((s) => s.name.toLowerCase().includes(songInput.trim().toLowerCase()))
      : songs
  );
  let assigningSong = $state(false);

  let streamUrl = $derived(`/api/bands/${slug}/tracks/${trackId}/stream`);
  let timedComments = $derived(
    comments
      .flatMap((c) => {
        const items = [];
        if (c.timestamp_ms != null) {
          items.push({
            id: c.id,
            timestamp_ms: c.timestamp_ms,
            body: c.body,
            user_name: c.user.display_name || c.user.email,
          });
        }
        return items;
      })
  );

  onMount(async () => {
    await loadData();
  });

  $effect(() => {
    if (track) {
      ws.subscribe(`track:${track.id}`);
      const off = ws.on("comment.new", () => loadComments());
      return () => {
        ws.unsubscribe(`track:${track!.id}`);
        off();
      };
    }
  });

  async function loadData() {
    loading = true;
    try {
      const bandDetail = api<{ band: any; members: Member[] }>(`/api/bands/${slug}`);
      [track, songs, personnel, allSets] = await Promise.all([
        api<Track>(`/api/bands/${slug}/tracks/${trackId}`),
        api<Song[]>(`/api/bands/${slug}/songs`),
        api<Personnel[]>(`/api/bands/${slug}/tracks/${trackId}/personnel`),
        api<SetSummary[]>(`/api/bands/${slug}/sets`),
      ]);
      const bd = await bandDetail;
      members = bd.members;
      await Promise.all([loadComments(), loadOverdubs()]);
    } finally {
      loading = false;
    }
  }

  async function loadComments() {
    comments = await api<Comment[]>(`/api/tracks/${trackId}/comments`);
  }

  function handleTimestampClick(ms: number) {
    commentTimestamp = ms;
    document.getElementById("comment-input")?.focus();
  }

  function handleSeek(_ms: number) {}

  async function submitComment() {
    if (!newComment.trim()) return;
    posting = true;
    try {
      await apiPost(`/api/tracks/${trackId}/comments`, {
        body: newComment.trim(),
        timestamp_ms: commentTimestamp,
      });
      newComment = "";
      commentTimestamp = null;
      await loadComments();
    } finally {
      posting = false;
    }
  }

  function clearTimestamp() {
    commentTimestamp = null;
  }

  async function loadOverdubs() {
    overdubs = await api<Overdub[]>(`/api/bands/${slug}/tracks/${trackId}/overdubs`);
  }

  async function voteOverdub(overdubId: string | null) {
    voting = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/overdubs/vote`, {
        overdub_id: overdubId,
      });
      await loadOverdubs();
    } finally {
      voting = false;
    }
  }

  async function bounceOverdub(overdubId: string) {
    bouncing = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/overdubs/bounce`, {
        overdub_id: overdubId,
      });
    } finally {
      bouncing = false;
    }
  }

  async function adjustOffset(overdubId: string, offsetMs: number) {
    await apiPatch(`/api/bands/${slug}/tracks/${trackId}/overdubs/${overdubId}/offset`, {
      offset_ms: offsetMs,
    });
    const od = overdubs.find((o) => o.id === overdubId);
    if (od) od.offset_ms = offsetMs;
  }

  async function scrubOverdubs(keepId?: string) {
    scrubbing = true;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/overdubs/scrub`, {
        keep_id: keepId || null,
      });
      await loadOverdubs();
    } finally {
      scrubbing = false;
    }
  }

  // @mention autocomplete
  let mentionQuery = $state("");
  let mentionOpen = $state(false);
  let mentionIndex = $state(0);
  let mentionMatches = $derived(
    mentionQuery
      ? members.filter((m) => {
          const name = (m.user.display_name || m.user.email).toLowerCase();
          return name.includes(mentionQuery.toLowerCase());
        }).slice(0, 5)
      : []
  );

  function handleCommentInput(e: Event) {
    const input = e.target as HTMLInputElement;
    const val = input.value;
    const cursor = input.selectionStart ?? val.length;
    const before = val.slice(0, cursor);
    const match = before.match(/@([\w.\-]*)$/);
    if (match) {
      mentionQuery = match[1];
      mentionOpen = true;
      mentionIndex = 0;
    } else {
      mentionOpen = false;
    }
  }

  function handleCommentKeydown(e: KeyboardEvent) {
    if (!mentionOpen || mentionMatches.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      mentionIndex = (mentionIndex + 1) % mentionMatches.length;
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      mentionIndex = (mentionIndex - 1 + mentionMatches.length) % mentionMatches.length;
    } else if (e.key === "Tab" || e.key === "Enter") {
      if (mentionOpen && mentionMatches.length > 0) {
        e.preventDefault();
        insertMention(mentionMatches[mentionIndex]);
      }
    } else if (e.key === "Escape") {
      mentionOpen = false;
    }
  }

  function insertMention(member: Member) {
    const input = document.getElementById("comment-input") as HTMLInputElement;
    const cursor = input.selectionStart ?? newComment.length;
    const before = newComment.slice(0, cursor);
    const after = newComment.slice(cursor);
    const name = member.user.display_name || member.user.email;
    const replaced = before.replace(/@[\w.\-]*$/, `@${name} `);
    newComment = replaced + after;
    mentionOpen = false;
    input.focus();
  }

  function startEditing() {
    if (!track) return;
    editTitle = track.title;
    editDesc = track.description || "";
    editNotes = track.notes || "";
    editRecordedAt = track.recorded_at ? track.recorded_at.slice(0, 10) : "";
    editing = true;
  }

  async function saveMeta() {
    if (!track || !editTitle.trim()) return;
    savingMeta = true;
    try {
      const updated = await apiPatch<Track>(`/api/bands/${slug}/tracks/${trackId}`, {
        title: editTitle.trim(),
        description: editDesc.trim(),
        notes: editNotes,
        recorded_at: editRecordedAt || "",
      });
      track.title = updated.title;
      track.description = updated.description;
      track.notes = updated.notes;
      track.recorded_at = updated.recorded_at;
      editing = false;
    } finally {
      savingMeta = false;
    }
  }

  async function assignSet(setId: string | null) {
    if (!track) return;
    assigningSet = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/set`, {
        set_id: setId || null,
      });
      track.set_id = setId || undefined;
    } finally {
      assigningSet = false;
    }
  }

  async function addTag() {
    const tag = newTag.trim().toLowerCase();
    if (!tag || !track || track.tags.includes(tag)) { newTag = ""; return; }
    savingTags = true;
    try {
      const updated = await apiPatch<Track>(`/api/bands/${slug}/tracks/${trackId}/tags`, {
        tags: [...track.tags, tag],
      });
      track.tags = updated.tags;
      newTag = "";
    } finally {
      savingTags = false;
    }
  }

  async function removeTag(tag: string) {
    if (!track) return;
    savingTags = true;
    try {
      const updated = await apiPatch<Track>(`/api/bands/${slug}/tracks/${trackId}/tags`, {
        tags: track.tags.filter((t) => t !== tag),
      });
      track.tags = updated.tags;
    } finally {
      savingTags = false;
    }
  }

  async function pickSong(song: Song) {
    if (!track) return;
    assigningSong = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/song`, { song_id: song.id });
      track.song_id = song.id;
      track.song = song;
      songInput = "";
      songDropdownOpen = false;
    } finally {
      assigningSong = false;
    }
  }

  async function createAndAssignSong() {
    if (!track || !songInput.trim()) return;
    assigningSong = true;
    try {
      const song = await apiPost<Song>(`/api/bands/${slug}/songs`, { name: songInput.trim() });
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/song`, { song_id: song.id });
      songs = [...songs, song];
      track.song_id = song.id;
      track.song = song;
      songInput = "";
      songDropdownOpen = false;
    } finally {
      assigningSong = false;
    }
  }

  async function unassignSong() {
    if (!track) return;
    assigningSong = true;
    try {
      await apiPatch(`/api/bands/${slug}/tracks/${trackId}/song`, { song_id: null });
      track.song_id = undefined;
      track.song = undefined;
    } finally {
      assigningSong = false;
    }
  }

  async function addPersonnel() {
    if (!addPersonnelId || !track) return;
    try {
      await apiPost(`/api/bands/${slug}/tracks/${trackId}/personnel`, {
        user_id: addPersonnelId,
        role: addPersonnelRole.trim(),
      });
      personnel = await api<Personnel[]>(`/api/bands/${slug}/tracks/${trackId}/personnel`);
      addPersonnelId = "";
      addPersonnelRole = "";
    } catch {}
  }

  async function removePersonnel(userId: string) {
    if (!track) return;
    await apiDelete(`/api/bands/${slug}/tracks/${trackId}/personnel/${userId}`);
    personnel = personnel.filter((p) => p.user_id !== userId);
  }
</script>

{#if loading}
  <div class="flex items-center justify-center gap-2 py-20 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
{:else if track}
  <div>
    <!-- Breadcrumb -->
    <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5 mb-8">
      <button onclick={() => navigate(`/band/${slug}`)} class="hover:text-accent/60 transition-colors py-1">{slug.toUpperCase()}</button>
      <span>/</span>
      <span class="text-text-muted/50">T:{track.title.replace(/\s+/g, "").toUpperCase()}</span>
    </div>

    <!-- Track info -->
    <div class="mb-8">
      {#if editing}
        <div class="bg-bg-surface border border-border p-6 space-y-5">
          <div>
            <span class="label text-text-muted block mb-2">title</span>
            <input
              bind:value={editTitle}
              class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors"
            />
          </div>
          <div>
            <span class="label text-text-muted block mb-2">description</span>
            <input
              bind:value={editDesc}
              placeholder="Short description..."
              class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
            />
          </div>
          <div>
            <span class="label text-text-muted block mb-2">notes / lyrics / tabs</span>
            <textarea
              bind:value={editNotes}
              rows="8"
              placeholder="Paste lyrics, chord charts, tabs, session notes..."
              class="w-full bg-bg-primary border border-border px-4 py-3 text-sm font-mono text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors resize-y"
            ></textarea>
          </div>
          <div>
            <span class="label text-text-muted block mb-2">recording date</span>
            <input
              bind:value={editRecordedAt}
              type="date"
              class="bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors"
            />
          </div>
          <div class="flex gap-4">
            <button
              onclick={saveMeta}
              disabled={savingMeta || !editTitle.trim()}
              class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
            >
              {savingMeta ? "..." : "save"}
            </button>
            <button
              onclick={() => (editing = false)}
              class="px-6 py-3 label text-text-muted hover:text-text-secondary transition-colors"
            >
              cancel
            </button>
          </div>
        </div>
      {:else}
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-2xl font-bold tracking-wider font-display">{track.title}</h2>
            {#if track.description}
              <p class="text-base font-medium text-text-secondary mt-3">{track.description}</p>
            {/if}
          </div>
          <button
            onclick={startEditing}
            class="label text-text-muted hover:text-accent transition-colors"
          >
            edit
          </button>
        </div>

        <div class="flex items-center gap-3 mt-4 label-sm text-text-muted flex-wrap">
          <span class="font-mono">{formatDuration(track.duration_ms)}</span>
          <span class="vr-divider">/</span>
          <span>{formatFileSize(track.file_size)}</span>
          <span class="vr-divider">/</span>
          <span>{track.uploader?.display_name || track.uploader?.email}</span>
          <span class="vr-divider">/</span>
          <span>{formatRelativeTime(track.created_at)}</span>
          <span class="vr-divider">/</span>
          <a
            href={`/api/bands/${slug}/tracks/${trackId}/stream?dl=1`}
            class="text-accent hover:text-accent-hover transition-colors"
          >download</a>
        </div>

        <!-- Tags -->
        <div class="flex items-center gap-3 mt-5 flex-wrap">
          {#each track.tags as tag}
            <button
              onclick={() => removeTag(tag)}
              class="label-sm text-accent bg-accent/10 px-3 py-1 hover:bg-danger/20 hover:text-danger transition-colors"
              title="Remove tag"
            >{tag} x</button>
          {/each}
          <form onsubmit={(e) => { e.preventDefault(); addTag(); }} class="flex">
            <input
              bind:value={newTag}
              type="text"
              placeholder="+ add tag"
              disabled={savingTags}
              class="bg-transparent border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-28"
            />
          </form>
        </div>

        <!-- Song assignment -->
        <div class="flex items-center gap-3 mt-5">
          <span class="label-sm text-text-muted">song:</span>
          {#if track.song}
            <span class="label-sm text-accent bg-accent/10 px-3 py-1">{track.song.name}</span>
            <button
              onclick={unassignSong}
              disabled={assigningSong}
              class="label-sm text-text-muted hover:text-danger transition-colors"
            >x</button>
          {:else}
            <div class="relative">
              <input
                type="text"
                bind:value={songInput}
                onfocus={() => (songDropdownOpen = true)}
                onblur={() => setTimeout(() => (songDropdownOpen = false), 150)}
                placeholder="type to search or create..."
                disabled={assigningSong}
                class="bg-bg-surface border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-56"
              />
              {#if songDropdownOpen && (filteredSongs.length > 0 || songInput.trim())}
                <div class="absolute top-full left-0 mt-1 w-56 bg-bg-surface border border-border z-10 max-h-48 overflow-y-auto">
                  {#each filteredSongs as song}
                    <button
                      type="button"
                      onmousedown={() => pickSong(song)}
                      class="block w-full text-left px-3 py-2 label-sm text-text-secondary hover:bg-accent/10 hover:text-accent transition-colors"
                    >{song.name}</button>
                  {/each}
                  {#if songInput.trim() && !songs.some((s) => s.name.toLowerCase() === songInput.trim().toLowerCase())}
                    <button
                      type="button"
                      onmousedown={createAndAssignSong}
                      class="block w-full text-left px-3 py-2 label-sm text-accent hover:bg-accent/10 transition-colors border-t border-border"
                    >+ create "{songInput.trim()}"</button>
                  {/if}
                </div>
              {/if}
            </div>
          {/if}
        </div>

        <!-- Source URL -->
        {#if track.source_url}
          <div class="flex items-center gap-2 mt-4">
            <span class="label-sm text-text-muted">source:</span>
            <a
              href={track.source_url}
              target="_blank"
              rel="noopener"
              class="label-sm text-accent hover:text-accent-hover transition-colors"
            >{track.source_url}</a>
          </div>
        {/if}

        <!-- Recording date display -->
        {#if track.recorded_at}
          <div class="flex items-center gap-2 mt-4">
            <span class="label-sm text-text-muted">recorded:</span>
            <span class="label-sm text-text-secondary">{new Date(track.recorded_at.slice(0, 10) + 'T00:00:00').toLocaleDateString()}</span>
          </div>
        {/if}

        <!-- Set assignment -->
        <div class="flex items-center gap-3 mt-4">
          <span class="label-sm text-text-muted">set:</span>
          {#if track.set_id}
            {@const currentSet = allSets.find(s => s.id === track!.set_id)}
            {#if currentSet}
              <button
                onclick={() => navigate(`/band/${slug}/set/${currentSet.id}`)}
                class="label-sm text-accent bg-accent/10 px-3 py-1 hover:bg-accent/20 transition-colors"
              >{currentSet.name}</button>
            {/if}
            <button
              onclick={() => assignSet(null)}
              disabled={assigningSet}
              class="label-sm text-text-muted hover:text-danger transition-colors"
            >x</button>
          {:else}
            <select
              onchange={(e) => {
                const val = (e.target as HTMLSelectElement).value;
                if (val) assignSet(val);
              }}
              disabled={assigningSet}
              class="bg-bg-surface border border-border px-2 py-1 label-sm text-text-secondary focus:outline-none focus:border-accent transition-colors"
            >
              <option value="">assign to set...</option>
              {#each allSets as set}
                <option value={set.id}>{set.name}</option>
              {/each}
            </select>
          {/if}
        </div>

        <!-- Personnel -->
        <div class="mt-5">
          <div class="flex items-center gap-3 flex-wrap">
            <span class="label-sm text-text-muted">personnel:</span>
            {#each personnel as p}
              <span class="label-sm text-text-secondary bg-bg-surface border border-border px-3 py-1 flex items-center gap-2">
                {p.user.display_name || p.user.email}
                {#if p.role}
                  <span class="text-text-muted">/ {p.role}</span>
                {/if}
                <button
                  onclick={() => removePersonnel(p.user_id)}
                  class="text-text-muted hover:text-danger transition-colors ml-1"
                >x</button>
              </span>
            {/each}
            <form
              onsubmit={(e) => { e.preventDefault(); addPersonnel(); }}
              class="flex items-center gap-2"
            >
              <select
                bind:value={addPersonnelId}
                class="bg-bg-surface border border-border px-2 py-1 label-sm text-text-secondary focus:outline-none focus:border-accent transition-colors"
              >
                <option value="">+ add</option>
                {#each members.filter((m) => !personnel.some((p) => p.user_id === m.user.id)) as member}
                  <option value={member.user.id}>{member.user.display_name || member.user.email}</option>
                {/each}
              </select>
              {#if addPersonnelId}
                <input
                  bind:value={addPersonnelRole}
                  type="text"
                  placeholder="role (guitar, vocals...)"
                  class="bg-bg-surface border border-border px-2 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-40"
                />
                <button
                  type="submit"
                  class="label-sm text-accent hover:text-accent-hover transition-colors"
                >add</button>
              {/if}
            </form>
          </div>
        </div>
      {/if}
    </div>

    <!-- Player -->
    {#if track.status === "ready"}
      <div class="mb-10">
        <WaveformPlayer
          src={streamUrl}
          peaks={track.waveform_data}
          duration={track.duration_ms}
          comments={timedComments}
          onTimestampClick={handleTimestampClick}
        />
      </div>
    {:else if track.status === "processing"}
      <div class="bg-bg-surface border border-border p-12 text-center mb-10">
        <div class="flex items-center justify-center gap-3 text-accent">
          <div class="w-2 h-2 bg-accent animate-pulse"></div>
          <span class="label">processing</span>
        </div>
      </div>
    {:else}
      <div class="bg-bg-surface border border-border p-12 text-center mb-10 text-danger label">
        processing failed
      </div>
    {/if}

    <!-- Overdubs -->
    {#if track.status === "ready" && !track.overdub_of}
      <div class="mb-10">
        <div class="flex items-center justify-between mb-4">
          <h3 class="label text-text-secondary">overdubs ({overdubs.length})</h3>
          <button
            onclick={() => (showOverdubRecord = !showOverdubRecord)}
            class="label-sm text-text-muted hover:text-accent transition-colors flex items-center gap-1.5"
          >
            <div class="w-2 h-2 rounded-full bg-red-400/60"></div>
            {showOverdubRecord ? "cancel" : "record overdub"}
          </button>
        </div>

        {#if showOverdubRecord}
          <div class="mb-4 bg-bg-surface border border-border p-4">
            <p class="label-sm text-text-muted mb-3">play the track through headphones while recording your overdub. the offset is captured automatically.</p>
            <Recorder
              bandSlug={slug}
              onRecorded={() => { showOverdubRecord = false; loadOverdubs(); }}
              overdubParentId={trackId}
              parentStreamUrl={streamUrl}
            />
          </div>
        {/if}

        {#if overdubs.length > 0}
          <div class="space-y-3">
            {#each overdubs as od}
              {@const offsetPct = track.duration_ms > 0 ? (od.offset_ms / track.duration_ms) * 100 : 0}
              {@const widthPct = track.duration_ms > 0 ? (od.duration_ms / track.duration_ms) * 100 : 100}
              <div class="bg-bg-surface border border-border p-4">
                <div class="flex items-center justify-between gap-4 mb-3">
                  <div class="flex items-center gap-3 min-w-0">
                    <span class="text-sm font-semibold text-text-primary font-display tracking-wide truncate">{od.title}</span>
                    <span class="label-sm text-text-muted">{od.uploader?.display_name || od.uploader?.email}</span>
                    {#if od.offset_ms > 0}
                      <span class="label-sm text-text-muted font-mono">+{formatDuration(od.offset_ms)}</span>
                    {/if}
                  </div>
                  <div class="flex items-center gap-3 shrink-0">
                    <span class="label-sm font-mono text-text-muted">{formatDuration(od.duration_ms)}</span>
                    <a
                      href={`/api/bands/${slug}/tracks/${od.id}/stream?dl=1`}
                      class="label-sm text-accent hover:text-accent-hover transition-colors"
                    >dl</a>
                  </div>
                </div>

                <!-- Offset-aligned mini waveform indicator -->
                <div class="relative h-2 bg-bg-primary mb-3">
                  <div
                    class="absolute top-0 h-full bg-accent/30"
                    style="left: {offsetPct}%; width: {Math.min(widthPct, 100 - offsetPct)}%"
                  ></div>
                </div>

                <!-- Offset adjustment -->
                <div class="flex items-center gap-3 mb-3">
                  <span class="label-sm text-text-muted shrink-0">offset</span>
                  <input
                    type="range"
                    min={Math.max(0, od.offset_ms - 500)}
                    max={od.offset_ms + 500}
                    step="10"
                    value={od.offset_ms}
                    oninput={(e) => {
                      const val = parseInt((e.target as HTMLInputElement).value);
                      const o = overdubs.find((x) => x.id === od.id);
                      if (o) o.offset_ms = val;
                    }}
                    onchange={(e) => adjustOffset(od.id, parseInt((e.target as HTMLInputElement).value))}
                    class="flex-1 h-1 accent-accent cursor-pointer"
                  />
                  <span class="label-sm font-mono text-text-muted w-16 text-right">{od.offset_ms}ms</span>
                </div>

                <!-- Vote + admin controls -->
                <div class="flex items-center gap-4 flex-wrap">
                  <button
                    onclick={() => voteOverdub(od.user_voted ? null : od.id)}
                    disabled={voting}
                    class="label-sm transition-colors flex items-center gap-1.5 {od.user_voted ? 'text-accent' : 'text-text-muted hover:text-accent'}"
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill={od.user_voted ? "currentColor" : "none"} stroke="currentColor" stroke-width="2">
                      <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3H14z"/>
                      <path d="M7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3"/>
                    </svg>
                    {od.vote_count}
                  </button>

                  {#if isAdmin}
                    <button
                      onclick={() => bounceOverdub(od.id)}
                      disabled={bouncing}
                      class="label-sm text-text-muted hover:text-accent transition-colors"
                    >{bouncing ? "bouncing..." : "bounce"}</button>
                    <button
                      onclick={() => scrubOverdubs(od.id)}
                      disabled={scrubbing}
                      class="label-sm text-text-muted hover:text-red-400 transition-colors"
                    >scrub others</button>
                  {/if}
                </div>
              </div>
            {/each}

            {#if isAdmin && overdubs.length > 1}
              <button
                onclick={() => scrubOverdubs()}
                disabled={scrubbing}
                class="label-sm text-red-400/60 hover:text-red-400 transition-colors"
              >{scrubbing ? "scrubbing..." : "scrub all overdubs"}</button>
            {/if}
          </div>
        {:else if !showOverdubRecord}
          <p class="text-sm text-text-muted italic">no overdubs yet</p>
        {/if}
      </div>
    {/if}

    <!-- Notes -->
    {#if track.notes && !editing}
      <div class="mb-10">
        <h3 class="label text-text-secondary mb-4">notes</h3>
        <pre class="bg-bg-surface border border-border p-6 text-sm font-mono text-text-secondary whitespace-pre-wrap leading-relaxed">{track.notes}</pre>
      </div>
    {/if}

    <!-- Comment input -->
    <div class="mb-10">
      <form onsubmit={(e) => { e.preventDefault(); if (!mentionOpen) submitComment(); }} class="flex gap-3">
        <div class="flex-1 relative">
          {#if commentTimestamp != null}
            <button
              type="button"
              onclick={clearTimestamp}
              class="absolute left-4 top-1/2 -translate-y-1/2 label-sm text-marker bg-marker/10 px-2 py-1 font-mono hover:bg-marker/20 transition-colors"
            >
              @{Math.floor(commentTimestamp / 1000)}s x
            </button>
          {/if}
          <input
            id="comment-input"
            bind:value={newComment}
            type="text"
            placeholder={commentTimestamp != null ? "" : "add a comment... (use @ to mention)"}
            oninput={handleCommentInput}
            onkeydown={handleCommentKeydown}
            class="w-full bg-bg-surface border border-border px-5 py-4 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
            style:padding-left={commentTimestamp != null ? "6rem" : undefined}
          />
          {#if mentionOpen && mentionMatches.length > 0}
            <div class="absolute bottom-full left-0 mb-1 w-64 bg-bg-surface border border-border z-10 max-h-48 overflow-y-auto">
              {#each mentionMatches as member, i}
                <button
                  type="button"
                  onmousedown={() => insertMention(member)}
                  class="block w-full text-left px-4 py-2 label-sm transition-colors {i === mentionIndex ? 'bg-accent/10 text-accent' : 'text-text-secondary hover:bg-accent/10 hover:text-accent'}"
                >
                  {member.user.display_name || member.user.email}
                </button>
              {/each}
            </div>
          {/if}
        </div>
        <button
          type="submit"
          disabled={posting || !newComment.trim()}
          class="px-6 py-4 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
        >
          post
        </button>
      </form>
      <div class="label-sm text-text-muted mt-3">
        click comment button or alt+click waveform to attach timestamp · type @ to mention
      </div>
    </div>

    <!-- Comments -->
    <div>
      <h3 class="label text-text-secondary mb-6">
        comments ({comments.length})
      </h3>
      <CommentList
        {comments}
        {trackId}
        onSeek={handleSeek}
        onRefresh={loadComments}
      />
    </div>
  </div>
{/if}
