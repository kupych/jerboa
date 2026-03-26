<script lang="ts">
  import { navigate } from "../lib/stores/router";
  import { changelog } from "../lib/changelog";
</script>

<div class="max-w-2xl mx-auto space-y-10">
  <div>
    <button
      onclick={() => navigate("/")}
      class="label-sm text-text-muted hover:text-accent transition-colors mb-6"
    >&larr; back</button>
    <h1 class="text-2xl font-display font-bold tracking-wider text-text-primary">changelog</h1>
  </div>

  {#each changelog as entry, i}
    <section class="space-y-4">
      <div class="flex items-center gap-4">
        <h2 class="font-mono font-bold text-lg text-accent tracking-wide">{entry.version}</h2>
        <span class="label-sm text-text-muted font-mono">{entry.date}</span>
        {#if i === 0}
          <span class="label-sm text-accent bg-accent/10 px-2 py-0.5">latest</span>
        {/if}
      </div>
      <div class="border-l-2 border-border pl-5 space-y-2">
        {#each entry.summary as item}
          <div class="flex gap-3 items-start">
            <span class="mt-1.5 shrink-0 w-1 h-1 bg-text-muted/40 rounded-full"></span>
            <span class="text-sm font-medium text-text-secondary">{item}</span>
          </div>
        {/each}
      </div>
    </section>
  {/each}
</div>
