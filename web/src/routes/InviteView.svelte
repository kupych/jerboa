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
      // Redirect to band after a brief moment
      setTimeout(() => navigate(`/band/${band.slug}`), 1000);
    } catch (e: any) {
      status = "error";
      error = e.message || "Invalid or expired invite";
    }
  });
</script>

<div class="text-center py-16">
  {#if status === "loading"}
    <div class="text-text-muted text-sm">joining band...</div>
  {:else if status === "success"}
    <div class="text-success text-sm">joined! redirecting...</div>
  {:else}
    <div class="space-y-3">
      <div class="text-danger text-sm">{error}</div>
      <button
        onclick={() => navigate("/")}
        class="text-xs text-accent hover:text-accent-hover transition-colors"
      >
        go home
      </button>
    </div>
  {/if}
</div>
