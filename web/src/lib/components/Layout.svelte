<script lang="ts">
  import Header from "./Header.svelte";
  import ChatPanel from "./ChatPanel.svelte";
  import KeyboardHelp from "./KeyboardHelp.svelte";
  import type { Snippet } from "svelte";

  let { children }: { children: Snippet } = $props();

  let chatOpen = $state(false);
</script>

<div class="min-h-screen flex flex-col">
  <Header onChatToggle={() => (chatOpen = !chatOpen)} {chatOpen} />
  <main class="flex-1 mx-5 my-6 md:mx-12 md:my-10 pb-8">
    {@render children()}
  </main>
  <div class="px-5 md:px-12 pb-3">
    <span class="text-[8px] font-semibold tracking-[0.25em] text-text-muted/15 uppercase select-none font-mono">whether.network</span>
  </div>
  <ChatPanel bind:open={chatOpen} />
</div>
<KeyboardHelp />
