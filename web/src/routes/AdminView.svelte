<script lang="ts">
  import { onMount } from "svelte";
  import { api, apiDelete } from "../lib/api";
  import { user } from "../lib/stores/auth";
  import { navigate } from "../lib/stores/router";
  import { formatRelativeTime } from "../lib/utils/format";

  let confirmDeleteUser = $state<string | null>(null);
  let confirmDeleteInvite = $state<string | null>(null);

  async function deleteUser(id: string) {
    await apiDelete(`/api/admin/users/${id}`);
    users = users.filter((u) => u.id !== id);
    confirmDeleteUser = null;
  }

  async function deleteInvite(id: string) {
    await apiDelete(`/api/admin/invites/${id}`);
    invites = invites.filter((i) => i.id !== id);
    confirmDeleteInvite = null;
  }

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

  type EventSession = {
    session_id: string;
    user_id?: string;
    display_name: string;
    email: string;
    first_seen: string;
    last_seen: string;
    event_count: number;
  };

  type AnalyticsEvent = {
    id: number;
    session_id: string;
    user_id?: string;
    kind: string;
    path: string;
    metadata: Record<string, any>;
    created_at: string;
  };

  let users = $state<AdminUser[]>([]);
  let bands = $state<AdminBand[]>([]);
  let invites = $state<AdminInvite[]>([]);
  let feedback = $state<FeedbackItem[]>([]);
  let eventSessions = $state<EventSession[]>([]);
  let eventKindCounts = $state<Record<string, number>>({});
  let selectedSession = $state<string | null>(null);
  let selectedEvents = $state<AnalyticsEvent[]>([]);
  let loadingEvents = $state(false);
  let loading = $state(true);
  let tab = $state<"users" | "bands" | "invites" | "feedback" | "events">("users");

  async function loadEvents() {
    const data = await api<{ sessions: EventSession[]; kind_counts: Record<string, number> }>(
      "/api/admin/events/sessions"
    );
    eventSessions = data.sessions;
    eventKindCounts = data.kind_counts;
  }

  async function openSession(id: string) {
    selectedSession = id;
    loadingEvents = true;
    try {
      selectedEvents = await api<AnalyticsEvent[]>(
        `/api/admin/events/session?id=${encodeURIComponent(id)}`
      );
    } finally {
      loadingEvents = false;
    }
  }

  function closeSession() {
    selectedSession = null;
    selectedEvents = [];
  }

  function describeTarget(m: Record<string, any>): string {
    const t = m?.target;
    if (!t) return "";
    const parts: string[] = [];
    parts.push(t.tag);
    if (t.id) parts.push("#" + t.id);
    if (t.classes) parts.push("." + String(t.classes).split(/\s+/).slice(0, 2).join("."));
    if (t.text) parts.push(`"${t.text}"`);
    return parts.join("");
  }

  $effect(() => {
    if (tab === "events" && eventSessions.length === 0 && !loading) {
      loadEvents();
    }
  });

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
    {#each ["users", "bands", "invites", "feedback", "events"] as t}
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
            {#if u.id !== $user?.id}
              {#if confirmDeleteUser === u.id}
                <button onclick={() => deleteUser(u.id)} class="text-danger hover:text-red-300 transition-colors">confirm</button>
                <button onclick={() => confirmDeleteUser = null} class="text-text-muted hover:text-text-secondary transition-colors">cancel</button>
              {:else}
                <button onclick={() => confirmDeleteUser = u.id} class="text-text-muted/30 hover:text-danger transition-colors">delete</button>
              {/if}
            {/if}
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
            {#if confirmDeleteInvite === inv.id}
              <button onclick={() => deleteInvite(inv.id)} class="text-danger hover:text-red-300 transition-colors">confirm</button>
              <button onclick={() => confirmDeleteInvite = null} class="text-text-muted hover:text-text-secondary transition-colors">cancel</button>
            {:else}
              <button onclick={() => confirmDeleteInvite = inv.id} class="text-text-muted/30 hover:text-danger transition-colors">delete</button>
            {/if}
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

  {:else if tab === "events"}
    {#if selectedSession}
      <div class="space-y-3">
        <button onclick={closeSession} class="label-sm text-text-muted hover:text-text-secondary transition-colors">
          ← back to sessions
        </button>
        <div class="label-sm font-mono text-text-muted/60 break-all">{selectedSession}</div>
        {#if loadingEvents}
          <div class="py-10 text-center label text-text-muted">loading…</div>
        {:else}
          <div class="space-y-0.5">
            {#each selectedEvents as e}
              <div class="bg-bg-surface border border-border px-3 py-2 flex gap-3 items-start text-sm">
                <span class="label-sm font-mono text-text-muted shrink-0 w-20">{new Date(e.created_at).toLocaleTimeString()}</span>
                <span class="label-sm font-mono shrink-0 w-24 {e.kind === 'click_dead' ? 'text-amber-400' : e.kind === 'click' ? 'text-accent' : 'text-text-muted'}">{e.kind}</span>
                <span class="label-sm font-mono text-text-muted shrink-0 max-w-[200px] truncate">{e.path}</span>
                <span class="flex-1 min-w-0 text-text-secondary truncate">{describeTarget(e.metadata)}</span>
                {#if e.metadata?.vx != null}
                  <span class="label-sm font-mono text-text-muted/60 shrink-0">{e.metadata.vx}%,{e.metadata.vy}%</span>
                {/if}
              </div>
            {/each}
            {#if selectedEvents.length === 0}
              <div class="text-center py-10 label text-text-muted">no events in this session</div>
            {/if}
          </div>
        {/if}
      </div>
    {:else}
      <div class="space-y-4">
        {#if Object.keys(eventKindCounts).length > 0}
          <div class="flex flex-wrap gap-2">
            {#each Object.entries(eventKindCounts) as [k, n]}
              <div class="bg-bg-surface border border-border px-3 py-2">
                <div class="font-mono text-sm font-semibold text-text-primary">{n}</div>
                <div class="label-sm text-text-muted">{k}</div>
              </div>
            {/each}
          </div>
        {/if}
        <div class="space-y-1">
          {#each eventSessions as s}
            <button
              onclick={() => openSession(s.session_id)}
              class="w-full text-left bg-bg-surface border border-border p-3 md:p-4 flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4 hover:border-accent/40 transition-colors"
            >
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-sm font-semibold text-text-primary truncate">{s.display_name || s.email || "anonymous"}</span>
                  {#if !s.user_id}
                    <span class="label-sm text-text-muted bg-bg-primary px-1.5 py-0.5">anon</span>
                  {/if}
                </div>
                <div class="label-sm font-mono text-text-muted/50 truncate">{s.session_id}</div>
              </div>
              <div class="flex items-center gap-3 label-sm text-text-muted shrink-0 flex-wrap">
                <span class="font-mono text-text-secondary">{s.event_count} events</span>
                <span class="font-mono">{formatRelativeTime(s.last_seen)}</span>
              </div>
            </button>
          {/each}
          {#if eventSessions.length === 0}
            <div class="text-center py-10 label text-text-muted">no events yet</div>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</div>
