<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost, apiPatch, apiDelete } from "../lib/api";
  import { ws } from "../lib/ws";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, formatFileSize, formatRelativeTime } from "../lib/utils/format";
  import WaveformPlayer from "../lib/components/WaveformPlayer.svelte";
  import CommentList from "../lib/components/CommentList.svelte";

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
      await loadComments();
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
      <button onclick={() => navigate("/")} class="hover:text-accent/60 transition-colors py-1">JRB</button>
      <span>/</span>
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
