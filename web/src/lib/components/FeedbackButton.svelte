<script lang="ts">
  import { uploadFile } from "../api";

  let open = $state(false);
  let body = $state("");
  let submitting = $state(false);
  let screenshot: Blob | null = $state(null);
  let screenshotUrl: string | null = $state(null);
  let capturing = $state(false);
  let submitted = $state(false);

  async function captureAndOpen() {
    capturing = true;
    try {
      const { default: html2canvas } = await import("html2canvas");
      const canvas = await html2canvas(document.body, {
        backgroundColor: "#131313",
        scale: 1,
        logging: false,
        useCORS: true,
      });
      screenshot = await new Promise<Blob | null>((resolve) =>
        canvas.toBlob(resolve, "image/png"),
      );
      if (screenshot) {
        screenshotUrl = URL.createObjectURL(screenshot);
      }
    } catch {
      // Screenshot failed, proceed without it
    }
    capturing = false;
    open = true;
  }

  function close() {
    open = false;
    body = "";
    submitted = false;
    if (screenshotUrl) {
      URL.revokeObjectURL(screenshotUrl);
      screenshotUrl = null;
    }
    screenshot = null;
  }

  function removeScreenshot() {
    if (screenshotUrl) {
      URL.revokeObjectURL(screenshotUrl);
      screenshotUrl = null;
    }
    screenshot = null;
  }

  async function submit() {
    if (!body.trim() || submitting) return;
    submitting = true;
    try {
      const file = screenshot
        ? new File([screenshot], "screenshot.png", { type: "image/png" })
        : null;

      if (file) {
        await uploadFile("/api/feedback", file, {
          body: body.trim(),
          page_url: window.location.pathname,
        });
      } else {
        const form = new FormData();
        form.append("body", body.trim());
        form.append("page_url", window.location.pathname);
        await fetch("/api/feedback", {
          method: "POST",
          credentials: "include",
          body: form,
        });
      }
      submitted = true;
      setTimeout(close, 1500);
    } finally {
      submitting = false;
    }
  }
</script>

<button
  onclick={captureAndOpen}
  disabled={capturing}
  class="fixed bottom-4 right-4 z-30 bg-bg-elevated border border-border hover:border-accent/50 text-text-muted hover:text-accent transition-colors px-3 py-2 label flex items-center gap-1.5"
  title="Send feedback"
>
  {#if capturing}
    <svg class="animate-spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M12 2v4m0 12v4m-7.07-3.93l2.83-2.83m8.48-8.48l2.83-2.83M2 12h4m12 0h4m-3.93 7.07l-2.83-2.83M7.76 7.76L4.93 4.93"/>
    </svg>
  {:else}
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M12 20h9"/>
      <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
    </svg>
  {/if}
  <span class="hidden sm:inline">feedback</span>
</button>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-50 bg-black/60 flex items-end sm:items-center justify-center p-0 sm:p-6" onclick={close}>
    <div
      class="bg-bg-secondary border border-border w-full sm:max-w-lg max-h-[85vh] flex flex-col"
      onclick={(e) => e.stopPropagation()}
    >
      <div class="flex items-center justify-between px-5 py-4 border-b border-border">
        <h2 class="label text-text-primary">send feedback</h2>
        <button onclick={close} class="text-text-muted hover:text-text-secondary transition-colors">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="18" y1="6" x2="6" y2="18"/>
            <line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>

      {#if submitted}
        <div class="px-5 py-10 text-center">
          <p class="label text-accent">sent! thanks for the feedback.</p>
        </div>
      {:else}
        <div class="flex-1 overflow-y-auto px-5 py-4 flex flex-col gap-4">
          <textarea
            bind:value={body}
            placeholder="what's on your mind? bugs, ideas, vibes..."
            class="w-full min-h-[100px] bg-bg-primary border border-border px-4 py-3 text-sm text-text-primary placeholder:text-text-muted resize-y focus:outline-none focus:border-accent transition-colors"
          ></textarea>

          {#if screenshotUrl}
            <div class="relative">
              <p class="label text-text-muted mb-2">screenshot attached</p>
              <div class="relative group">
                <img src={screenshotUrl} alt="Screenshot" class="w-full border border-border max-h-48 object-cover object-top" />
                <button
                  onclick={removeScreenshot}
                  class="absolute top-2 right-2 bg-bg-elevated/90 border border-border p-1 text-text-muted hover:text-text-primary transition-colors opacity-0 group-hover:opacity-100"
                  title="Remove screenshot"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="18" y1="6" x2="6" y2="18"/>
                    <line x1="6" y1="6" x2="18" y2="18"/>
                  </svg>
                </button>
              </div>
            </div>
          {:else}
            <p class="text-[11px] font-semibold text-text-muted/50">screenshot removed</p>
          {/if}
        </div>

        <div class="px-5 py-4 border-t border-border flex justify-end gap-3">
          <button onclick={close} class="label text-text-muted hover:text-text-secondary transition-colors px-4 py-2">
            cancel
          </button>
          <button
            onclick={submit}
            disabled={!body.trim() || submitting}
            class="label bg-accent hover:bg-accent-hover disabled:opacity-50 text-bg-primary px-5 py-2 transition-colors"
          >
            {submitting ? "sending..." : "send"}
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}
