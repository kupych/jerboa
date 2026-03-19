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
</script>

<div class="space-y-0">
  {#each comments as comment}
    <div class="group flex gap-3 py-3 px-3 hover:bg-bg-surface/50 transition-colors border-b border-border/50 last:border-0">
      {#if comment.user.avatar_url}
        <img src={comment.user.avatar_url} alt="" class="w-6 h-6 rounded-sm mt-0.5 shrink-0" />
      {:else}
        <div class="w-6 h-6 rounded-sm bg-accent/15 text-accent text-[9px] flex items-center justify-center font-bold mt-0.5 shrink-0 tracking-wider">
          {initials(comment.user.display_name || comment.user.email)}
        </div>
      {/if}

      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-3">
          <span class="text-xs font-semibold tracking-wider text-text-primary">
            {comment.user.display_name || comment.user.email}
          </span>
          {#if comment.timestamp_ms != null}
            <button
              onclick={() => onSeek(comment.timestamp_ms!)}
              class="text-[10px] text-marker hover:text-marker-hover font-mono tabular-nums transition-colors tracking-wider"
            >
              @{formatTimestamp(comment.timestamp_ms)}
            </button>
          {/if}
          <span class="text-[10px] text-text-muted tracking-wider">{formatRelativeTime(comment.created_at)}</span>

          {#if comment.user.id === $user?.id}
            <button
              onclick={() => deleteComment(comment.id)}
              class="ml-auto text-[10px] tracking-[0.15em] uppercase text-text-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-all"
            >
              del
            </button>
          {/if}
        </div>
        <p class="text-xs text-text-secondary mt-1 leading-relaxed">{comment.body}</p>

        {#if comment.replies && comment.replies.length > 0}
          <div class="mt-2 ml-3 border-l border-border pl-3 space-y-2">
            {#each comment.replies as reply}
              <div class="flex gap-2 py-1">
                <div class="w-4 h-4 rounded-sm bg-accent/10 text-accent text-[7px] flex items-center justify-center font-bold mt-0.5 shrink-0">
                  {initials(reply.user.display_name || reply.user.email)}
                </div>
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="text-[11px] font-semibold tracking-wider text-text-primary">{reply.user.display_name || reply.user.email}</span>
                    <span class="text-[10px] text-text-muted">{formatRelativeTime(reply.created_at)}</span>
                  </div>
                  <p class="text-[11px] text-text-secondary mt-0.5">{reply.body}</p>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/each}

  {#if comments.length === 0}
    <div class="text-center py-10 text-text-muted text-xs tracking-[0.3em] uppercase">
      no comments yet
    </div>
  {/if}
</div>
