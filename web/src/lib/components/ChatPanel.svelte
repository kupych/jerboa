<script lang="ts">
  import { onMount, tick } from "svelte";
  import { api, apiPost } from "../api";
  import { ws } from "../ws";
  import { route } from "../stores/router";
  import { initials, formatRelativeTime } from "../utils/format";

  let { open = $bindable(false) }: { open: boolean } = $props();

  interface ChatMsg {
    id: string;
    body: string;
    created_at: string;
    user: { id: string; display_name: string; email: string; avatar_url?: string };
  }

  let messages = $state<ChatMsg[]>([]);
  let input = $state("");
  let sending = $state(false);
  let messagesEl = $state<HTMLDivElement | null>(null);
  let inputEl = $state<HTMLInputElement | null>(null);
  let slug = $derived($route.bandSlug);
  let loaded = $state(false);

  $effect(() => {
    if (open && slug && !loaded) {
      loadMessages();
    }
  });

  $effect(() => {
    if (slug) {
      const off = ws.on("chat.message", (payload: any) => {
        const msg = payload as ChatMsg;
        if (!messages.some((m) => m.id === msg.id)) {
          messages = [...messages, msg];
          tick().then(scrollBottom);
        }
      });
      return off;
    }
  });

  async function loadMessages() {
    if (!slug) return;
    try {
      const msgs = await api<ChatMsg[]>(`/api/bands/${slug}/chat`);
      messages = (msgs || []).reverse();
      loaded = true;
      await tick();
      scrollBottom();
    } catch {}
  }

  async function send() {
    if (!input.trim() || !slug) return;
    sending = true;
    try {
      const msg = await apiPost<ChatMsg>(`/api/bands/${slug}/chat`, { body: input.trim() });
      if (!messages.some((m) => m.id === msg.id)) {
        messages = [...messages, msg];
      }
      input = "";
      await tick();
      scrollBottom();
    } finally {
      sending = false;
      await tick();
      inputEl?.focus();
    }
  }

  function scrollBottom() {
    if (messagesEl) {
      messagesEl.scrollTop = messagesEl.scrollHeight;
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }
</script>

<!-- Backdrop -->
{#if open}
  <!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
  <div
    class="fixed inset-0 bg-black/40 z-40 md:bg-transparent md:pointer-events-none"
    onclick={() => (open = false)}
  ></div>
{/if}

<!-- Panel -->
<div
  class="fixed top-0 right-0 h-full w-80 max-w-[85vw] bg-bg-secondary border-l border-border z-50 flex flex-col transition-transform {open ? 'translate-x-0 duration-200 ease-out' : 'translate-x-full duration-150 ease-in'}"
>
  <!-- Header -->
  <div class="flex items-center justify-between px-5 h-16 border-b border-border shrink-0">
    <span class="label text-text-secondary">chat</span>
    <button
      onclick={() => (open = false)}
      class="label text-text-muted hover:text-text-secondary transition-colors"
    >x</button>
  </div>

  {#if !slug}
    <div class="flex-1 flex items-center justify-center label text-text-muted">
      open a band to chat
    </div>
  {:else}
    <!-- Messages -->
    <div bind:this={messagesEl} class="flex-1 overflow-y-auto px-4 py-4 space-y-4">
      {#each messages as msg}
        <div class="flex gap-3">
          {#if msg.user.avatar_url}
            <img src={msg.user.avatar_url} alt="" class="w-6 h-6 shrink-0 mt-0.5" />
          {:else}
            <div class="w-6 h-6 bg-accent/15 text-accent text-[8px] flex items-center justify-center font-bold shrink-0 mt-0.5">
              {initials(msg.user.display_name || msg.user.email)}
            </div>
          {/if}
          <div class="min-w-0">
            <div class="flex items-baseline gap-2">
              <span class="text-xs font-bold text-text-primary truncate">{msg.user.display_name || msg.user.email}</span>
              <span class="text-[10px] font-semibold text-text-muted shrink-0">{formatRelativeTime(msg.created_at)}</span>
            </div>
            <p class="text-sm text-text-secondary mt-0.5 break-words">{msg.body}</p>
          </div>
        </div>
      {/each}

      {#if messages.length === 0 && loaded}
        <div class="text-center label text-text-muted py-10">no messages yet</div>
      {/if}
    </div>

    <!-- Input -->
    <div class="border-t border-border px-4 py-3 shrink-0">
      <div class="flex gap-2">
        <input
          bind:this={inputEl}
          bind:value={input}
          type="text"
          placeholder="say something..."
          onkeydown={handleKeydown}
          disabled={sending}
          class="flex-1 bg-bg-surface border border-border px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
        />
        <button
          onclick={send}
          disabled={sending || !input.trim()}
          class="px-3 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors border border-accent"
        >go</button>
      </div>
    </div>
  {/if}
</div>
