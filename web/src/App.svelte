<script lang="ts">
  import { onMount } from "svelte";
  import { user, authLoading, checkAuth } from "./lib/stores/auth";
  import { route } from "./lib/stores/router";
  import { ws } from "./lib/ws";
  import Layout from "./lib/components/Layout.svelte";
  import Login from "./routes/Login.svelte";
  import Dashboard from "./routes/Dashboard.svelte";
  import BandView from "./routes/BandView.svelte";
  import TrackView from "./routes/TrackView.svelte";
  import SongView from "./routes/SongView.svelte";
  import SetView from "./routes/SetView.svelte";
  import InviteView from "./routes/InviteView.svelte";
  import Profile from "./routes/Profile.svelte";

  onMount(() => {
    checkAuth();
  });

  $effect(() => {
    if ($user) {
      ws.connect();
      return () => ws.disconnect();
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
{:else}
  <Layout>
    {#if $route.isProfile}
      <Profile />
    {:else if $route.isInvite}
      <InviteView token={$route.inviteToken!} />
    {:else if $route.isTrack}
      <TrackView slug={$route.bandSlug!} trackId={$route.trackId!} />
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
