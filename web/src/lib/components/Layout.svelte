<script lang="ts">
  import Header from "./Header.svelte";
  import type { Snippet } from "svelte";

  let { children }: { children: Snippet } = $props();
</script>

<div class="min-h-screen flex flex-col">
  <Header />
  <main class="flex-1 max-w-6xl w-full mx-auto px-6 py-8">
    {@render children()}
  </main>
</div>
