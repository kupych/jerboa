<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { login } from "../lib/stores/auth";
  import ThemeToggle from "../lib/components/ThemeToggle.svelte";

  const COLS = 32;
  const ROWS = 8;
  const DECAY = 0.6;          // cells to drop per tick
  const ATTACK_CHANCE = 0.12; // chance of a new spike per column per tick
  let levels = $state(Array(COLS).fill(0));
  let timer: ReturnType<typeof setInterval>;

  onMount(() => {
    timer = setInterval(() => {
      levels = levels.map((prev) => {
        // Fast attack: occasional spike to a random peak
        if (Math.random() < ATTACK_CHANCE) {
          const peak = Math.ceil(Math.random() * ROWS);
          return Math.max(prev, peak);
        }
        // Slow decay: drop gradually
        return Math.max(0, prev - DECAY);
      });
    }, 80);
  });

  onDestroy(() => clearInterval(timer));
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
    <div class="border-t border-accent/30 flex justify-between items-start" style="margin-top: 2px; padding-top: 0.25rem;">
      <span class="text-[9px] font-semibold tracking-[0.15em] text-accent/25 uppercase select-none leading-none" style="margin-top: 1px;">{version} — JRB·001A</span>
      <button
        onclick={login}
        style="margin-right: -0.25em;"
        class="text-accent hover:text-accent-hover text-sm font-bold tracking-[0.25em] uppercase transition-colors leading-none"
      >
        sign in
      </button>
    </div>
  </div>
</div>
