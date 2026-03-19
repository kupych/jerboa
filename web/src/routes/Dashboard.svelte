<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost } from "../lib/api";
  import { navigate } from "../lib/stores/router";

  interface Band {
    id: string;
    name: string;
    slug: string;
    role: string;
    created_at: string;
  }

  let bands = $state<Band[]>([]);
  let loading = $state(true);
  let showCreate = $state(false);
  let newBandName = $state("");
  let creating = $state(false);

  onMount(async () => {
    await loadBands();
  });

  async function loadBands() {
    loading = true;
    try {
      bands = await api<Band[]>("/api/bands");
    } catch {
      bands = [];
    } finally {
      loading = false;
    }
  }

  async function createBand() {
    if (!newBandName.trim()) return;
    creating = true;
    try {
      const band = await apiPost<Band>("/api/bands", { name: newBandName.trim() });
      newBandName = "";
      showCreate = false;
      navigate(`/band/${band.slug}`);
    } finally {
      creating = false;
    }
  }
</script>

<div>
  <div class="flex items-center justify-between mb-8">
    <h2 class="text-sm font-semibold tracking-[0.3em] uppercase text-text-secondary">bands</h2>
    <button
      onclick={() => (showCreate = !showCreate)}
      class="text-xs tracking-[0.2em] uppercase text-accent hover:text-accent-hover transition-colors flex items-center gap-2"
    >
      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <line x1="12" y1="5" x2="12" y2="19"/>
        <line x1="5" y1="12" x2="19" y2="12"/>
      </svg>
      new band
    </button>
  </div>

  {#if showCreate}
    <form
      onsubmit={(e) => { e.preventDefault(); createBand(); }}
      class="mb-8 bg-bg-surface border border-border p-6"
    >
      <label class="block">
        <span class="text-xs tracking-[0.2em] uppercase text-text-muted block mb-2">band name</span>
        <input
          bind:value={newBandName}
          type="text"
          placeholder="e.g. The Wavelengths"
          class="w-full bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
        />
      </label>
      <div class="flex gap-3 mt-4">
        <button
          type="submit"
          disabled={creating || !newBandName.trim()}
          class="px-6 py-2 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary text-xs font-semibold tracking-[0.2em] uppercase transition-colors"
        >
          {creating ? "..." : "create"}
        </button>
        <button
          type="button"
          onclick={() => (showCreate = false)}
          class="px-6 py-2 text-xs tracking-[0.2em] uppercase text-text-muted hover:text-text-secondary transition-colors"
        >
          cancel
        </button>
      </div>
    </form>
  {/if}

  {#if loading}
    <div class="text-center py-16 text-text-muted text-xs tracking-[0.3em] uppercase">loading</div>
  {:else if bands.length === 0}
    <div class="text-center py-16 space-y-4">
      <div class="text-text-muted text-sm tracking-wider">no bands yet</div>
      <div class="text-text-muted text-xs tracking-wider">create one or join via invite link</div>
    </div>
  {:else}
    <div class="grid gap-2">
      {#each bands as band}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="bg-bg-surface border border-border p-5 hover:border-accent/40 cursor-pointer transition-colors group"
          onclick={() => navigate(`/band/${band.slug}`)}
        >
          <div class="flex items-center justify-between">
            <h3 class="text-base font-semibold tracking-wider group-hover:text-accent transition-colors">
              {band.name}
            </h3>
            <span class="text-[10px] tracking-[0.3em] uppercase text-text-muted">{band.role}</span>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
