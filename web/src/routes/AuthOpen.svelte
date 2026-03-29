<script lang="ts">
  import { onMount } from "svelte";
  import { navigate } from "../lib/stores/router";

  // If already running inside the installed PWA, just go straight to the app.
  const inPWA =
    typeof window !== "undefined" &&
    window.matchMedia("(display-mode: standalone)").matches;

  onMount(() => {
    if (inPWA) navigate("/");
  });
</script>

{#if !inPWA}
  <div class="min-h-screen flex flex-col items-center justify-center px-6 bg-bg-primary">
    <div class="w-full max-w-xs space-y-8 text-center">
      <!-- Logo -->
      <div class="flex justify-center">
        <div
          class="w-14 h-14 text-accent"
          style="-webkit-mask: url(/logo.svg) center/contain no-repeat; mask: url(/logo.svg) center/contain no-repeat; background: currentColor;"
        ></div>
      </div>

      <div class="space-y-2">
        <h1 class="text-lg font-bold tracking-wider font-display text-text-primary">you're logged in</h1>
        <p class="text-sm font-semibold text-text-muted leading-relaxed">
          if you have jerboa on your home screen, switch to it now — you're already in
        </p>
      </div>

      <!-- Continue in browser -->
      <a
        href="/"
        class="block w-full px-6 py-3 bg-accent hover:bg-accent-hover text-bg-primary label transition-colors text-center"
      >
        open jerboa
      </a>
    </div>
  </div>
{/if}
