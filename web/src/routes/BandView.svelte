<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost, apiPatch, apiDelete } from "../lib/api";
  import { markBandSeen } from "../lib/stores/notifications";
  import { ws } from "../lib/ws";
  import { user as currentUser, isDemo } from "../lib/stores/auth";
  import { navigate, route, type BandTab } from "../lib/stores/router";
  import { loadBands } from "../lib/stores/bands";
  import { colorSchemes, applyColorScheme } from "../lib/colorSchemes";
  import TrackCard from "../lib/components/TrackCard.svelte";
  import MiniPlayer from "../lib/components/MiniPlayer.svelte";
  import TrackUpload from "../lib/components/TrackUpload.svelte";
  import Recorder from "../lib/components/Recorder.svelte";
  import { setTypeCode, setTypeLabel, formatRelativeTime, formatDuration, formatFileSize } from "../lib/utils/format";
  import { uploadFile } from "../lib/api";

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
  interface ActivityItem {
    type: "track" | "overdub" | "comment" | "song";
    actor_name: string;
    subject: string;
    link_id: string;
    created_at: string;
    peaks?: number[];
    duration_ms?: number;
    preview?: string;
    timestamp_ms?: number;
    stream_id?: string;
    is_new: boolean;
  }

  interface FeedGroup {
    key: string;
    type: ActivityItem["type"];
    actor_name: string;
    subject: string;
    link_id: string;
    latest_at: string;
    has_new: boolean;
    items: ActivityItem[];
  }

  function groupFeedItems(items: ActivityItem[]): FeedGroup[] {
    const groups: FeedGroup[] = [];
    for (const item of items) {
      const key = `${item.type}|${item.actor_name}|${item.subject}`;
      const last = groups[groups.length - 1];
      if (last && last.key === key) {
        last.items.push(item);
        if (item.is_new) last.has_new = true;
      } else {
        groups.push({ key, type: item.type, actor_name: item.actor_name, subject: item.subject, link_id: item.link_id, latest_at: item.created_at, has_new: item.is_new, items: [item] });
      }
    }
    return groups;
  }

  interface BandFile {
    id: string;
    name: string;
    file_size: number;
    content_type: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
  }

  let loading = $state(true);
  let feedItems = $state<ActivityItem[]>([]);
  let expandedGroups = $state(new Set<string>());
  // The active tab lives in the URL, so a tab is linkable and the back button
  // steps through tabs instead of leaving the band entirely.
  let viewTab = $derived($route.bandTab ?? "feed");

  function selectTab(tab: BandTab) {
    navigate(tab === "feed" ? `/band/${slug}` : `/band/${slug}/${tab}`);
  }
  let files = $state<BandFile[]>([]);
  let filesLoading = $state(false);
  let filesLoaded = $state(false);
  let fileUploadProgress = $state(0);
  let fileUploading = $state(false);
  let fileUploadError = $state("");

  let feedGroups = $derived(groupFeedItems(feedItems));

  function feedCode(type: ActivityItem["type"]) {
    switch (type) {
      case "track": return "TRK";
      case "overdub": return "ODB";
      case "comment": return "CMT";
      case "song": return "SNG";
      default: return "---";
    }
  }

  function toggleGroup(key: string) {
    const next = new Set(expandedGroups);
    next.has(key) ? next.delete(key) : next.add(key);
    expandedGroups = next;
  }
  let showInviteUrl = $state("");
  let generatingInvite = $state(false);
  let inviteEmail = $state("");
  let showInviteForm = $state(false);
  let newSongName = $state("");
  let creatingSong = $state(false);

  // All unique tags across all tracks, with their associated takes
  let taggedTracks = $derived(
    (() => {
      const m = new Map<string, Track[]>();
      for (const t of tracks) {
        if (!t.tags?.length) continue;
        for (const tag of t.tags) {
          if (!m.has(tag)) m.set(tag, []);
          m.get(tag)!.push(t);
        }
      }
      return new Map([...m.entries()].sort(([a], [b]) => a.localeCompare(b)));
    })()
  );
  let newSetName = $state("");
  let newSetType = $state("rehearsal");
  let creatingSet = $state(false);
  let showImport = $state(false);
  let importUrl = $state("");
  let importTitle = $state("");
  let importing = $state(false);

  // Band management
  let showSettings = $state(false);

  // One-line installer for the sync utility. curl doesn't quarantine what it
  // downloads, so on a Mac this skips the chmod/xattr dance a browser download needs.
  const installCommand = `curl -fsSL ${window.location.origin}/install.sh | sh`;
  let installCopied = $state(false);

  async function copyInstallCommand() {
    try {
      await navigator.clipboard.writeText(installCommand);
      installCopied = true;
      setTimeout(() => (installCopied = false), 2000);
    } catch {
      // Clipboard can be unavailable (non-secure context); the command is still selectable.
    }
  }
  let editBandName = $state("");
  let savingBandName = $state(false);
  let addMemberEmail = $state("");
  let addingMember = $state(false);
  let addMemberError = $state("");
  let membersExpanded = $state(false);

  let ungroupedTracks = $derived(tracks.filter((t) => !t.song_id && !t.set_id && !t.bounced_to && !t.overdub_of));
  let activeTabCode = $derived(
    viewTab === "feed" ? "ACT" :
    viewTab === "songs" ? "SNG" :
    viewTab === "sets" ? "SET" :
    viewTab === "tags" ? "TAG" :
    "FIL"
  );
  let activeTabLabel = $derived(
    viewTab === "feed" ? "activity" :
    viewTab === "songs" ? "songs" :
    viewTab === "sets" ? "sets" :
    viewTab === "tags" ? "tags" :
    "files"
  );
  let activeTabCount = $derived(
    viewTab === "feed" ? feedGroups.length :
    viewTab === "songs" ? songs.length :
    viewTab === "sets" ? sets.length :
    viewTab === "tags" ? taggedTracks.size :
    files.length
  );

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
      const [bandRes, tracksRes, songsRes, setsRes, feedRes] = await Promise.allSettled([
        api<BandDetail>(`/api/bands/${slug}`),
        api<Track[]>(`/api/bands/${slug}/tracks`),
        api<Song[]>(`/api/bands/${slug}/songs`),
        api<SetSummary[]>(`/api/bands/${slug}/sets`),
        api<ActivityItem[]>(`/api/bands/${slug}/activity`),
      ]);
      if (bandRes.status === "fulfilled") band = bandRes.value;
      if (tracksRes.status === "fulfilled") tracks = tracksRes.value;
      if (songsRes.status === "fulfilled") songs = songsRes.value;
      if (setsRes.status === "fulfilled") sets = setsRes.value;
      if (feedRes.status === "fulfilled") feedItems = feedRes.value;
      markBandSeen(slug);
    } finally {
      loading = false;
    }
  }

  async function loadTracks() {
    tracks = await api<Track[]>(`/api/bands/${slug}/tracks`);
  }

  async function loadFiles() {
    filesLoading = true;
    try {
      files = await api<BandFile[]>(`/api/bands/${slug}/files`);
      filesLoaded = true;
    } finally {
      filesLoading = false;
    }
  }

  // Files are the one tab not fetched up front, so landing directly on
  // /band/:slug/files has to trigger the load itself.
  $effect(() => {
    if (viewTab === "files" && !filesLoaded && !filesLoading) loadFiles();
  });

  // Files larger than this threshold use the multipart path (browser → S3 direct).
  // Smaller files still go through the regular POST endpoint.
  const MULTIPART_THRESHOLD = 100 * 1024 * 1024; // 100 MB
  const CHUNK_SIZE = 25 * 1024 * 1024; // 25 MB per part

  // PUT a chunk directly to a presigned S3 URL via XHR so we get upload progress
  // events within the chunk. Returns the ETag S3 puts on the part.
  function putChunk(
    url: string,
    chunk: Blob,
    onProgress: (loaded: number) => void,
    attempt = 0,
  ): Promise<string> {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("PUT", url);
      xhr.upload.onprogress = (e) => { if (e.lengthComputable) onProgress(e.loaded); };
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          const etag = xhr.getResponseHeader("ETag") ?? xhr.getResponseHeader("etag") ?? "";
          if (!etag) { reject(new Error("S3 did not return an ETag — check bucket CORS ExposeHeaders")); return; }
          resolve(etag);
        } else if (attempt < 3) {
          putChunk(url, chunk, onProgress, attempt + 1).then(resolve, reject);
        } else {
          reject(new Error(`Part upload failed (HTTP ${xhr.status})`));
        }
      };
      xhr.onerror = () => {
        if (attempt < 3) putChunk(url, chunk, onProgress, attempt + 1).then(resolve, reject);
        else reject(new Error("Network error uploading part"));
      };
      xhr.send(chunk);
    });
  }

  async function uploadBandFile(file: File) {
    fileUploading = true;
    fileUploadError = "";
    fileUploadProgress = 0;

    if (file.size <= MULTIPART_THRESHOLD) {
      // Small file: use the existing single-request upload path.
      try {
        const created = await uploadFile<BandFile>(
          `/api/bands/${slug}/files`,
          file,
          {},
          (pct) => (fileUploadProgress = pct),
        );
        files = [created, ...files];
      } catch (e: any) {
        fileUploadError = e.message || "Upload failed";
      } finally {
        fileUploading = false;
        fileUploadProgress = 0;
      }
      return;
    }

    // Large file: chunk directly to S3 via presigned multipart URLs.
    // State is persisted to localStorage so uploads survive page refreshes.
    const CONCURRENCY = 4;
    const stateKey = `jerboa:upload:${slug}:${file.name}:${file.size}`;

    const saveState = (id: string, done: { part_number: number; etag: string }[]) => {
      try { localStorage.setItem(stateKey, JSON.stringify({ fileId: id, parts: done })); } catch {}
    };
    const clearState = () => { try { localStorage.removeItem(stateKey); } catch {} };

    let fileId = "";
    try {
      const totalChunks = Math.ceil(file.size / CHUNK_SIZE);
      const parts: ({ part_number: number; etag: string } | undefined)[] = new Array(totalChunks);
      const chunkLoaded = new Float64Array(totalChunks);

      // Try to resume a previous upload for this exact file.
      let resumedCount = 0;
      const saved = (() => { try { const s = localStorage.getItem(stateKey); return s ? JSON.parse(s) : null; } catch { return null; } })();
      if (saved?.fileId) {
        try {
          // Ask S3 (via server) which parts it actually has — don't trust client state alone.
          const { parts: doneParts } = await api<{ parts: { part_number: number; etag: string }[] }>(
            `/api/bands/${slug}/files/multipart/parts?file_id=${saved.fileId}`,
          );
          fileId = saved.fileId;
          for (const p of doneParts) {
            const i = p.part_number - 1;
            parts[i] = p;
            chunkLoaded[i] = file.slice(i * CHUNK_SIZE, Math.min((i + 1) * CHUNK_SIZE, file.size)).size;
            resumedCount++;
          }
        } catch {
          // Upload no longer valid (expired, aborted, or already complete) — start fresh.
          clearState();
        }
      }

      if (!fileId) {
        const initiated = await apiPost<{ file_id: string }>(
          `/api/bands/${slug}/files/multipart/initiate`,
          { name: file.name, content_type: file.type || "application/octet-stream", file_size: file.size },
        );
        fileId = initiated.file_id;
        saveState(fileId, []);
      }

      const updateProgress = () => {
        let total = 0;
        for (let j = 0; j < totalChunks; j++) total += chunkLoaded[j];
        fileUploadProgress = Math.round((total / file.size) * 100);
      };
      if (resumedCount > 0) updateProgress();

      const urlCache = new Map<number, Promise<string>>();
      const prefetch = (i: number) => {
        if (i < totalChunks && !parts[i] && !urlCache.has(i)) {
          urlCache.set(i, api<{ url: string }>(
            `/api/bands/${slug}/files/multipart/part?file_id=${fileId}&part=${i + 1}`,
          ).then(r => r.url));
        }
      };
      // Seed the first batch of URLs, skipping already-done chunks.
      let seeded = 0;
      for (let i = 0; i < totalChunks && seeded < CONCURRENCY * 2; i++) {
        if (!parts[i]) { prefetch(i); seeded++; }
      }

      let nextIndex = 0;
      const worker = async () => {
        while (nextIndex < totalChunks) {
          const i = nextIndex++;
          if (parts[i]) continue; // already uploaded, skip

          prefetch(i + CONCURRENCY);
          const start = i * CHUNK_SIZE;
          const chunk = file.slice(start, Math.min(start + CHUNK_SIZE, file.size));
          const url = await (urlCache.get(i) ?? api<{ url: string }>(
            `/api/bands/${slug}/files/multipart/part?file_id=${fileId}&part=${i + 1}`,
          ).then(r => r.url));

          const etag = await putChunk(url, chunk, (loaded) => {
            chunkLoaded[i] = loaded;
            updateProgress();
          });
          chunkLoaded[i] = chunk.size;
          parts[i] = { part_number: i + 1, etag };
          updateProgress();
          saveState(fileId, parts.filter(Boolean) as { part_number: number; etag: string }[]);
        }
      };

      await Promise.all(Array.from({ length: CONCURRENCY }, worker));

      const created = await apiPost<BandFile>(
        `/api/bands/${slug}/files/multipart/complete`,
        { file_id: fileId, parts: parts as { part_number: number; etag: string }[] },
      );
      clearState();
      files = [created, ...files];
    } catch (e: any) {
      fileUploadError = e.message || "Upload failed";
      // Don't clear localStorage on error — leave state so the user can resume.
      // Only abort the S3 upload if the user explicitly deletes the pending file.
    } finally {
      fileUploading = false;
      fileUploadProgress = 0;
    }
  }

  async function deleteBandFile(id: string) {
    await apiDelete(`/api/bands/${slug}/files/${id}`);
    files = files.filter((f) => f.id !== id);
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
    <div class="mb-2 sys-title-block">
      <div class="sys-kicker mb-3">band channel // live workspace</div>
      <div class="flex items-center gap-3 mb-1">
        <h2 class="text-3xl md:text-[3.4rem] leading-none font-bold tracking-[0.05em] font-display uppercase">{band.band.name}</h2>
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
        </div>
      </div>

      <div class="flex items-center gap-2 flex-wrap mb-3">
        {#each band.members as member, i}
          <span class="sys-chip inline-flex items-center gap-1.5 label-sm {i >= 4 && !membersExpanded ? 'hidden md:inline-flex' : ''}">
            {member.user.display_name || member.user.email}
            {#if member.user.id === $currentUser?.id}
              <span class="sys-code text-accent">YOU</span>
            {/if}
          </span>
        {/each}
        {#if !membersExpanded && band.members.length > 4}
          <button
            class="sys-chip inline-flex items-center label-sm text-text-muted/70 hover:text-text-primary transition-colors md:hidden"
            onclick={() => (membersExpanded = true)}
          >+{band.members.length - 4} more</button>
        {/if}
        {#each band.pending_invites ?? [] as invite}
          <span class="sys-chip inline-flex items-center gap-1.5 label-sm border-dashed text-text-muted/60">
            {invite.email}
            <span class="sys-code text-text-muted/35">invited</span>
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
            disabled={generatingInvite || !inviteEmail.trim() || $isDemo}
            title={$isDemo ? "demo account is read-only" : ""}
            class="px-5 py-2.5 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors whitespace-nowrap"
          >
            {$isDemo ? "demo: read-only" : generatingInvite ? "..." : "send invite"}
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
    {#if showSettings && band}
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
              disabled={savingBandName || !editBandName.trim() || editBandName.trim() === band.band.name || $isDemo}
              title={$isDemo ? "demo account is read-only" : ""}
              class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors shrink-0"
            >
              {$isDemo ? "demo" : savingBandName ? "..." : "save"}
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
              disabled={addingMember || !addMemberEmail.trim() || $isDemo}
              title={$isDemo ? "demo account is read-only" : ""}
              class="px-4 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors shrink-0"
            >
              {$isDemo ? "demo" : addingMember ? "..." : "add"}
            </button>
          </form>
          {#if addMemberError}
            <div class="label-sm text-danger mt-2">{addMemberError}</div>
          {/if}
        </div>

        <div>
          <span class="label-sm md:label text-text-muted block mb-1">daw sync</span>
          <p class="label-sm text-text-muted/50 mb-3">back up garageband projects and sync reaper sessions straight to jerboa — no zipping</p>

          <span class="label-sm text-text-muted block mb-1">mac</span>
          <p class="label-sm text-text-muted/50 mb-2">paste into terminal. it installs, connects to your band, and adds a jerboa sync app to drop projects onto</p>
          <div class="flex items-stretch gap-2 mb-4">
            <!-- Not label-sm: that uppercases, and this has to read exactly as typed. -->
            <code class="flex-1 min-w-0 overflow-x-auto whitespace-nowrap px-3 py-1.5 border border-border bg-bg-surface font-mono text-xs font-semibold text-text-secondary">{installCommand}</code>
            <button
              onclick={copyInstallCommand}
              class="px-3 py-1.5 border border-border hover:border-accent/60 label-sm text-text-muted hover:text-accent transition-colors shrink-0"
            >{installCopied ? "copied" : "copy"}</button>
          </div>

          <span class="label-sm text-text-muted block mb-1">windows / linux</span>
          <p class="label-sm text-text-muted/50 mb-2">drop the utility in your reaper project folder and run it</p>
          <div class="flex gap-2 flex-wrap">
            <a
              href="/api/bands/{slug}/sync/binary?platform=windows"
              class="px-3 py-1.5 border border-border hover:border-accent/60 label-sm text-text-muted hover:text-accent transition-colors"
            >windows</a>
            <a
              href="/api/bands/{slug}/sync/binary?platform=linux"
              class="px-3 py-1.5 border border-border hover:border-accent/60 label-sm text-text-muted hover:text-accent transition-colors"
            >linux</a>
          </div>
        </div>

        <button
          onclick={() => (showSettings = false)}
          class="label text-text-muted hover:text-text-secondary transition-colors"
        >close</button>
      </div>
    {/if}

    <!-- Add material slab -->
    <div class="sys-panel mb-4 mt-6">
      <div class="px-3 py-2 border-b border-border/25">
        <span class="sys-kicker">MAT // add material</span>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-px bg-border/30">
      <div class="bg-bg-primary"><TrackUpload bandSlug={slug} onUploaded={loadTracks} /></div>
      <div class="bg-bg-primary"><Recorder bandSlug={slug} onRecorded={loadTracks} /></div>

      <!-- Import URL -->
      {#if showImport}
        <form
          onsubmit={(e) => { e.preventDefault(); importFromUrl(); }}
          class="bg-accent/5 p-3 col-span-full border-t border-border/40"
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
              disabled={importing || !importUrl.trim() || $isDemo}
              title={$isDemo ? "demo account is read-only" : ""}
              class="px-4 py-2.5 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label-sm transition-colors"
            >
              {$isDemo ? "demo" : importing ? "..." : "import"}
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
        <div class="bg-bg-primary">
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="border-dashed border border-border/50 hover:border-accent/40 hover:bg-accent/[0.02] p-3 md:py-2 text-center transition-colors cursor-pointer flex items-center justify-center gap-2 whitespace-nowrap h-full"
            onclick={() => (showImport = true)}
          >
            <svg class="text-text-muted shrink-0" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
              <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
            </svg>
            <span class="text-sm text-text-muted font-semibold">import url</span>
            <span class="label-sm text-text-muted/30 font-mono hidden md:inline translate-y-px">youtube, etc</span>
          </div>
        </div>
      {/if}
      </div><!-- /grid -->
    </div><!-- /add material slab -->

    <!-- Desktop rail + content grid -->
    <div class="lg:grid lg:grid-cols-[1fr_160px] lg:gap-6 lg:items-start">
    <div><!-- main column start -->

    <!-- Tab toggle — sticky on mobile -->
    <div class="sticky top-0 z-20 bg-bg-primary -mx-5 px-5 md:mx-0 md:px-0 border-b border-border/40 mb-2">
      <div class="flex items-center gap-5 py-2.5">
        <button
          onclick={() => selectTab("feed")}
          class="label transition-colors {viewTab === 'feed' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
        >activity</button>
        <button
          onclick={() => selectTab("songs")}
          class="label transition-colors {viewTab === 'songs' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
        >songs</button>
        <button
          onclick={() => selectTab("sets")}
          class="label transition-colors {viewTab === 'sets' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
        >sets</button>
        {#if taggedTracks.size > 0}
          <button
            onclick={() => selectTab("tags")}
            class="label transition-colors {viewTab === 'tags' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
          >tags</button>
        {/if}
        <button
          onclick={() => selectTab("files")}
          class="label transition-colors {viewTab === 'files' ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
        >files</button>
      </div>
    </div>

    <!-- Creation form — separate row so it never competes with the tabs on mobile -->
    {#if viewTab === "songs"}
      <form onsubmit={(e) => { e.preventDefault(); createSong(); }} class="flex gap-2 mb-2">
        <input
          bind:value={newSongName}
          type="text"
          placeholder={$isDemo ? "demo: read-only" : "+ new song"}
          disabled={creatingSong || $isDemo}
          title={$isDemo ? "demo account is read-only" : ""}
          class="bg-transparent border border-accent/30 px-3 py-1 label-sm text-text-secondary placeholder:text-accent/70 focus:outline-none focus:border-accent focus:bg-accent/[0.03] transition-colors w-40 disabled:opacity-50"
        />
      </form>
    {:else if viewTab === "sets"}
      <form onsubmit={(e) => { e.preventDefault(); createSet(); }} class="flex gap-2 mb-2">
        <select
          bind:value={newSetType}
          disabled={$isDemo}
          class="bg-transparent border border-border px-2 py-1 label-sm text-text-secondary focus:outline-none focus:border-accent transition-colors disabled:opacity-50"
        >
          <option value="rehearsal">rehearsal</option>
          <option value="live">live</option>
          <option value="pre-production">pre-production</option>
          <option value="other">other</option>
        </select>
        <input
          bind:value={newSetName}
          type="text"
          placeholder={$isDemo ? "demo: read-only" : "+ new set"}
          disabled={creatingSet || $isDemo}
          title={$isDemo ? "demo account is read-only" : ""}
          class="bg-transparent border border-border px-3 py-1 label-sm text-text-secondary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors w-40 disabled:opacity-50"
        />
      </form>
    {/if}

    {#if viewTab === "feed"}
      {#if feedGroups.length === 0}
        <div class="sys-empty mb-4">
          <div class="sys-empty-code">ACT // 00</div>
          <div class="sys-empty-title text-text-primary mt-3">No Activity</div>
          <p class="sys-empty-copy mt-3">Fresh takes, comments, and overdubs will land here as soon as the band starts moving.</p>
        </div>
      {:else}
        <div class="space-y-px mb-4">
          {#each feedGroups as group (group.key + group.latest_at)}
            {@const n = group.items.length}
            {@const expanded = expandedGroups.has(group.key + group.latest_at)}
            {@const solo = n === 1}
            {@const item = group.items[0]}

            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="sys-panel border-l-2 border-l-transparent transition-all {solo && item.link_id ? 'hover:border-l-accent hover:border-accent/40 group cursor-pointer' : !solo ? 'hover:border-accent/20' : ''}">

              <!-- Header row -->
              <div
                class="flex items-start gap-3 pl-3 pr-4 py-3 {!solo ? 'cursor-pointer' : ''}"
                onclick={() => solo ? (item.link_id && navigate(`/band/${slug}/track/${item.link_id}`)) : toggleGroup(group.key + group.latest_at)}
              >
                <!-- Event code gutter -->
                <div class="shrink-0 flex w-10 self-stretch items-center justify-center">
                  <span class="sys-code text-center {group.has_new ? 'text-accent/90' : 'text-text-muted/45'}">{feedCode(group.type)}</span>
                </div>

                <!-- Summary text -->
                <div class="flex-1 min-w-0 flex items-center justify-between gap-3">
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium leading-snug">
                      {#if group.type === "track"}
                        <span class="font-bold text-text-primary">{group.actor_name}</span>
                        <span class="text-text-secondary"> {solo ? "recorded a new take for" : `recorded ${n} takes for`} </span>
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <span class="text-accent hover:underline cursor-pointer" onclick={(e) => { e.stopPropagation(); navigate(`/band/${slug}/track/${group.link_id}`); }}>{group.subject}</span>
                      {:else if group.type === "overdub"}
                        <span class="font-bold text-text-primary">{group.actor_name}</span>
                        <span class="text-text-secondary"> {solo ? "added an overdub to" : `added ${n} overdubs to`} </span>
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <span class="text-accent hover:underline cursor-pointer" onclick={(e) => { e.stopPropagation(); navigate(`/band/${slug}/track/${group.link_id}`); }}>{group.subject}</span>
                      {:else if group.type === "comment"}
                        <span class="font-bold text-text-primary">{group.actor_name}</span>
                        <span class="text-text-secondary"> {solo ? "left a comment on" : `left ${n} comments on`} </span>
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <span class="text-accent hover:underline cursor-pointer" onclick={(e) => { e.stopPropagation(); navigate(`/band/${slug}/track/${group.link_id}`); }}>{group.subject}</span>
                      {:else if group.type === "song"}
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <span class="text-accent hover:underline cursor-pointer" onclick={(e) => { e.stopPropagation(); navigate(`/band/${slug}/song/${group.link_id}`); }}>{group.subject}</span>
                        <span class="text-text-secondary"> added to songs</span>
                      {/if}
                    </p>

                    <!-- Metadata line -->
                    <p class="mt-0.5 label-sm text-text-muted/45">
                      {formatRelativeTime(group.latest_at)}{#if solo && item.type === "comment" && item.timestamp_ms != null} · at {formatDuration(item.timestamp_ms)}{/if}
                    </p>

                    <!-- Solo item preview -->
                    {#if solo}
                      {#if (item.type === "track" || item.type === "overdub") && (item.stream_id || item.link_id)}
                        <MiniPlayer
                          src={`/api/bands/${slug}/tracks/${item.stream_id || item.link_id}/stream`}
                          peaks={item.peaks ?? []}
                          duration_ms={item.duration_ms ?? 0}
                        />
                      {:else if item.type === "comment" && item.preview}
                        <p class="mt-1.5 text-sm text-text-muted italic border-l-2 border-border pl-2 leading-snug">"{item.preview}"</p>
                      {/if}
                    {/if}
                  </div>

                  {#if !solo}
                    <div class="flex items-center gap-2 shrink-0 self-center">
                      <span class="label-sm text-text-muted/50 bg-bg-elevated px-1.5 py-0.5 tabular-nums min-w-5 h-5 inline-flex items-center justify-center">{n}</span>
                      <span class="w-4 h-4 inline-flex items-center justify-center">
                        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="text-text-muted/50 transition-transform {expanded ? 'rotate-180' : ''}">
                          <polyline points="6 9 12 15 18 9"/>
                        </svg>
                      </span>
                    </div>
                  {/if}
                </div>
              </div>

              <!-- Expanded items -->
              {#if !solo && expanded}
                <div class="border-t border-border/30 divide-y divide-border/20">
                  {#each group.items as item}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <!-- svelte-ignore a11y_no_static_element_interactions -->
                    <div
                      class="pl-16 pr-4 py-2.5 {item.link_id ? 'cursor-pointer hover:bg-bg-elevated/30 group/item' : ''}"
                      onclick={() => item.link_id && navigate(`/band/${slug}/track/${item.link_id}`)}
                    >
                      <div class="flex items-center justify-between gap-2 mb-0.5">
                        <span class="label-sm text-text-muted/50">{formatRelativeTime(item.created_at)}</span>
                        {#if item.type === "comment" && item.timestamp_ms != null}
                          <span class="label-sm text-text-muted/40">at {formatDuration(item.timestamp_ms)}</span>
                        {/if}
                      </div>
                      {#if (item.type === "track" || item.type === "overdub") && (item.stream_id || item.link_id)}
                        <MiniPlayer
                          src={`/api/bands/${slug}/tracks/${item.stream_id || item.link_id}/stream`}
                          peaks={item.peaks ?? []}
                          duration_ms={item.duration_ms ?? 0}
                        />
                      {:else if item.type === "comment" && item.preview}
                        <p class="text-sm text-text-muted italic border-l-2 border-border pl-2 leading-snug">"{item.preview}"</p>
                      {/if}
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    {:else if viewTab === "songs"}
      {#if songs.length > 0}
        <div class="space-y-1 mb-4">
          {#each songs as song}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="sys-panel border-l-2 border-l-transparent hover:border-l-accent hover:border-accent/40 px-4 py-3 transition-all cursor-pointer group flex items-center justify-between"
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
      {:else}
        <div class="sys-empty mb-4">
          <div class="sys-empty-code">SNG // 00</div>
          <div class="sys-empty-title text-text-primary mt-3">No Songs</div>
          <p class="sys-empty-copy mt-3">Start a song document here, then attach takes, notes, lyrics, and set usage around it.</p>
        </div>
      {/if}
    {:else if viewTab === "sets"}
      {#if sets.length > 0}
        <div class="space-y-1 mb-4">
          {#each sets as set}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="sys-panel px-4 py-3 hover:border-accent/40 transition-colors cursor-pointer group flex items-center justify-between"
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
        <div class="sys-empty mb-4">
          <div class="sys-empty-code">SET // 00</div>
          <div class="sys-empty-title text-text-primary mt-3">No Sets</div>
          <p class="sys-empty-copy mt-3">Build rehearsal runs, live sequences, or pre-production lists here, then tag them against recordings.</p>
        </div>
      {/if}
    {/if}

    {#if viewTab === "files"}
      <!-- Upload area -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <label
        class="block border border-dashed border-border hover:border-accent/60 transition-colors px-4 py-5 mb-3 cursor-pointer text-center"
        ondragover={(e) => e.preventDefault()}
        ondrop={(e) => { e.preventDefault(); const f = e.dataTransfer?.files[0]; if (f) uploadBandFile(f); }}
      >
        <input
          type="file"
          class="sr-only"
          disabled={fileUploading}
          onchange={(e) => { const f = (e.currentTarget as HTMLInputElement).files?.[0]; if (f) uploadBandFile(f); (e.currentTarget as HTMLInputElement).value = ""; }}
        />
        {#if fileUploading}
          <div class="flex flex-col items-center gap-2">
            <div class="w-32 h-1 bg-border rounded-full overflow-hidden">
              <div class="h-full bg-accent transition-all" style="width: {fileUploadProgress}%"></div>
            </div>
            <span class="label-sm text-text-muted">{fileUploadProgress}%</span>
          </div>
        {:else}
          <span class="label-sm text-text-muted">drop a file or click to upload</span>
        {/if}
      </label>
      {#if fileUploadError}
        <p class="label-sm text-red-400 mb-3">{fileUploadError}</p>
      {/if}
      <p class="label-sm text-text-muted mb-3">
        project too big to zip? don't — sync it file-by-file with the
        <button
          onclick={() => (showSettings = true)}
          class="text-accent hover:text-accent/70 transition-colors underline underline-offset-2"
        >daw sync utility</button>
      </p>

      {#if filesLoading}
        <div class="text-center py-8 label text-text-muted">loading...</div>
      {:else if files.length === 0}
        <div class="sys-empty mb-4">
          <div class="sys-empty-code">FIL // 00</div>
          <div class="sys-empty-title text-text-primary mt-3">No Files</div>
          <p class="sys-empty-copy mt-3">Drop stems, charts, reference docs, or session exports here so the band always has a clean shared stash.</p>
        </div>
      {:else}
        <div class="space-y-1 mb-4">
          {#each files as file}
            <div class="bg-bg-surface border border-border px-4 py-3 flex items-center justify-between gap-4">
              <div class="min-w-0 flex-1">
                <p class="text-sm font-semibold text-text-primary truncate">{file.name}</p>
                <p class="label-sm text-text-muted/60 mt-0.5">
                  {formatFileSize(file.file_size)}
                  {#if file.uploader}· {file.uploader.display_name}{/if}
                  · {formatRelativeTime(file.created_at)}
                </p>
              </div>
              <div class="flex items-center gap-3 shrink-0">
                <a
                  href="/api/bands/{slug}/files/{file.id}/download"
                  class="label-sm text-accent hover:text-accent/70 transition-colors"
                  onclick={(e) => e.stopPropagation()}
                >download</a>
                <button
                  onclick={() => deleteBandFile(file.id)}
                  class="label-sm text-text-muted/40 hover:text-red-400 transition-colors"
                >delete</button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    {/if}

    {#if viewTab === "tags"}
      {#if taggedTracks.size > 0}
        <div class="space-y-1 mb-4">
          {#each [...taggedTracks.entries()] as [tag, tagTracks]}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="sys-panel border-l-2 border-l-transparent hover:border-l-accent hover:border-accent/40 px-4 py-3 transition-all cursor-pointer group flex items-center justify-between"
              onclick={() => navigate(`/band/${slug}/tag/${encodeURIComponent(tag)}`)}
            >
              <h4 class="text-base font-semibold tracking-wider text-text-primary font-display group-hover:text-accent transition-colors">{tag}</h4>
              <div class="flex items-center gap-3">
                <span class="label-sm text-text-muted group-hover:text-accent transition-colors">{tagTracks.length} {tagTracks.length === 1 ? 'take' : 'takes'}</span>
                <span class="label-sm text-text-muted/0 group-hover:text-accent transition-colors">&rsaquo;</span>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="sys-empty mb-4">
          <div class="sys-empty-code">TAG // 00</div>
          <div class="sys-empty-title text-text-primary mt-3">No Tags</div>
          <p class="sys-empty-copy mt-3">Tags become useful once takes start piling up. Use them to sort moods, revisions, instrumentation, or decisions.</p>
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

    </div><!-- /main column -->

    <!-- Desktop context rail -->
    <aside class="hidden lg:block">
      <div class="sticky top-6 space-y-3">
        <div class="sys-panel relative overflow-hidden p-3 pr-10">
          <span class="sys-edge-label">OVR</span>
          <div class="sys-code text-text-muted/45">{activeTabCode} // {activeTabLabel}</div>
          <div class="sys-stat-value text-accent mt-2">{String(activeTabCount).padStart(2, "0")}</div>
          <div class="label-sm text-text-muted/55 mt-1 mb-3">{activeTabLabel} visible</div>
          <div class="sys-kicker mb-2.5">OVR // overview</div>
          <div class="space-y-1.5">
            <div class="flex items-center justify-between">
              <span class="label-sm text-text-muted">songs</span>
              <span class="label-sm text-text-primary font-bold tabular-nums">{songs.length}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="label-sm text-text-muted">sets</span>
              <span class="label-sm text-text-primary font-bold tabular-nums">{sets.length}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="label-sm text-text-muted">members</span>
              <span class="label-sm text-text-primary font-bold tabular-nums">{band.members.length}</span>
            </div>
            {#if tracks.length > 0}
              <div class="flex items-center justify-between">
                <span class="label-sm text-text-muted">takes</span>
                <span class="label-sm text-text-primary font-bold tabular-nums">{tracks.length}</span>
              </div>
            {/if}
          </div>
        </div>
      </div>
    </aside>

    </div><!-- /rail grid -->
  </div>
{/if}
