<script lang="ts">
  import Header from "./Header.svelte";
  import ChatPanel from "./ChatPanel.svelte";
  import type { Snippet } from "svelte";

  let { children }: { children: Snippet } = $props();

  let chatOpen = $state(false);
</script>

<div class="min-h-screen flex flex-col">
  <Header onChatToggle={() => (chatOpen = !chatOpen)} {chatOpen} />
  <main class="flex-1 mx-5 my-6 md:mx-12 md:my-10">
    {@render children()}
  </main>
  <ChatPanel bind:open={chatOpen} />
</div>
