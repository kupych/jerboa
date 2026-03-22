<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/api";
  import { user } from "../lib/stores/auth";
  import { navigate } from "../lib/stores/router";
  import { formatRelativeTime } from "../lib/utils/format";

  type AdminUser = {
    id: string;
    email: string;
    display_name: string;
    is_admin: boolean;
    created_at: string;
    bands: string;
  };

  type AdminBand = {
    id: string;
    name: string;
    slug: string;
    color_scheme: string;
    created_at: string;
    member_count: number;
    track_count: number;
  };

  type AdminInvite = {
    id: string;
    email: string;
    expires_at: string;
    used_at?: string;
    created_at: string;
    band_name: string;
    created_by_name: string;
    used_by_name?: string;
    status: string;
  };

  type FeedbackItem = {
    id: string;
    body: string;
    page_url: string;
    has_image: boolean;
    status: string;
    created_at: string;
    user: { display_name: string; email: string };
  };

  let users = $state<AdminUser[]>([]);
  let bands = $state<AdminBand[]>([]);
  let invites = $state<AdminInvite[]>([]);
  let feedback = $state<FeedbackItem[]>([]);
  let loading = $state(true);
  let tab = $state<"users" | "bands" | "invites" | "feedback">("users");

  onMount(async () => {
    if (!$user?.is_admin) {
      navigate("/");
      return;
    }
    try {
      const data = await api<{
        users: AdminUser[];
        bands: AdminBand[];
        invites: AdminInvite[];
        feedback: FeedbackItem[];
      }>("/api/admin/overview");
      users = data.users;
      bands = data.bands;
      invites = data.invites;
      feedback = data.feedback;
    } finally {
      loading = false;
    }
  });
</script>

