<script lang="ts">
  import { uploadFile, apiPatch } from "../api";
  import { isDemo } from "../stores/auth";

  let { bandSlug, songId, onUploaded }: {
    bandSlug: string;
    songId?: string;
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
    if ($isDemo) return;
    const file = e.dataTransfer?.files[0];
    if (file) upload(file);
  }

  function handleSelect(e: Event) {
    if ($isDemo) return;
    const file = (e.target as HTMLInputElement).files?.[0];
    if (file) upload(file);
  }

  async function upload(file: File) {
    error = "";
    uploading = true;
    progress = 0;

    try {
      const track = await uploadFile<{ id: string }>(
        `/api/bands/${bandSlug}/tracks`,
        file,
        {},
        (pct) => (progress = pct),
      );
      if (songId) {
        await apiPatch(`/api/bands/${bandSlug}/tracks/${track.id}/song`, { song_id: songId });
      }
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
  class="border border-dashed p-3 md:py-2 text-center transition-colors
    {isDragging ? 'border-accent bg-accent/5' : 'border-border hover:border-accent/40 hover:bg-accent/[0.02]'}"
  ondragover={(e) => { e.preventDefault(); isDragging = true; }}
  ondragleave={() => (isDragging = false)}
  ondrop={handleDrop}
>
  {#if uploading}
    <div class="flex items-center gap-4">
      <div class="flex-1 bg-bg-primary h-1">
        <div
          class="bg-accent h-1 transition-all duration-300"
          style="width: {progress}%"
        ></div>
      </div>
      <span class="label-sm font-mono text-text-muted">{progress}%</span>
    </div>
  {:else}
    <div class="flex items-center justify-center gap-2">
      <svg class="text-text-muted" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
        <polyline points="17 8 12 3 7 8"/>
        <line x1="12" y1="3" x2="12" y2="15"/>
      </svg>
      <span class="text-sm text-text-muted font-semibold">
        {#if $isDemo}
          demo: uploads disabled
        {:else}
          drop audio or
          <button
            onclick={() => fileInput?.click()}
            class="text-accent hover:text-accent-hover underline underline-offset-4"
          >browse</button>
        {/if}
      </span>
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
    <div class="mt-3 label-sm text-danger">{error}</div>
  {/if}
</div>
