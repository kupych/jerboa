<script lang="ts">
  import { user, logout } from "../stores/auth";
  import { navigate, route } from "../stores/router";
  import ThemeToggle from "./ThemeToggle.svelte";
  import { initials } from "../utils/format";

  let { onChatToggle, chatOpen }: { onChatToggle: () => void; chatOpen: boolean } = $props();
</script>

<header class="border-b border-border bg-bg-secondary">
  <nav class="flex items-center justify-between h-16 md:h-20 mx-5 md:mx-12">
    <div class="flex items-center gap-4">
      <button
        onclick={() => navigate("/")}
        class="flex items-center gap-2.5 text-lg font-bold tracking-[0.25em] uppercase text-accent hover:text-accent-hover transition-colors font-display"
      >
        <div class="h-7 md:h-8 w-7 md:w-8 bg-current" style="-webkit-mask: url(/logo.svg) center/contain no-repeat; mask: url(/logo.svg) center/contain no-repeat;"></div>
        jerboa
      </button>
      <span class="hidden md:inline text-[9px] font-mono font-semibold tracking-[0.15em] text-text-muted/20 select-none mt-0.5">SYS·AUD·01</span>
    </div>

    <div class="flex items-center gap-4 md:gap-6">
      {#if $route.bandSlug}
        <button
          onclick={onChatToggle}
          class="label transition-colors {chatOpen ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
          title="Chat"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
          </svg>
        </button>
      {/if}

      <ThemeToggle />

      {#if $user}
        <div class="flex items-center gap-3 md:gap-4">
          <button onclick={() => navigate("/profile")} class="cursor-pointer" title="Profile">
            {#if $user.avatar_url}
              <img
                src={$user.avatar_url}
                alt=""
                class="w-7 h-7 md:w-8 md:h-8"
              />
            {:else}
              <div
                class="w-7 h-7 md:w-8 md:h-8 bg-accent/15 text-accent text-[10px] md:text-[11px] flex items-center justify-center font-bold tracking-wider hover:bg-accent/25 transition-colors"
              >
                {initials($user.display_name || $user.email)}
              </div>
            {/if}
          </button>
          <button
            onclick={() => logout()}
            class="label text-text-muted hover:text-text-secondary transition-colors"
          >
            out
          </button>
        </div>
      {/if}
    </div>
  </nav>
</header>
