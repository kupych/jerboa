<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiPost } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { user, isDemo } from "../lib/stores/auth";
  import { initials } from "../lib/utils/format";
  import { bands as bandsStore, loadBands as refreshBands } from "../lib/stores/bands";

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
    // Auto-redirect if user is in exactly one band
    if (bands.length === 1) {
      navigate(`/band/${bands[0].slug}`);
    }
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
  <!-- Section header -->
  <div class="sys-title-block mb-8">
    <div class="flex items-start justify-between gap-6">
      <div>
        <div class="sys-kicker mb-3">switchboard // band index</div>
        <div class="flex items-end gap-4 flex-wrap">
          <div class="sys-mega text-accent/90">{String(bands.length).padStart(2, "0")}</div>
          <div class="pb-1">
            <h2 class="label text-text-secondary font-display">bands</h2>
            <p class="label-sm text-text-muted/50 mt-1">active workspaces you can jump into</p>
          </div>
        </div>
      </div>
      {#if $user?.is_admin}
        <button
          onclick={() => (showCreate = !showCreate)}
          class="label text-accent hover:text-accent-hover transition-colors flex items-center gap-2 pt-1"
        >
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
            <line x1="12" y1="5" x2="12" y2="19"/>
            <line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
          new band
        </button>
      {/if}
    </div>
  </div>

  {#if showCreate}
    <form
      onsubmit={(e) => { e.preventDefault(); createBand(); }}
      class="mb-10 bg-bg-surface border border-border p-8"
    >
      <label class="block">
        <span class="label text-text-muted block mb-3">band name</span>
        <input
          bind:value={newBandName}
          type="text"
          placeholder="e.g. The Wavelengths"
          class="w-full bg-bg-primary border border-border px-5 py-3 text-base text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
        />
      </label>
      <div class="flex gap-4 mt-6">
        <button
          type="submit"
          disabled={creating || !newBandName.trim() || $isDemo}
          title={$isDemo ? "demo account is read-only" : ""}
          class="px-8 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
        >
          {$isDemo ? "demo: read-only" : creating ? "..." : "create"}
        </button>
        <button
          type="button"
          onclick={() => (showCreate = false)}
          class="px-8 py-3 label text-text-muted hover:text-text-secondary transition-colors"
        >
          cancel
        </button>
      </div>
    </form>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center gap-2 py-20 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
  {:else if bands.length === 0}
    <div class="sys-empty">
      <div class="sys-empty-code">BND // 00</div>
      <div class="sys-empty-title text-text-primary mt-3">No Bands</div>
      <p class="sys-empty-copy mt-3">Create a workspace or join one through an invite link. This is the switchboard, so keep it clean and obvious.</p>
      {#if $user?.is_admin}
        <div class="mt-5">
          <button
            onclick={() => (showCreate = true)}
            class="px-5 py-2.5 bg-accent hover:bg-accent-hover text-bg-primary label-sm transition-colors"
          >
            create band
          </button>
        </div>
      {/if}
    </div>
  {:else}
    <div class="grid gap-3">
      {#each bands as band}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="group flex items-stretch bg-bg-surface border border-border hover:border-accent/50 cursor-pointer transition-colors"
          onclick={() => navigate(`/band/${band.slug}`)}
        >
          <!-- Monogram -->
          <div class="w-14 shrink-0 flex items-center justify-center border-r border-border/60 group-hover:border-accent/20 group-hover:bg-accent/[0.04] transition-colors">
            <span class="text-xs font-bold font-display tracking-widest text-accent/40 group-hover:text-accent/70 transition-colors select-none">
              {initials(band.name)}
            </span>
          </div>
          <!-- Name + role -->
          <div class="flex-1 min-w-0 px-5 py-4">
            <div class="text-xl font-bold font-display tracking-wider text-text-primary group-hover:text-accent transition-colors truncate">
              {band.name}
            </div>
            <div class="mt-0.5 label-sm text-text-muted/50">{band.role}</div>
          </div>
          <!-- Chevron -->
          <div class="flex items-center pr-4 text-text-muted/20 group-hover:text-accent/50 transition-colors">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
              <polyline points="9 18 15 12 9 6"/>
            </svg>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
