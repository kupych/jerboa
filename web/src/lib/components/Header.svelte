<script lang="ts">
  import { user, logout } from "../stores/auth";
  import { navigate } from "../stores/router";
  import ThemeToggle from "./ThemeToggle.svelte";
  import { initials } from "../utils/format";
</script>

<header class="border-b border-border bg-bg-secondary">
  <div class="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between">
    <button
      onclick={() => navigate("/")}
      class="text-base font-bold tracking-[0.35em] uppercase text-accent hover:text-accent-hover transition-colors"
    >
      jerboa
    </button>

    <div class="flex items-center gap-4">
      <ThemeToggle />

      {#if $user}
        <div class="flex items-center gap-3">
          {#if $user.avatar_url}
            <img
              src={$user.avatar_url}
              alt=""
              class="w-7 h-7 rounded-sm"
            />
          {:else}
            <div
              class="w-7 h-7 rounded-sm bg-accent/20 text-accent text-[11px] flex items-center justify-center font-bold tracking-wider"
            >
              {initials($user.display_name || $user.email)}
            </div>
          {/if}
          <button
            onclick={() => logout()}
            class="text-xs tracking-widest uppercase text-text-muted hover:text-text-secondary transition-colors"
          >
            out
          </button>
        </div>
      {/if}
    </div>
  </div>
</header>
