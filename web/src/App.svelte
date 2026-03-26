<script lang="ts">
  import { onMount } from "svelte";
  import { user, authLoading, checkAuth } from "./lib/stores/auth";
  import { apiPatch } from "./lib/api";
  import { route, navigate } from "./lib/stores/router";
  import { bands, loadBands } from "./lib/stores/bands";
  import { loadUnread, totalUnread } from "./lib/stores/notifications";
  import { applyColorScheme, resetColorScheme } from "./lib/colorSchemes";
  import { ws } from "./lib/ws";
  import Layout from "./lib/components/Layout.svelte";
  import Login from "./routes/Login.svelte";
  import Dashboard from "./routes/Dashboard.svelte";
  import BandView from "./routes/BandView.svelte";
  import TrackView from "./routes/TrackView.svelte";
  import SongView from "./routes/SongView.svelte";
  import SetView from "./routes/SetView.svelte";
  import InviteView from "./routes/InviteView.svelte";
  import CarMode from "./routes/CarMode.svelte";
  import Profile from "./routes/Profile.svelte";
  import AdminView from "./routes/AdminView.svelte";
  import PerformView from "./routes/PerformView.svelte";
  import Changelog from "./routes/Changelog.svelte";

  let onboardName = $state("");
  let onboardSaving = $state(false);
  let needsOnboarding = $derived($user != null && !$user.display_name);

  async function saveOnboardName() {
    if (!onboardName.trim()) return;
    onboardSaving = true;
    try {
      const updated = await apiPatch<{ display_name: string }>("/auth/me", {
        display_name: onboardName.trim(),
      });
      user.update((u) => u ? { ...u, display_name: updated.display_name } : u);
    } finally {
      onboardSaving = false;
    }
  }

  onMount(() => {
    checkAuth();
  });

  $effect(() => {
    if ($user) {
      ws.connect();
      loadBands();
      loadUnread();
      const off = ws.on("activity.update", () => loadUnread());
      return () => { ws.disconnect(); off(); };
    }
  });

  // Subscribe to all band channels for activity updates
  $effect(() => {
    if ($user && $bands.length > 0) {
      for (const band of $bands) {
        ws.subscribe(`band:${band.id}`);
      }
      return () => {
        for (const band of $bands) {
          ws.unsubscribe(`band:${band.id}`);
        }
      };
    }
  });

  // PWA badge
  $effect(() => {
    const count = $totalUnread;
    if ("setAppBadge" in navigator) {
      if (count > 0) {
        (navigator as any).setAppBadge(count);
      } else {
        (navigator as any).clearAppBadge();
      }
    }
  });

  // Apply band color scheme when navigating between bands
  $effect(() => {
    const slug = $route.bandSlug;
    if (slug && $bands.length > 0) {
      const band = $bands.find((b) => b.slug === slug);
      if (band) {
        applyColorScheme(band.color_scheme);
      }
    } else {
      resetColorScheme();
    }
  });
</script>

{#if $authLoading}
  <div class="flex items-center justify-center min-h-screen">
    <div class="flex items-center gap-2 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.LOAD</span></div>
  </div>
{:else if $route.isNoAccess}
  <div class="flex flex-col items-center justify-center min-h-screen gap-4">
    <div class="label text-danger">access denied</div>
    <div class="text-sm text-text-muted">you need an invite to join jerboa</div>
  </div>
{:else if !$user}
  <Login />
{:else if needsOnboarding}
  <div class="min-h-screen flex flex-col items-center justify-center px-6">
    <div class="w-full max-w-sm space-y-6">
      <div class="flex justify-center">
        <div class="w-16 h-16 text-accent" title="jerbert" style="-webkit-mask: url(/logo.svg) center/contain no-repeat; mask: url(/logo.svg) center/contain no-repeat; background: currentColor;"></div>
      </div>
      <div class="text-center">
        <h2 class="text-lg font-bold tracking-wider font-display text-text-primary">welcome to jerboa</h2>
        <p class="text-xs font-semibold text-text-muted mt-2">what should we call you?</p>
      </div>
      <form onsubmit={(e) => { e.preventDefault(); saveOnboardName(); }} class="space-y-4">
        <input
          type="text"
          bind:value={onboardName}
          placeholder="your name"
          autofocus
          class="w-full bg-transparent border border-border text-text-primary text-sm font-semibold px-4 py-3 placeholder:text-text-muted/30 focus:border-accent focus:outline-none transition-colors text-center"
        />
        <button
          type="submit"
          disabled={onboardSaving || !onboardName.trim()}
          class="w-full px-6 py-3 bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary label transition-colors"
        >
          {onboardSaving ? "..." : "let's go"}
        </button>
      </form>
    </div>
  </div>
{:else}
  <Layout>
    {#if $route.isChangelog}
      <Changelog />
    {:else if $route.isAdmin}
      <AdminView />
    {:else if $route.isProfile}
      <Profile />
    {:else if $route.isInvite}
      <InviteView token={$route.inviteToken!} />
    {:else if $route.isTrack}
      <TrackView slug={$route.bandSlug!} trackId={$route.trackId!} />
    {:else if $route.isCar}
      <CarMode slug={$route.bandSlug!} />
    {:else if $route.isPerform}
      <PerformView slug={$route.bandSlug!} setId={$route.setId ?? undefined} />
    {:else if $route.isSet}
      <SetView slug={$route.bandSlug!} setId={$route.setId!} />
    {:else if $route.isSong}
      <SongView slug={$route.bandSlug!} songId={$route.songId!} />
    {:else if $route.bandSlug}
      <BandView slug={$route.bandSlug} />
    {:else}
      <Dashboard />
    {/if}
  </Layout>
{/if}
