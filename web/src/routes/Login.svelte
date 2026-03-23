<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { login, sendMagicLink } from "../lib/stores/auth";
  import ThemeToggle from "../lib/components/ThemeToggle.svelte";

  const COLS = 32;
  const ROWS = 8;
  const DECAY = 0.6;
  const ATTACK_CHANCE = 0.12;
  let levels = $state(Array(COLS).fill(0));
  let timer: ReturnType<typeof setInterval>;

  let emailInput = $state("");
  let magicLinkSent = $state(false);
  let sending = $state(false);

  onMount(() => {
    timer = setInterval(() => {
      levels = levels.map((prev) => {
        if (Math.random() < ATTACK_CHANCE) {
          const peak = Math.ceil(Math.random() * ROWS);
          return Math.max(prev, peak);
        }
        return Math.max(0, prev - DECAY);
      });
    }, 80);
  });

  onDestroy(() => clearInterval(timer));

  async function handleSubmit() {
    if (!emailInput.trim()) return;
    sending = true;
    try {
      const method = await sendMagicLink(emailInput.trim());
      if (method === "oidc") {
        // Google account — redirect to OIDC flow
        login();
        return;
      }
      magicLinkSent = true;
    } catch {
      magicLinkSent = true;
    } finally {
      sending = false;
    }
  }
</script>

<div class="min-h-screen flex flex-col items-center justify-center px-6">
  <div class="absolute top-4 right-6">
    <ThemeToggle />
  </div>

  <div class="w-full max-w-sm -mt-16">
    <!-- Logo -->
    <div class="flex justify-center mb-6">
      <div class="w-24 h-24 md:w-28 md:h-28 text-accent" title="jerbert" style="-webkit-mask: url(/logo.svg) center/contain no-repeat; mask: url(/logo.svg) center/contain no-repeat; background: currentColor;"></div>
    </div>

    <!-- Title block -->
    <div class="flex items-baseline justify-between">
      <h1 class="text-2xl font-bold tracking-[0.2em] uppercase text-accent font-display">
        jerboa
      </h1>
      <p class="text-xs font-semibold text-accent/60 tracking-[0.25em] uppercase" style="margin-right: -0.25em;">all ears.</p>
    </div>

    <!-- LED spectrum analyzer -->
    <div class="flex gap-[2px] mt-5 mb-6 w-full">
      {#each levels as level, col}
        <div class="flex-1 flex flex-col-reverse gap-[2px]">
          {#each Array(ROWS) as _, row}
            <div
              class="h-1.5"
              style="opacity: {row < Math.ceil(level) ? 0.6 : 0.04}; background: var(--color-accent);"
            ></div>
          {/each}
        </div>
      {/each}
    </div>

    <!-- Sign in -->
    <div class="border-t border-accent/30" style="margin-top: 2px; padding-top: 0.25rem;">
      <span class="text-[9px] font-semibold tracking-[0.15em] text-accent/25 uppercase select-none leading-none block" style="margin-bottom: 12px;">{version} — JRB·001A</span>

      {#if magicLinkSent}
        <div class="text-center py-2 space-y-3">
          <p class="text-sm font-semibold text-accent">check your email for a login link</p>
          <button
            onclick={() => { magicLinkSent = false; emailInput = ""; }}
            class="text-[10px] font-semibold tracking-[0.15em] text-accent/30 hover:text-accent/50 uppercase transition-colors"
          >
            try again
          </button>
        </div>
      {:else}
        <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="flex gap-2">
          <input
            type="email"
            bind:value={emailInput}
            placeholder="email"
            autofocus
            class="flex-1 bg-transparent border border-accent/20 text-accent text-sm font-semibold px-3 py-2.5 placeholder:text-accent/20 focus:border-accent/40 focus:outline-none transition-colors"
          />
          <button
            type="submit"
            disabled={sending || !emailInput.trim()}
            class="px-5 py-2.5 bg-accent text-bg-primary text-[10px] font-bold tracking-[0.2em] uppercase hover:bg-accent-hover transition-colors disabled:opacity-30"
          >
            {sending ? "..." : "sign in"}
          </button>
        </form>
        <button
          onclick={login}
          class="mt-3 text-[10px] font-semibold tracking-[0.15em] text-accent/30 hover:text-accent/50 uppercase transition-colors"
        >
          sign in with google
        </button>
      {/if}
    </div>
  </div>
</div>
