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
  import InviteView from "./routes/InviteView.svelte";

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
    <div class="text-text-muted text-sm tracking-widest uppercase">loading</div>
  </div>
{:else if !$user}
  <Login />
{:else}
  <Layout>
    {#if $route.isInvite}
      <InviteView token={$route.inviteToken!} />
    {:else if $route.isTrack}
      <TrackView slug={$route.bandSlug!} trackId={$route.trackId!} />
    {:else if $route.bandSlug}
      <BandView slug={$route.bandSlug} />
    {:else}
      <Dashboard />
    {/if}
  </Layout>
{/if}
