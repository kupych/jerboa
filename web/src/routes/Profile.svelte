<script lang="ts">
  import { user } from "../lib/stores/auth";
  import { apiPatch } from "../lib/api";
  import { navigate } from "../lib/stores/router";

  let displayName = $state($user?.display_name || "");
  let saving = $state(false);
  let saved = $state(false);

  async function save() {
    if (!displayName.trim()) return;
    saving = true;
    try {
      const updated = await apiPatch<{ display_name: string; email: string; is_admin: boolean }>("/auth/me", {
        display_name: displayName.trim(),
      });
      user.update((u) => u ? { ...u, display_name: updated.display_name } : u);
      saved = true;
      setTimeout(() => (saved = false), 2000);
    } finally {
      saving = false;
    }
  }
</script>

<div>
  <button
    onclick={() => navigate("/")}
    class="label text-text-muted hover:text-text-secondary transition-colors mb-8 flex items-center gap-2"
  >
    <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
      <polyline points="15 18 9 12 15 6"/>
    </svg>
    back
  </button>

  <h2 class="text-xl font-bold tracking-wider font-display mb-8">profile</h2>

  <div class="bg-bg-surface border border-border p-6 space-y-5 max-w-md">
    <div>
      <span class="label text-text-muted block mb-2">email</span>
      <div class="text-base text-text-secondary">{$user?.email}</div>
    </div>

    <div>
      <span class="label text-text-muted block mb-2">display name</span>
      <input
        bind:value={displayName}
        type="text"
        class="w-full bg-bg-primary border border-border px-4 py-3 text-base text-text-primary focus:outline-none focus:border-accent transition-colors"
      />
    </div>

    <div class="flex items-center gap-4">
      <button
        onclick={save}
        disabled={saving || !displayName.trim()}
        class="px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
      >
        {saving ? "..." : "save"}
      </button>
      {#if saved}
        <span class="label text-success">saved</span>
      {/if}
    </div>
  </div>
</div>
