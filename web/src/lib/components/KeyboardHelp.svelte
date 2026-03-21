<script lang="ts">
  import { player } from "../stores/player";

  let showHelp = $state(false);
  let toast = $state<string | null>(null);
  let toastTimeout: ReturnType<typeof setTimeout> | null = null;

  function showToast(msg: string) {
    toast = msg;
    if (toastTimeout) clearTimeout(toastTimeout);
    toastTimeout = setTimeout(() => (toast = null), 1500);
  }

  const shortcuts: Array<{ keys: string; desc: string }> = [
    { keys: "space", desc: "play / pause" },
    { keys: "← →", desc: "seek ±5s" },
    { keys: "shift + ← →", desc: "seek ±30s" },
    { keys: "n", desc: "next take" },
    { keys: "p", desc: "previous take" },
    { keys: "m", desc: "mute / unmute" },
    { keys: "[ ]", desc: "playback speed" },
    { keys: "0", desc: "restart" },
    { keys: "?", desc: "this help" },
    { keys: "esc", desc: "close / unfocus" },
  ];

  function handleKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement;
    const tag = target.tagName;
    if (
      tag === "INPUT" ||
      tag === "TEXTAREA" ||
      tag === "SELECT" ||
      target.isContentEditable
    ) {
      if (e.key === "Escape") {
        (target as HTMLElement).blur();
      }
      return;
    }

    // Blur focused buttons so space/enter don't also trigger their click handler
    if (tag === "BUTTON") {
      (target as HTMLElement).blur();
    }

    switch (e.key) {
      case " ":
        e.preventDefault();
        player.toggle();
        break;
      case "ArrowLeft":
        e.preventDefault();
        player.seekRelative(e.shiftKey ? -30 : -5);
        break;
      case "ArrowRight":
        e.preventDefault();
        player.seekRelative(e.shiftKey ? 30 : 5);
        break;
      case "n":
        player.next();
        break;
      case "p":
        player.prev();
        break;
      case "m": {
        const muted = player.toggleMute();
        if (muted != null) showToast(muted ? "muted" : "unmuted");
        break;
      }
      case "]": {
        const speed = player.speedUp();
        if (speed != null) showToast(`${speed}x`);
        break;
      }
      case "[": {
        const speed = player.speedDown();
        if (speed != null) showToast(`${speed}x`);
        break;
      }
      case "0":
        player.restart();
        break;
      case "?":
        showHelp = !showHelp;
        break;
      case "Escape":
        if (showHelp) showHelp = false;
        break;
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Toast -->
{#if toast}
  <div
    class="fixed bottom-6 right-6 z-50 bg-bg-surface border border-border px-4 py-2 label-sm text-text-primary shadow-lg animate-in"
  >
    {toast}
  </div>
{/if}

<!-- Help modal -->
{#if showHelp}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
    onclick={() => (showHelp = false)}
  >
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="bg-bg-primary border border-border w-full max-w-md mx-4"
      onclick={(e) => e.stopPropagation()}
    >
      <div class="px-6 py-5 border-b border-border">
        <h2 class="label text-text-primary tracking-wider">keyboard shortcuts</h2>
      </div>
      <div class="px-6 py-4">
        {#each shortcuts as s}
          <div class="flex items-center justify-between py-2.5">
            <span class="font-mono text-sm font-semibold text-accent tracking-wide">{s.keys}</span>
            <span class="label-sm text-text-muted">{s.desc}</span>
          </div>
        {/each}
      </div>
      <div class="px-6 py-4 border-t border-border">
        <p class="label-sm text-text-muted text-center">press <span class="text-accent font-mono">?</span> or <span class="text-accent font-mono">esc</span> to dismiss</p>
      </div>
    </div>
  </div>
{/if}

<style>
  .animate-in {
    animation: fadeSlideIn 0.15s ease-out;
  }
  @keyframes fadeSlideIn {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
