<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/api";
  import { ws } from "../lib/ws";
  import TrackCard from "../lib/components/TrackCard.svelte";
  import TrackUpload from "../lib/components/TrackUpload.svelte";

  let { slug }: { slug: string } = $props();

  interface BandDetail {
    band: { id: string; name: string; slug: string };
    members: Array<{ user: { display_name: string; email: string }; role: string }>;
  }

  interface Track {
    id: string;
    title: string;
    description?: string;
    duration_ms: number;
    format: string;
    file_size: number;
    status: string;
    created_at: string;
    uploader?: { display_name: string; email: string };
  }

  let band = $state<BandDetail | null>(null);
  let tracks = $state<Track[]>([]);
  let loading = $state(true);
  let showInviteUrl = $state("");
  let generatingInvite = $state(false);

  onMount(() => {
    loadData();
  });

  $effect(() => {
    if (band) {
      ws.subscribe(`band:${band.band.id}`);
      const off = ws.on("track.ready", () => loadTracks());
      return () => {
        ws.unsubscribe(`band:${band!.band.id}`);
        off();
      };
    }
  });

  async function loadData() {
    loading = true;
    try {
      [band, tracks] = await Promise.all([
        api<BandDetail>(`/api/bands/${slug}`),
        api<Track[]>(`/api/bands/${slug}/tracks`),
      ]);
    } finally {
      loading = false;
    }
  }

  async function loadTracks() {
    tracks = await api<Track[]>(`/api/bands/${slug}/tracks`);
  }

  async function generateInvite() {
    generatingInvite = true;
    try {
      const res = await api<{ invite_url: string }>(`/api/bands/${slug}/invite`, { method: "POST" });
      showInviteUrl = res.invite_url;
    } finally {
      generatingInvite = false;
    }
  }

  async function copyInvite() {
    await navigator.clipboard.writeText(showInviteUrl);
  }
</script>

{#if loading}
  <div class="text-center py-16 text-text-muted text-xs tracking-[0.3em] uppercase">loading</div>
{:else if band}
  <div>
    <!-- Band header -->
    <div class="flex items-start justify-between mb-8">
      <div>
        <h2 class="text-xl font-bold tracking-wider">{band.band.name}</h2>
        <div class="flex items-center gap-2 mt-2">
          {#each band.members as member}
            <span class="text-[10px] tracking-[0.15em] uppercase text-text-muted px-2 py-1 bg-bg-surface border border-border">
              {member.user.display_name || member.user.email}
              {#if member.role === "admin"}
                <span class="text-accent">*</span>
              {/if}
            </span>
          {/each}
        </div>
      </div>

      <button
        onclick={generateInvite}
        disabled={generatingInvite}
        class="text-xs tracking-[0.2em] uppercase text-text-muted hover:text-accent transition-colors flex items-center gap-2"
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
          <circle cx="8.5" cy="7" r="4"/>
          <line x1="20" y1="8" x2="20" y2="14"/>
          <line x1="23" y1="11" x2="17" y2="11"/>
        </svg>
        invite
      </button>
    </div>

    <!-- Invite URL -->
    {#if showInviteUrl}
      <div class="mb-6 bg-bg-surface border border-accent/30 p-4 flex items-center gap-4">
        <input
          type="text"
          value={showInviteUrl}
          readonly
          class="flex-1 bg-transparent text-xs text-text-secondary font-mono outline-none tracking-wider"
        />
        <button
          onclick={copyInvite}
          class="text-xs tracking-[0.2em] uppercase text-accent hover:text-accent-hover shrink-0 transition-colors"
        >
          copy
        </button>
        <button
          onclick={() => (showInviteUrl = "")}
          class="text-xs tracking-[0.2em] uppercase text-text-muted hover:text-text-secondary shrink-0 transition-colors"
        >
          dismiss
        </button>
      </div>
    {/if}

    <!-- Upload -->
    <div class="mb-6">
      <TrackUpload bandSlug={slug} onUploaded={loadTracks} />
    </div>

    <!-- Tracks -->
    <div class="space-y-2">
      {#each tracks as track}
        <TrackCard {track} bandSlug={slug} />
      {/each}

      {#if tracks.length === 0}
        <div class="text-center py-16 text-text-muted text-xs tracking-[0.3em] uppercase">
          no tracks yet
        </div>
      {/if}
    </div>
  </div>
{/if}
