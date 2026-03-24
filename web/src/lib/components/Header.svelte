<script lang="ts">
  import { user, logout } from "../stores/auth";
  import { navigate, route } from "../stores/router";
  import { bands, loadBands } from "../stores/bands";
  import { unreadCounts, totalUnread, markBandSeen } from "../stores/notifications";
  import { apiPost } from "../api";
  import ThemeToggle from "./ThemeToggle.svelte";
  import { initials } from "../utils/format";

  let { onChatToggle, chatOpen }: { onChatToggle: () => void; chatOpen: boolean } = $props();

  let bandMenuOpen = $state(false);
  let creatingBand = $state(false);
  let newBandName = $state("");
  let submitting = $state(false);
  let notifOpen = $state(false);

  let currentBand = $derived($bands.find((b) => b.slug === $route.bandSlug));

  function handleLogoClick() {
    if ($route.bandSlug) {
      navigate(`/band/${$route.bandSlug}`);
    } else if ($bands.length === 1) {
      navigate(`/band/${$bands[0].slug}`);
    } else {
      navigate("/");
    }
  }

  function switchBand(slug: string) {
    bandMenuOpen = false;
    creatingBand = false;
    navigate(`/band/${slug}`);
  }

  async function createBand() {
    if (!newBandName.trim() || submitting) return;
    submitting = true;
    try {
      const band = await apiPost<{ slug: string }>("/api/bands", { name: newBandName.trim() });
      newBandName = "";
      creatingBand = false;
      bandMenuOpen = false;
      await loadBands();
      navigate(`/band/${band.slug}`);
    } finally {
      submitting = false;
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
{#if bandMenuOpen}
  <div class="fixed inset-0 z-40" onclick={() => (bandMenuOpen = false)}></div>
{/if}
{#if notifOpen}
  <div class="fixed inset-0 z-40" onclick={() => (notifOpen = false)}></div>
{/if}

<header class="border-b border-border bg-bg-secondary">
  <nav class="flex items-center justify-between h-14 md:h-20 mx-4 md:mx-12">
    <div class="flex items-center gap-3 md:gap-4 min-w-0">
      <button
        onclick={handleLogoClick}
        class="flex items-center gap-2 md:gap-2.5 text-base md:text-lg font-bold tracking-[0.25em] uppercase text-accent hover:text-accent-hover transition-colors font-display shrink-0"
      >
        <div class="h-6 w-6 md:h-8 md:w-8 bg-current" title="jerbert" style="-webkit-mask: url(/logo.svg) center/contain no-repeat; mask: url(/logo.svg) center/contain no-repeat;"></div>
        <span class="hidden md:inline">jerboa</span>
      </button>

      {#if currentBand}
        <span class="text-text-muted/30 font-mono text-sm select-none">/</span>
        <div class="relative min-w-0">
          <button
            onclick={() => (bandMenuOpen = !bandMenuOpen)}
            class="label text-text-secondary hover:text-text-primary transition-colors flex items-center gap-1.5 truncate"
          >
            <span class="truncate">{currentBand.name}</span>
            <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" class="shrink-0 opacity-50">
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </button>
          {#if bandMenuOpen}
            <div class="absolute top-full left-0 mt-2 bg-bg-elevated border border-border z-50 min-w-[200px] py-1">
              {#each $bands as band}
                <button
                  onclick={() => switchBand(band.slug)}
                  class="w-full text-left px-4 py-2.5 label transition-colors {band.slug === $route.bandSlug ? 'text-accent' : 'text-text-secondary hover:text-text-primary hover:bg-bg-surface'}"
                >
                  {band.name}
                </button>
              {/each}
              {#if $user?.is_admin}
                <div class="border-t border-border mt-1 pt-1">
                  {#if creatingBand}
                    <form onsubmit={(e) => { e.preventDefault(); createBand(); }} class="px-3 py-2 flex gap-2">
                      <input
                        bind:value={newBandName}
                        type="text"
                        placeholder="band name"
                        class="flex-1 min-w-0 bg-bg-primary border border-border px-3 py-1.5 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
                      />
                      <button
                        type="submit"
                        disabled={submitting || !newBandName.trim()}
                        class="px-3 py-1.5 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors shrink-0"
                      >
                        {submitting ? "..." : "go"}
                      </button>
                    </form>
                  {:else}
                    <button
                      onclick={() => (creatingBand = true)}
                      class="w-full text-left px-4 py-2.5 label text-accent hover:text-accent-hover hover:bg-bg-surface transition-colors flex items-center gap-2"
                    >
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                        <line x1="12" y1="5" x2="12" y2="19"/>
                        <line x1="5" y1="12" x2="19" y2="12"/>
                      </svg>
                      new band
                    </button>
                  {/if}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      <span class="hidden lg:inline text-[9px] font-mono font-semibold tracking-[0.15em] text-text-muted/20 select-none mt-0.5">SYS·AUD·01</span>
    </div>

    <div class="flex items-center gap-4 md:gap-5">
      {#if $route.bandSlug}
        <button
          onclick={onChatToggle}
          class="flex items-center justify-center w-8 h-8 transition-colors {chatOpen ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
          title="Chat"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
          </svg>
        </button>
      {/if}

      <div class="relative flex items-center">
        <button
          onclick={() => (notifOpen = !notifOpen)}
          class="flex items-center justify-center w-8 h-8 transition-colors {notifOpen ? 'text-accent' : 'text-text-muted hover:text-text-secondary'}"
          title="What's new"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="translate-y-px">
            <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
            <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
          </svg>
          {#if $totalUnread > 0}
            <span class="absolute top-0 right-0 min-w-[16px] h-4 bg-accent text-bg-primary text-[9px] font-bold flex items-center justify-center px-1">
              {$totalUnread > 99 ? "99+" : $totalUnread}
            </span>
          {/if}
        </button>
        {#if notifOpen}
          <div class="absolute top-full right-0 mt-2 bg-bg-elevated border border-border z-50 min-w-[260px] py-1">
            {#each $unreadCounts.filter(c => c.total > 0) as band}
              <button
                onclick={() => { notifOpen = false; markBandSeen(band.band_slug); navigate(`/band/${band.band_slug}`); }}
                class="w-full text-left px-4 py-3 hover:bg-bg-surface transition-colors"
              >
                <div class="label text-text-primary">{$bands.find(b => b.slug === band.band_slug)?.name || band.band_slug}</div>
                <div class="label-sm text-text-muted mt-1 flex gap-2 flex-wrap">
                  {#if band.new_tracks > 0}<span>{band.new_tracks} new track{band.new_tracks > 1 ? 's' : ''}</span>{/if}
                  {#if band.new_comments > 0}<span>{band.new_comments} comment{band.new_comments > 1 ? 's' : ''}</span>{/if}
                  {#if band.new_chats > 0}<span>{band.new_chats} chat{band.new_chats > 1 ? 's' : ''}</span>{/if}
                  {#if band.new_songs > 0}<span>{band.new_songs} new song{band.new_songs > 1 ? 's' : ''}</span>{/if}
                </div>
              </button>
            {:else}
              <div class="px-4 py-6 text-center label-sm text-text-muted">all caught up</div>
            {/each}
          </div>
        {/if}
      </div>

      {#if $user?.is_admin}
        <button
          onclick={() => navigate("/admin")}
          class="flex items-center justify-center w-8 h-8 text-text-muted hover:text-text-secondary transition-colors"
          title="Admin"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
          </svg>
        </button>
      {/if}

      <ThemeToggle />

      {#if $user}
        <div class="flex items-center gap-2 md:gap-3">
          <button onclick={() => navigate("/profile")} class="cursor-pointer" title="Profile">
            {#if $user.avatar_url}
              <img
                src={$user.avatar_url}
                alt=""
                class="w-7 h-7 md:w-8 md:h-8"
              />
            {:else}
              <div
                class="w-7 h-7 md:w-8 md:h-8 bg-accent/15 text-accent text-[10px] md:text-[11px] flex items-center justify-center font-bold tracking-wider hover:bg-accent/25 transition-colors"
              >
                {initials($user.display_name || $user.email)}
              </div>
            {/if}
          </button>
          <button
            onclick={() => logout()}
            class="label text-text-muted hover:text-text-secondary transition-colors hidden md:block"
          >
            out
          </button>
        </div>
      {/if}
    </div>
  </nav>
</header>
