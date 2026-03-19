<script lang="ts">
  import { user } from "../stores/auth";
  import { apiDelete } from "../api";
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

<div class="space-y-0">
  {#each comments as comment}
    <div class="group flex gap-4 py-4 px-4 hover:bg-bg-surface/50 transition-colors border-b border-border/50 last:border-0">
      {#if comment.user.avatar_url}
        <img src={comment.user.avatar_url} alt="" class="w-7 h-7 mt-0.5 shrink-0" />
      {:else}
        <div class="w-7 h-7 bg-accent/15 text-accent text-[10px] flex items-center justify-center font-bold mt-0.5 shrink-0 tracking-wider">
          {initials(comment.user.display_name || comment.user.email)}
        </div>
      {/if}

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

          {#if comment.user.id === $user?.id}
            <button
              onclick={() => deleteComment(comment.id)}
              class="ml-auto label-sm text-text-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-all"
            >
              del
            </button>
          {/if}
        </div>
        <p class="text-sm font-medium text-text-secondary mt-2 leading-relaxed">{#each parseMentions(comment.body) as part}{#if part.type === "mention"}<span class="text-accent font-semibold">@{part.value}</span>{:else}{part.value}{/if}{/each}</p>

        {#if comment.replies && comment.replies.length > 0}
          <div class="mt-3 ml-4 border-l-2 border-border pl-4 space-y-3">
            {#each comment.replies as reply}
              <div class="flex gap-3 py-1">
                <div class="w-5 h-5 bg-accent/10 text-accent text-[8px] flex items-center justify-center font-bold mt-0.5 shrink-0">
                  {initials(reply.user.display_name || reply.user.email)}
                </div>
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-semibold tracking-wider text-text-primary">{reply.user.display_name || reply.user.email}</span>
                    <span class="label-sm text-text-muted">{formatRelativeTime(reply.created_at)}</span>
                  </div>
                  <p class="text-sm font-medium text-text-secondary mt-1">{#each parseMentions(reply.body) as part}{#if part.type === "mention"}<span class="text-accent font-semibold">@{part.value}</span>{:else}{part.value}{/if}{/each}</p>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/each}

  {#if comments.length === 0}
    <div class="text-center py-14 label text-text-muted">
      no comments yet
    </div>
  {/if}
</div>
