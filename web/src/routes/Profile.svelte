<script lang="ts">
  import { user } from "../lib/stores/auth";
  import { apiPatch } from "../lib/api";
  import { navigate } from "../lib/stores/router";
  import { bands } from "../lib/stores/bands";
  import { themePreference, setTheme, type ThemePreference } from "../lib/stores/theme";

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
  <!-- Breadcrumb -->
  <div class="text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none flex items-center gap-1.5 mb-8">
    <button onclick={() => navigate($bands.length === 1 ? `/band/${$bands[0].slug}` : "/")} class="hover:text-accent/60 transition-colors py-1">{$bands.length === 1 ? $bands[0].slug.toUpperCase() : 'JRB'}</button>
    <span>/</span>
    <span class="text-text-muted/50">PROFILE</span>
  </div>

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

    <div>
      <span class="label text-text-muted block mb-2">theme</span>
      <div class="flex">
        {#each [["dark", "dark"], ["light", "light"], ["system", "system"]] as [value, label]}
          <button
            onclick={() => setTheme(value as ThemePreference)}
            class="px-4 py-2 label-sm border border-border transition-colors -ml-px first:ml-0 {$themePreference === value ? 'bg-accent text-bg-primary border-accent' : 'text-text-muted hover:text-text-secondary'}"
          >{label}</button>
        {/each}
      </div>
    </div>
  </div>
</div>
