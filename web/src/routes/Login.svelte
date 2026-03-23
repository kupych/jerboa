<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { login, sendMagicLink } from "../lib/stores/auth";
  import ThemeToggle from "../lib/components/ThemeToggle.svelte";

  const COLS = 32;
  const ROWS = 8;
  const DECAY = 0.6;          // cells to drop per tick
  const ATTACK_CHANCE = 0.12; // chance of a new spike per column per tick
  let levels = $state(Array(COLS).fill(0));
  let timer: ReturnType<typeof setInterval>;

  let emailInput = $state("");
  let magicLinkSent = $state(false);
  let magicLinkSending = $state(false);
  let showEmailLogin = $state(false);

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

  async function handleMagicLink() {
    if (!emailInput.trim()) return;
    magicLinkSending = true;
    try {
      await sendMagicLink(emailInput.trim());
      magicLinkSent = true;
    } catch {
      // still show sent (no email enumeration)
      magicLinkSent = true;
    } finally {
      magicLinkSending = false;
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
      <div class="w-24 h-24 md:w-28 md:h-28 text-accent" style="-webkit-mask: url(/logo.svg) center/contain no-repeat; mask: url(/logo.svg) center/contain no-repeat; background: currentColor;"></div>
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
      <div class="flex justify-between items-start">
        <span class="text-[9px] font-semibold tracking-[0.15em] text-accent/25 uppercase select-none leading-none" style="margin-top: 1px;">{version} — JRB·001A</span>
        <button
          onclick={login}
          style="margin-right: -0.25em;"
          class="text-accent hover:text-accent-hover text-sm font-bold tracking-[0.25em] uppercase transition-colors leading-none"
        >
          sign in
        </button>
      </div>

      <!-- Email login toggle -->
      {#if !showEmailLogin}
        <button
          onclick={() => showEmailLogin = true}
          class="mt-4 text-[10px] font-semibold tracking-[0.15em] text-accent/30 hover:text-accent/50 uppercase transition-colors"
        >
          no google? sign in with email
        </button>
      {:else if magicLinkSent}
        <div class="mt-4">
          <p class="text-xs font-semibold text-accent/60">check your email for a login link</p>
        </div>
      {:else}
        <form onsubmit={(e) => { e.preventDefault(); handleMagicLink(); }} class="mt-4 flex gap-2">
          <input
            type="email"
            bind:value={emailInput}
            placeholder="email"
            class="flex-1 bg-transparent border border-accent/20 text-accent text-xs font-semibold px-3 py-2 placeholder:text-accent/20 focus:border-accent/40 focus:outline-none transition-colors"
          />
          <button
            type="submit"
            disabled={magicLinkSending || !emailInput.trim()}
            class="px-4 py-2 bg-accent/10 border border-accent/20 text-accent text-[10px] font-bold tracking-[0.15em] uppercase hover:bg-accent/20 transition-colors disabled:opacity-30"
          >
            {magicLinkSending ? "..." : "send"}
          </button>
        </form>
      {/if}
    </div>
  </div>
</div>