<div class="space-y-6">
  <div class="flex items-center gap-1.5 text-[11px] font-mono font-semibold tracking-[0.2em] text-text-muted/30 uppercase select-none">
    <button onclick={() => navigate("/")} class="hover:text-accent/60 transition-colors py-1">HOME</button>
    <span>/</span>
    <span class="text-text-muted/50">ADMIN</span>
  </div>

  <h1 class="text-xl md:text-2xl font-bold tracking-wider font-display">system admin</h1>

  <!-- Stats bar -->
  {#if !loading}
    <div class="flex flex-wrap gap-3 md:gap-4">
      <div class="bg-bg-surface border border-border px-4 py-3 flex-1 min-w-[120px]">
        <div class="text-lg md:text-xl font-bold text-text-primary font-mono">{users.length}</div>
        <div class="label-sm text-text-muted">users</div>
      </div>
      <div class="bg-bg-surface border border-border px-4 py-3 flex-1 min-w-[120px]">
        <div class="text-lg md:text-xl font-bold text-text-primary font-mono">{bands.length}</div>
        <div class="label-sm text-text-muted">bands</div>
      </div>
      <div class="bg-bg-surface border border-border px-4 py-3 flex-1 min-w-[120px]">
        <div class="text-lg md:text-xl font-bold text-text-primary font-mono">{invites.filter((i) => i.status === "pending").length}</div>
        <div class="label-sm text-text-muted">pending invites</div>
      </div>
      <div class="bg-bg-surface border border-border px-4 py-3 flex-1 min-w-[120px]">
        <div class="text-lg md:text-xl font-bold text-text-primary font-mono">{feedback.filter((f) => f.status === "open").length}</div>
        <div class="label-sm text-text-muted">open feedback</div>
      </div>
    </div>
  {/if}

  <!-- Tabs -->
  <div class="flex items-center gap-4 md:gap-6 border-b border-border pb-2">
    {#each ["users", "bands", "invites", "feedback"] as t}
      <button
        onclick={() => (tab = t as typeof tab)}
        class="label-sm md:label transition-colors pb-2 -mb-2 {tab === t ? 'text-accent border-b-2 border-accent' : 'text-text-muted hover:text-text-secondary'}"
      >
        {t}
        {#if t === "invites"}
          <span class="text-text-muted/50 ml-1">{invites.length}</span>
        {:else if t === "feedback"}
          <span class="text-text-muted/50 ml-1">{feedback.filter((f) => f.status === "open").length}</span>
        {/if}
      </button>
    {/each}
  </div>

  {#if loading}
    <div class="flex items-center gap-2 py-20 justify-center label text-text-muted">
      <span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span>
      <span class="tracking-[0.2em] font-mono">SYS.LOAD</span>
    </div>
  {:else if tab === "users"}
    <div class="space-y-1">
      {#each users as u}
        <div class="bg-bg-surface border border-border p-3 md:p-4 flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-sm font-semibold text-text-primary truncate">{u.display_name || u.email}</span>
              {#if u.is_admin}
                <span class="label-sm text-accent bg-accent/10 px-1.5 py-0.5">admin</span>
              {/if}
            </div>
            <div class="label-sm text-text-muted truncate">{u.email}</div>
          </div>
          <div class="flex items-center gap-3 label-sm text-text-muted shrink-0 flex-wrap">
            {#if u.bands}
              <span class="text-text-secondary">{u.bands}</span>
            {:else}
              <span class="text-text-muted/40 italic">no bands</span>
            {/if}
            <span class="font-mono">{formatRelativeTime(u.created_at)}</span>
          </div>
        </div>
      {/each}
    </div>

  {:else if tab === "bands"}
    <div class="space-y-1">
      {#each bands as b}
        <div
          class="bg-bg-surface border border-border p-3 md:p-4 flex items-center gap-4 hover:border-accent/40 transition-colors cursor-pointer"
          role="button"
          tabindex="0"
          onclick={() => navigate(`/band/${b.slug}`)}
          onkeydown={(e) => { if (e.key === 'Enter') navigate(`/band/${b.slug}`); }}
        >
          <div class="w-3 h-3 shrink-0" style="background: var(--color-accent, #2ec4b6)"></div>
          <div class="flex-1 min-w-0">
            <span class="text-sm font-semibold text-text-primary font-display tracking-wide">{b.name}</span>
            <span class="label-sm text-text-muted ml-2">/{b.slug}</span>
          </div>
          <div class="flex items-center gap-4 label-sm text-text-muted shrink-0">
            <span>{b.member_count} {b.member_count === 1 ? "member" : "members"}</span>
            <span>{b.track_count} {b.track_count === 1 ? "track" : "tracks"}</span>
            <span class="font-mono">{formatRelativeTime(b.created_at)}</span>
          </div>
        </div>
      {/each}
    </div>

  {:else if tab === "invites"}
    <div class="space-y-1">
      {#each invites as inv}
        <div class="bg-bg-surface border border-border p-3 md:p-4 flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-sm font-semibold text-text-primary truncate">{inv.email || "no email"}</span>
              <span class="label-sm px-1.5 py-0.5 {inv.status === 'accepted' ? 'text-green-400 bg-green-400/10' : inv.status === 'pending' ? 'text-amber-400 bg-amber-400/10' : 'text-text-muted bg-bg-primary'}">{inv.status}</span>
            </div>
            <div class="label-sm text-text-muted">
              to <span class="text-text-secondary">{inv.band_name}</span>
              by <span class="text-text-secondary">{inv.created_by_name}</span>
            </div>
          </div>
          <div class="flex items-center gap-3 label-sm text-text-muted shrink-0 flex-wrap">
            {#if inv.used_by_name}
              <span class="text-green-400">joined as {inv.used_by_name}</span>
            {/if}
            <span class="font-mono">{formatRelativeTime(inv.created_at)}</span>
          </div>
        </div>
      {/each}
      {#if invites.length === 0}
        <div class="text-center py-10 label text-text-muted">no invites yet</div>
      {/if}
    </div>

  {:else if tab === "feedback"}
    <div class="space-y-1">
      {#each feedback as fb}
        <div class="bg-bg-surface border border-border p-3 md:p-4">
          <div class="flex flex-col sm:flex-row sm:items-start gap-2 sm:gap-4">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap mb-1">
                <span class="label-sm text-text-secondary">{fb.user.display_name || fb.user.email}</span>
                <span class="label-sm px-1.5 py-0.5 {fb.status === 'open' ? 'text-amber-400 bg-amber-400/10' : 'text-text-muted bg-bg-primary'}">{fb.status}</span>
                {#if fb.has_image}
                  <span class="label-sm text-text-muted/50">has image</span>
                {/if}
              </div>
              <p class="text-sm text-text-primary whitespace-pre-wrap">{fb.body}</p>
              {#if fb.page_url}
                <div class="label-sm text-text-muted/50 mt-1 font-mono">{fb.page_url}</div>
              {/if}
            </div>
            <span class="label-sm text-text-muted font-mono shrink-0">{formatRelativeTime(fb.created_at)}</span>
          </div>
        </div>
      {/each}
      {#if feedback.length === 0}
        <div class="text-center py-10 label text-text-muted">no feedback yet</div>
      {/if}
    </div>
  {/if}
</div>
