<script lang="ts">
  import { user } from "../stores/auth";
  import { apiDelete, apiPut, apiPost } from "../api";
  import { formatTimestamp, formatRelativeTime, initials } from "../utils/format";

  let { comments, trackId, onSeek, onRefresh }: {
    comments: Array<{
      id: string;
      body: string;
      timestamp_ms: number | null;
      created_at: string;
      user: { id: string; display_name: string; email: string; avatar_url?: string };
      replies?: Array<any>;
    }>;
    trackId: string;
    onSeek: (ms: number) => void;
    onRefresh: () => void;
  } = $props();

  let editingId = $state<string | null>(null);
  let editBody = $state("");
  let replyingTo = $state<string | null>(null);
  let replyBody = $state("");

  function startEdit(comment: { id: string; body: string }) {
    editingId = comment.id;
    editBody = comment.body;
    replyingTo = null;
  }

  function cancelEdit() {
    editingId = null;
    editBody = "";
  }

  async function saveEdit() {
    if (!editingId || !editBody.trim()) return;
    await apiPut(`/api/comments/${editingId}`, { body: editBody.trim() });
    editingId = null;
    editBody = "";
    onRefresh();
  }

  function startReply(commentId: string) {
    replyingTo = commentId;
    replyBody = "";
    editingId = null;
  }

  function cancelReply() {
    replyingTo = null;
    replyBody = "";
  }

  async function submitReply() {
    if (!replyingTo || !replyBody.trim()) return;
    await apiPost(`/api/tracks/${trackId}/comments`, {
      body: replyBody.trim(),
      parent_id: replyingTo,
    });
    replyingTo = null;
    replyBody = "";
    onRefresh();
  }

  async function deleteComment(id: string) {
    await apiDelete(`/api/comments/${id}`);
    onRefresh();
  }

  function parseMentions(text: string): Array<{ type: "text" | "mention"; value: string }> {
    const parts: Array<{ type: "text" | "mention"; value: string }> = [];
    const regex = /@([\w.\-]+(?:\s[\w.\-]+)?)/g;
    let lastIndex = 0;
    let match;
    while ((match = regex.exec(text)) !== null) {
      if (match.index > lastIndex) {
        parts.push({ type: "text", value: text.slice(lastIndex, match.index) });
      }
      parts.push({ type: "mention", value: match[1] });
      lastIndex = regex.lastIndex;
    }
    if (lastIndex < text.length) {
      parts.push({ type: "text", value: text.slice(lastIndex) });
    }
    return parts;
  }
</script>

{#snippet commentBody(text: string)}
  {#each parseMentions(text) as part}{#if part.type === "mention"}<span class="text-accent font-semibold">@{part.value}</span>{:else}{part.value}{/if}{/each}
{/snippet}

{#snippet avatar(u: { display_name?: string; email?: string; avatar_url?: string }, size: string = "w-7 h-7")}
  {#if u.avatar_url}
    <img src={u.avatar_url} alt="" class="{size} mt-0.5 shrink-0" />
  {:else}
    <div class="{size} bg-accent/15 text-accent text-[10px] flex items-center justify-center font-bold mt-0.5 shrink-0 tracking-wider">
      {initials(u.display_name || u.email || "")}
    </div>
  {/if}
{/snippet}

<div class="space-y-0">
  {#each comments as comment}
    <div id="comment-{comment.id}" class="group py-4 px-4 hover:bg-bg-surface/50 transition-colors border-b border-border/50 last:border-0">
      <div class="flex gap-4">
        {@render avatar(comment.user)}

        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-3">
            <span class="text-sm font-semibold tracking-wider text-text-primary">
              {comment.user.display_name || comment.user.email}
            </span>
            {#if comment.timestamp_ms != null}
              <button
                onclick={() => onSeek(comment.timestamp_ms!)}
                class="label-sm text-marker hover:text-marker-hover font-mono tabular-nums transition-colors"
              >
                @{formatTimestamp(comment.timestamp_ms)}
              </button>
            {/if}
            <span class="label-sm text-text-muted">{formatRelativeTime(comment.created_at)}</span>

            <div class="ml-auto flex items-center gap-3 opacity-0 group-hover:opacity-100 transition-all">
              <button
                onclick={() => startReply(comment.id)}
                class="label-sm text-text-muted hover:text-accent transition-colors"
              >
                reply
              </button>
              {#if comment.user.id === $user?.id}
                <button
                  onclick={() => startEdit(comment)}
                  class="label-sm text-text-muted hover:text-accent transition-colors"
                >
                  edit
                </button>
                <button
                  onclick={() => deleteComment(comment.id)}
                  class="label-sm text-text-muted hover:text-danger transition-colors"
                >
                  del
                </button>
              {/if}
            </div>
          </div>

          {#if editingId === comment.id}
            <div class="mt-2">
              <textarea
                bind:value={editBody}
                onkeydown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); saveEdit(); } if (e.key === "Escape") cancelEdit(); }}
                class="w-full bg-bg-primary border border-border text-sm font-medium text-text-primary px-3 py-2 resize-none focus:outline-none focus:border-accent"
                rows="2"
              ></textarea>
              <div class="flex items-center gap-2 mt-1">
                <button onclick={saveEdit} class="label-sm text-accent hover:text-accent/80 transition-colors">save</button>
                <button onclick={cancelEdit} class="label-sm text-text-muted hover:text-text-secondary transition-colors">cancel</button>
              </div>
            </div>
          {:else}
            <p class="text-sm font-medium text-text-secondary mt-2 leading-relaxed">{@render commentBody(comment.body)}</p>
          {/if}
        </div>
      </div>

      <!-- Replies -->
      {#if (comment.replies && comment.replies.length > 0) || replyingTo === comment.id}
        <div class="mt-3 ml-11 border-l-2 border-border pl-4 space-y-0">
          {#each comment.replies ?? [] as reply}
            <div class="group/reply flex gap-3 py-2">
              {@render avatar(reply.user, "w-5 h-5")}
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-semibold tracking-wider text-text-primary">{reply.user.display_name || reply.user.email}</span>
                  <span class="label-sm text-text-muted">{formatRelativeTime(reply.created_at)}</span>
                  {#if reply.user.id === $user?.id}
                    <button
                      onclick={() => deleteComment(reply.id)}
                      class="ml-auto label-sm text-text-muted hover:text-danger opacity-0 group-hover/reply:opacity-100 transition-all"
                    >
                      del
                    </button>
                  {/if}
                </div>
                <p class="text-sm font-medium text-text-secondary mt-1">{@render commentBody(reply.body)}</p>
              </div>
            </div>
          {/each}

          {#if replyingTo === comment.id}
            <div class="flex gap-3 py-2">
              {@render avatar($user ?? { display_name: "", email: "" }, "w-5 h-5")}
              <div class="flex-1">
                <textarea
                  bind:value={replyBody}
                  onkeydown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submitReply(); } if (e.key === "Escape") cancelReply(); }}
                  placeholder="reply..."
                  class="w-full bg-bg-primary border border-border text-sm font-medium text-text-primary px-3 py-2 resize-none focus:outline-none focus:border-accent placeholder:text-text-muted/50"
                  rows="1"
                ></textarea>
                <div class="flex items-center gap-2 mt-1">
                  <button onclick={submitReply} class="label-sm text-accent hover:text-accent/80 transition-colors">reply</button>
                  <button onclick={cancelReply} class="label-sm text-text-muted hover:text-text-secondary transition-colors">cancel</button>
                </div>
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/each}

  {#if comments.length === 0}
    <div class="text-center py-14 label text-text-muted">
      no comments yet
    </div>
  {/if}
</div>
