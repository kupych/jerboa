<script lang="ts">
  import { onMount } from "svelte";
  import Header from "./Header.svelte";
  import ChatPanel from "./ChatPanel.svelte";
  import KeyboardHelp from "./KeyboardHelp.svelte";
  import FeedbackButton from "./FeedbackButton.svelte";
  import WhatsNewModal from "./WhatsNewModal.svelte";
  import PersistentPlayer from "./PersistentPlayer.svelte";
  import { globalPlayer, playerState } from "../stores/globalPlayer";
  import { isDemo } from "../stores/auth";
  import { layoutWidth, widthClass } from "../stores/layoutWidth";
  import type { Snippet } from "svelte";

  let { children }: { children: Snippet } = $props();

  let chatOpen = $state(false);
  let audioEl = $state<HTMLAudioElement | null>(null);

  onMount(() => {
    if (audioEl) globalPlayer.setAudioElement(audioEl);
  });
</script>

<div class="min-h-screen flex flex-col">
  {#if $isDemo}
    <div class="bg-accent/10 border-b border-accent/20 px-5 md:px-12 py-1.5 text-center">
      <span class="text-[10px] font-bold tracking-[0.2em] uppercase text-accent/80">demo mode · read-only</span>
    </div>
  {/if}
  <Header onChatToggle={() => (chatOpen = !chatOpen)} {chatOpen} />
  <main class="flex-1 px-5 my-6 md:px-12 md:my-10 w-full {widthClass[$layoutWidth]} mx-auto {$playerState.track ? 'pb-20' : 'pb-8'}">
    {@render children()}
  </main>
  <div class="px-5 md:px-12 pb-3">
    <span class="text-[8px] font-semibold tracking-[0.25em] text-text-muted/10 uppercase select-none font-mono">A part of the Whether Network</span>
  </div>
  <ChatPanel bind:open={chatOpen} />
  <FeedbackButton />
</div>
<KeyboardHelp />
<WhatsNewModal />
<PersistentPlayer />
<audio bind:this={audioEl} class="hidden"></audio>
