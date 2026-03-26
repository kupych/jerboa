<script lang="ts">
  import { onMount } from "svelte";
  import { navigate } from "../stores/router";
  import { changelog, CURRENT_VERSION } from "../changelog";

  const STORAGE_KEY = "app:last-seen-version";

  let open = $state(false);
  let latest = changelog[0];

  onMount(() => {
    try {
      const seen = localStorage.getItem(STORAGE_KEY);
      if (seen !== CURRENT_VERSION) {
        open = true;
      }
    } catch {}
  });

  function dismiss() {
    open = false;
    try {
      localStorage.setItem(STORAGE_KEY, CURRENT_VERSION);
    } catch {}
  }

  function viewChangelog() {
    dismiss();
    navigate("/changelog");
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
    onclick={dismiss}
  >
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="bg-bg-primary border border-border w-full max-w-md mx-4"
      onclick={(e) => e.stopPropagation()}
    >
      <div class="px-6 py-5 border-b border-border flex items-center justify-between">
        <div>
          <h2 class="label text-text-primary tracking-wider">what's new</h2>
          <span class="label-sm text-text-muted font-mono mt-1">{latest.version} &mdash; {latest.date}</span>
        </div>
        <button onclick={dismiss} class="label-sm text-text-muted hover:text-text-secondary transition-colors">esc</button>
      </div>
      <div class="px-6 py-5 space-y-2 max-h-80 overflow-y-auto">
        {#each latest.summary as item}
          <div class="flex gap-3 items-start">
            <span class="text-accent mt-1.5 shrink-0 w-1 h-1 bg-accent rounded-full"></span>
            <span class="text-sm font-medium text-text-secondary">{item}</span>
          </div>
        {/each}
      </div>
      <div class="px-6 py-4 border-t border-border flex items-center justify-between">
        <button
          onclick={viewChangelog}
          class="label-sm text-text-muted hover:text-accent transition-colors"
        >full changelog</button>
        <button
          onclick={dismiss}
          class="px-4 py-2 bg-accent hover:bg-accent-hover text-bg-primary label-sm transition-colors"
        >got it</button>
      </div>
    </div>
  </div>
{/if}
