<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost } from "../lib/api";
  import { ws } from "../lib/ws";
  import { navigate } from "../lib/stores/router";
  import { formatDuration, formatFileSize, formatRelativeTime } from "../lib/utils/format";
  import WaveformPlayer from "../lib/components/WaveformPlayer.svelte";
  import CommentList from "../lib/components/CommentList.svelte";

  let { slug, trackId }: { slug: string; trackId: string } = $props();

  interface Track {
    id: string;
    band_id: string;
    title: string;
    description?: string;
    waveform_data?: number[];
    duration_ms: number;
    format: string;
    sample_rate: number;
    file_size: number;
    status: string;
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

  let track = $state<Track | null>(null);
  let comments = $state<Comment[]>([]);
  let loading = $state(true);
  let newComment = $state("");
  let commentTimestamp = $state<number | null>(null);
  let posting = $state(false);

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
      track = await api<Track>(`/api/bands/${slug}/tracks/${trackId}`);
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
</script>

{#if loading}
  <div class="text-center py-16 text-text-muted text-xs tracking-[0.3em] uppercase">loading</div>
{:else if track}
  <div>
    <!-- Back -->
    <button
      onclick={() => navigate(`/band/${slug}`)}
      class="text-xs tracking-[0.2em] uppercase text-text-muted hover:text-text-secondary transition-colors mb-6 flex items-center gap-2"
    >
      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <polyline points="15 18 9 12 15 6"/>
      </svg>
      back
    </button>

    <!-- Track info -->
    <div class="mb-6">
      <h2 class="text-xl font-bold tracking-wider">{track.title}</h2>
      {#if track.description}
        <p class="text-sm text-text-secondary mt-2">{track.description}</p>
      {/if}
      <div class="flex items-center gap-3 mt-3 text-[10px] tracking-[0.15em] uppercase text-text-muted">
        <span class="font-mono">{formatDuration(track.duration_ms)}</span>
        <span class="text-border">/</span>
        <span>{track.format}</span>
        <span class="text-border">/</span>
        <span class="font-mono">{track.sample_rate} hz</span>
        <span class="text-border">/</span>
        <span>{formatFileSize(track.file_size)}</span>
        <span class="text-border">/</span>
        <span>{track.uploader?.display_name || track.uploader?.email}</span>
        <span class="text-border">/</span>
        <span>{formatRelativeTime(track.created_at)}</span>
      </div>
    </div>

    <!-- Player -->
    {#if track.status === "ready"}
      <div class="mb-8">
        <WaveformPlayer
          src={streamUrl}
          peaks={track.waveform_data}
          duration={track.duration_ms}
          comments={timedComments}
          onTimestampClick={handleTimestampClick}
        />
      </div>
    {:else if track.status === "processing"}
      <div class="bg-bg-surface border border-border p-10 text-center mb-8">
        <div class="flex items-center justify-center gap-2 text-accent">
          <div class="w-1.5 h-1.5 bg-accent animate-pulse"></div>
          <span class="text-sm tracking-[0.2em] uppercase">processing</span>
        </div>
      </div>
    {:else}
      <div class="bg-bg-surface border border-border p-10 text-center mb-8 text-danger text-sm tracking-wider">
        processing failed
      </div>
    {/if}

    <!-- Comment input -->
    <div class="mb-8">
      <form onsubmit={(e) => { e.preventDefault(); submitComment(); }} class="flex gap-2">
        <div class="flex-1 relative">
          {#if commentTimestamp != null}
            <button
              type="button"
              onclick={clearTimestamp}
              class="absolute left-3 top-1/2 -translate-y-1/2 text-[10px] text-marker bg-marker/10 px-2 py-0.5 font-mono tracking-wider hover:bg-marker/20 transition-colors"
            >
              @{Math.floor(commentTimestamp / 1000)}s x
            </button>
          {/if}
          <input
            id="comment-input"
            bind:value={newComment}
            type="text"
            placeholder={commentTimestamp != null ? "" : "add a comment..."}
            class="w-full bg-bg-surface border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
            style:padding-left={commentTimestamp != null ? "5.5rem" : undefined}
          />
        </div>
        <button
          type="submit"
          disabled={posting || !newComment.trim()}
          class="px-5 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary text-xs font-semibold tracking-[0.2em] uppercase transition-colors"
        >
          post
        </button>
      </form>
      <div class="text-[10px] tracking-[0.15em] uppercase text-text-muted mt-2">
        click comment button or alt+click waveform to attach timestamp
      </div>
    </div>

    <!-- Comments -->
    <div>
      <h3 class="text-xs font-semibold tracking-[0.3em] uppercase text-text-secondary mb-4">
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
