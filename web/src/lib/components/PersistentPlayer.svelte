<script lang="ts">
  import { playerState, globalPlayer } from "../stores/globalPlayer";
  import { navigate } from "../stores/router";

  function formatTime(sec: number): string {
    if (!sec || !isFinite(sec)) return "0:00";
    const m = Math.floor(sec / 60);
    const s = Math.floor(sec % 60);
    return `${m}:${s.toString().padStart(2, "0")}`;
  }

  let collapsed = $state(false);
  let showQueue = $state(false);

  function handleBarClick(e: MouseEvent) {
    const bar = e.currentTarget as HTMLElement;
    const rect = bar.getBoundingClientRect();
    const frac = (e.clientX - rect.left) / rect.width;
    globalPlayer.seekFraction(Math.max(0, Math.min(1, frac)));
  }

  let upcomingCount = $derived(
    $playerState.queue.length > 0
      ? $playerState.queue.length - $playerState.queueIndex - 1
      : 0
  );
</script>

{#if $playerState.track}
  <div class="fixed bottom-0 left-0 right-0 z-40 bg-bg-surface border-t border-border">
    <!-- Progress bar -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="h-1 bg-bg-primary cursor-pointer group/bar"
      onclick={handleBarClick}
    >
      <div
        class="h-full bg-accent group-hover/bar:h-1.5 transition-[height]"
        style="width: {$playerState.duration > 0 ? ($playerState.currentTime / $playerState.duration) * 100 : 0}%"
      ></div>
    </div>

    {#if !collapsed}
      <div class="flex items-center gap-1 sm:gap-2 px-3 sm:px-4 py-1.5">
        <!-- Transport: prev / play / next -->
        <button
          onclick={() => globalPlayer.prev()}
          class="w-7 h-7 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors shrink-0"
          title="Previous"
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
            <rect x="3" y="5" width="3" height="14"/>
            <polygon points="21,5 9,12 21,19"/>
          </svg>
        </button>

        <button
          onclick={() => globalPlayer.toggle()}
          class="w-8 h-8 flex items-center justify-center text-text-primary hover:text-accent transition-colors shrink-0"
        >
          {#if $playerState.loading}
            <div class="w-3.5 h-3.5 border-2 border-accent border-t-transparent rounded-full animate-spin"></div>
          {:else if $playerState.playing}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="4" width="4" height="16"/>
              <rect x="14" y="4" width="4" height="16"/>
            </svg>
          {:else}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="5,3 19,12 5,21"/>
            </svg>
          {/if}
        </button>

        <button
          onclick={() => globalPlayer.next()}
          class="w-7 h-7 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors shrink-0"
          title="Next"
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
            <polygon points="3,5 15,12 3,19"/>
            <rect x="18" y="5" width="3" height="14"/>
          </svg>
        </button>

        <!-- Track info -->
        <div class="flex-1 min-w-0 mx-1 sm:mx-2">
          <button
            onclick={() => navigate(`/band/${$playerState.track!.bandSlug}/track/${$playerState.track!.id}`)}
            class="text-sm font-semibold text-text-primary hover:text-accent transition-colors truncate block text-left font-display tracking-wide"
          >
            {$playerState.track.title}
          </button>
          {#if $playerState.track.songName || $playerState.track.bandName}
            <span class="label-sm text-text-muted/50 truncate block">
              {$playerState.track.songName ?? ""}{$playerState.track.songName && $playerState.track.bandName ? " · " : ""}{$playerState.track.bandName ?? ""}
            </span>
          {/if}
        </div>

        <!-- Time -->
        <span class="label-sm text-text-muted font-mono shrink-0 hidden sm:block">
          {formatTime($playerState.currentTime)}<span class="text-text-muted/30 mx-0.5">/</span>{formatTime($playerState.duration)}
        </span>

        <!-- Loop -->
        <button
          onclick={() => globalPlayer.cycleLoop()}
          class="w-7 h-7 items-center justify-center transition-colors shrink-0 hidden sm:flex relative
            {$playerState.loop !== 'off' ? 'text-accent' : 'text-text-muted/40 hover:text-text-muted'}"
          title="Loop: {$playerState.loop}"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="17 1 21 5 17 9"/>
            <path d="M3 11V9a4 4 0 0 1 4-4h14"/>
            <polyline points="7 23 3 19 7 15"/>
            <path d="M21 13v2a4 4 0 0 1-4 4H3"/>
          </svg>
          {#if $playerState.loop === "one"}
            <span class="absolute text-[7px] font-bold leading-none" style="top: 2px; right: 2px;">1</span>
          {/if}
        </button>

        <!-- Shuffle -->
        <button
          onclick={() => globalPlayer.toggleShuffle()}
          class="w-7 h-7 items-center justify-center transition-colors shrink-0 hidden sm:flex
            {$playerState.shuffle ? 'text-accent' : 'text-text-muted/40 hover:text-text-muted'}"
          title="Shuffle"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="16 3 21 3 21 8"/>
            <line x1="4" y1="20" x2="21" y2="3"/>
            <polyline points="21 16 21 21 16 21"/>
            <line x1="15" y1="15" x2="21" y2="21"/>
            <line x1="4" y1="4" x2="9" y2="9"/>
          </svg>
        </button>

        <!-- Queue toggle -->
        <button
          onclick={() => (showQueue = !showQueue)}
          class="w-7 h-7 flex items-center justify-center transition-colors shrink-0 relative
            {showQueue ? 'text-accent' : 'text-text-muted/40 hover:text-text-muted'}"
          title="Queue ({$playerState.queue.length})"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="8" y1="6" x2="21" y2="6"/>
            <line x1="8" y1="12" x2="21" y2="12"/>
            <line x1="8" y1="18" x2="21" y2="18"/>
            <line x1="3" y1="6" x2="3.01" y2="6"/>
            <line x1="3" y1="12" x2="3.01" y2="12"/>
            <line x1="3" y1="18" x2="3.01" y2="18"/>
          </svg>
          {#if upcomingCount > 0}
            <span class="absolute -top-0.5 -right-0.5 w-3.5 h-3.5 bg-accent text-bg-primary text-[8px] font-bold rounded-full flex items-center justify-center">
              {upcomingCount > 9 ? "9+" : upcomingCount}
            </span>
          {/if}
        </button>

        <!-- Collapse -->
        <button
          onclick={() => (collapsed = true)}
          class="w-6 h-6 flex items-center justify-center text-text-muted/30 hover:text-text-muted transition-colors shrink-0"
          title="Minimize"
        >
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>

        <!-- Close -->
        <button
          onclick={() => { showQueue = false; globalPlayer.stop(); }}
          class="w-6 h-6 flex items-center justify-center text-text-muted/30 hover:text-red-400 transition-colors shrink-0"
          title="Close"
        >
          <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
            <line x1="18" y1="6" x2="6" y2="18"/>
            <line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>

      <!-- Queue panel -->
      {#if showQueue}
        <div class="border-t border-border max-h-64 overflow-y-auto">
          {#if $playerState.queue.length === 0}
            <div class="px-4 py-6 text-center label-sm text-text-muted/40">queue is empty</div>
          {:else}
            <div class="px-2 py-1">
              {#each $playerState.queue as item, i}
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="flex items-center gap-2 px-2 py-1.5 rounded transition-colors cursor-pointer group/qi
                    {i === $playerState.queueIndex ? 'bg-accent/10' : 'hover:bg-bg-primary/50'}"
                  onclick={() => {
                    state: { /* use loadTrackAtIndex via play */ }
                    // Play this specific queue item
                    globalPlayer.play(item);
                  }}
                >
                  <span class="label-sm font-mono w-5 text-right shrink-0
                    {i === $playerState.queueIndex ? 'text-accent' : 'text-text-muted/30'}">
                    {#if i === $playerState.queueIndex && $playerState.playing}
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" class="text-accent inline">
                        <polygon points="5,3 19,12 5,21"/>
                      </svg>
                    {:else}
                      {i + 1}
                    {/if}
                  </span>
                  <span class="text-sm font-semibold truncate flex-1
                    {i === $playerState.queueIndex ? 'text-accent' : 'text-text-primary'}">
                    {item.title}
                  </span>
                  {#if item.songName}
                    <span class="label-sm text-text-muted/40 truncate shrink-0">{item.songName}</span>
                  {/if}
                  {#if i !== $playerState.queueIndex}
                    <button
                      onclick={(e) => { e.stopPropagation(); globalPlayer.removeFromQueue(i); }}
                      class="w-5 h-5 flex items-center justify-center text-text-muted/0 group-hover/qi:text-text-muted/40 hover:!text-red-400 transition-colors shrink-0"
                    >
                      <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                        <line x1="18" y1="6" x2="6" y2="18"/>
                        <line x1="6" y1="6" x2="18" y2="18"/>
                      </svg>
                    </button>
                  {/if}
                </div>
              {/each}
            </div>
            {#if $playerState.queue.length > 1}
              <div class="px-4 py-2 border-t border-border/50">
                <button
                  onclick={() => globalPlayer.clearQueue()}
                  class="label-sm text-text-muted/40 hover:text-text-muted transition-colors"
                >clear queue</button>
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    {:else}
      <!-- Collapsed: minimal bar -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="flex items-center gap-2.5 px-3 py-0.5 cursor-pointer"
        onclick={() => (collapsed = false)}
      >
        <button
          onclick={(e) => { e.stopPropagation(); globalPlayer.toggle(); }}
          class="text-text-muted/50 hover:text-accent transition-colors shrink-0"
        >
          {#if $playerState.playing}
            <svg width="9" height="9" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="4" width="4" height="16"/>
              <rect x="14" y="4" width="4" height="16"/>
            </svg>
          {:else}
            <svg width="9" height="9" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="5,3 19,12 5,21"/>
            </svg>
          {/if}
        </button>
        <span class="label-sm text-text-muted/50 truncate">{$playerState.track.title}</span>
        <span class="label-sm text-text-muted/25 font-mono ml-auto shrink-0 hidden sm:block">{formatTime($playerState.currentTime)}</span>
        <button
          onclick={(e) => { e.stopPropagation(); collapsed = false; }}
          class="text-text-muted/25 hover:text-text-muted/50 transition-colors shrink-0"
          title="Expand"
        >
          <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
            <polyline points="18 15 12 9 6 15"/>
          </svg>
        </button>
      </div>
    {/if}
  </div>
{/if}
