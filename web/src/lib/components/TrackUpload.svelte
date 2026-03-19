<script lang="ts">
  import { uploadFile } from "../api";

  let { bandSlug, onUploaded }: {
    bandSlug: string;
    onUploaded: () => void;
  } = $props();

  let isDragging = $state(false);
  let uploading = $state(false);
  let progress = $state(0);
  let error = $state("");
  let fileInput = $state<HTMLInputElement | null>(null);

  const ACCEPTED = ".mp3,.wav,.flac,.ogg,.aac,.m4a,.aiff,.aif,.opus";

  function handleDrop(e: DragEvent) {
    e.preventDefault();
    isDragging = false;
    const file = e.dataTransfer?.files[0];
    if (file) upload(file);
  }

  function handleSelect(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (file) upload(file);
  }

  async function upload(file: File) {
    error = "";
    uploading = true;
    progress = 0;

    try {
      await uploadFile(
        `/api/bands/${bandSlug}/tracks`,
        file,
        {},
        (pct) => (progress = pct),
      );
      onUploaded();
    } catch (e: any) {
      error = e.message || "Upload failed";
    } finally {
      uploading = false;
      if (fileInput) fileInput.value = "";
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="border border-dashed p-10 text-center transition-colors
    {isDragging ? 'border-accent bg-accent-muted' : 'border-border hover:border-text-muted'}"
  ondragover={(e) => { e.preventDefault(); isDragging = true; }}
  ondragleave={() => (isDragging = false)}
  ondrop={handleDrop}
>
  {#if uploading}
    <div class="space-y-4">
      <div class="text-sm tracking-[0.2em] uppercase text-text-secondary">uploading</div>
      <div class="w-full bg-bg-primary h-1">
        <div
          class="bg-accent h-1 transition-all duration-300"
          style="width: {progress}%"
        ></div>
      </div>
      <div class="text-xs font-mono text-text-muted">{progress}%</div>
    </div>
  {:else}
    <div class="space-y-3">
      <svg class="mx-auto text-text-muted" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
        <polyline points="17 8 12 3 7 8"/>
        <line x1="12" y1="3" x2="12" y2="15"/>
      </svg>
      <div class="text-sm text-text-secondary tracking-wider">
        drop audio or
        <button
          onclick={() => fileInput?.click()}
          class="text-accent hover:text-accent-hover underline underline-offset-2"
        >browse</button>
      </div>
      <div class="text-[10px] tracking-[0.2em] uppercase text-text-muted">
        mp3 / wav / flac / ogg / aac / m4a / aiff / opus
      </div>
    </div>
    <input
      bind:this={fileInput}
      type="file"
      accept={ACCEPTED}
      class="hidden"
      onchange={handleSelect}
    />
  {/if}

  {#if error}
    <div class="mt-3 text-xs text-danger tracking-wider">{error}</div>
  {/if}
</div>
