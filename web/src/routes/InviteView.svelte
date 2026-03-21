<script lang="ts">
  import { onMount } from "svelte";
  import { apiPost } from "../lib/api";
  import { navigate } from "../lib/stores/router";

  let { token }: { token: string } = $props();

  let status = $state<"loading" | "success" | "error">("loading");
  let error = $state("");

  onMount(async () => {
    try {
      const band = await apiPost<{ slug: string }>(`/api/invite/${token}`, {});
      status = "success";
      setTimeout(() => navigate(`/band/${band.slug}`), 1000);
    } catch (e: any) {
      status = "error";
      error = e.message || "Invalid or expired invite";
    }
  });
</script>

<div class="text-center py-20">
  {#if status === "loading"}
    <div class="flex items-center justify-center gap-2 label text-text-muted"><span class="w-1.5 h-1.5 bg-accent/40 animate-pulse"></span><span class="tracking-[0.2em] font-mono">SYS.JOIN</span></div>
  {:else if status === "success"}
    <div class="label text-success">joined! redirecting...</div>
  {:else}
    <div class="space-y-4">
      <div class="label text-danger">{error}</div>
      <button
        onclick={() => navigate("/")}
        class="label text-accent hover:text-accent-hover transition-colors"
      >
        go home
      </button>
    </div>
  {/if}
</div>
