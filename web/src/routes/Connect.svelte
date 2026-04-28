<script lang="ts">
  import { onMount } from "svelte";
  import { apiPost } from "../lib/api";
  import { bands, loadBands } from "../lib/stores/bands";

  let code = $state("");
  let selectedBand = $state<string>("");
  let submitting = $state(false);
  let done = $state(false);
  let error = $state<string | null>(null);

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    code = (params.get("code") || "").toUpperCase();
    loadBands();
  });

  $effect(() => {
    if (!selectedBand && $bands.length > 0) {
      selectedBand = $bands[0].slug;
    }
  });

  async function authorize() {
    if (!code || !selectedBand) return;
    submitting = true;
    error = null;
    try {
      await apiPost("/api/pair/authorize", { code, band_slug: selectedBand });
      done = true;
    } catch (e: any) {
      error = e?.message || "failed to authorize";
    } finally {
      submitting = false;
    }
  }
</script>

<div class="min-h-screen flex flex-col items-center justify-center px-6">
  <div class="w-full max-w-md space-y-6">
    <div class="text-center">
      <h2 class="text-lg font-bold tracking-wider font-display text-text-primary">connect device</h2>
      <p class="text-xs font-semibold text-text-muted mt-2">authorize a Reaper installation to sync with one of your bands</p>
    </div>

    {#if done}
      <div class="border border-accent/40 px-4 py-6 text-center space-y-2">
        <div class="label text-accent">connected</div>
        <p class="text-xs font-semibold text-text-muted">return to Reaper — the panel will pick up the connection automatically</p>
      </div>
    {:else}
      <div class="space-y-4">
        <div>
          <label for="pair-code" class="block label text-text-muted mb-1">pairing code</label>
          <input
            id="pair-code"
            type="text"
            bind:value={code}
            placeholder="XXXX-XXXX"
            class="w-full bg-transparent border border-border text-text-primary text-sm font-semibold font-mono px-4 py-3 placeholder:text-text-muted/30 focus:border-accent focus:outline-none transition-colors text-center tracking-[0.3em]"
          />
        </div>

        <div>
          <label for="pair-band" class="block label text-text-muted mb-1">band</label>
          <select
            id="pair-band"
            bind:value={selectedBand}
            class="w-full bg-transparent border border-border text-text-primary text-sm font-semibold px-4 py-3 focus:border-accent focus:outline-none transition-colors"
          >
            {#each $bands as b}
              <option value={b.slug}>{b.name}</option>
            {/each}
          </select>
        </div>

        {#if error}
          <p class="text-xs font-semibold text-danger">{error}</p>
        {/if}

        <button
          onclick={authorize}
          disabled={submitting || !code || !selectedBand}
          class="w-full px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
        >
          {submitting ? "..." : "authorize"}
        </button>
      </div>
    {/if}
  </div>
</div>
