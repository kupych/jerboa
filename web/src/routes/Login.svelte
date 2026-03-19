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

  <div class="w-full max-w-md">
    <!-- Title block: left-aligned -->
    <div class="flex items-baseline justify-between">
      <h1 class="text-3xl font-bold tracking-[0.2em] uppercase text-accent font-display">
        jerboa
      </h1>
      <p class="text-xs font-semibold text-accent tracking-[0.25em] uppercase" style="margin-right: -0.25em;">all ears.</p>
    </div>

    <!-- LED spectrum analyzer -->
    <div class="flex gap-[3px] mt-6 mb-8 w-full">
      {#each levels as level, col}
        <div class="flex-1 flex flex-col-reverse gap-[3px]">
          {#each Array(ROWS) as _, row}
            <div
              class="h-2"
              style="opacity: {row < Math.ceil(level) ? 0.8 : 0.06}; background: var(--color-accent);"
            ></div>
          {/each}
        </div>
      {/each}
    </div>

    <!-- Sign in: flush right -->
    <div class="border-t border-accent/40 flex justify-end" style="margin-top: 3px; padding-top: 0.25rem;">
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
