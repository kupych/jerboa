<script lang="ts">
  import { uploadFile } from "../api";
  import { apiPatch } from "../api";

  let {
    bandSlug,
    songId = undefined,
    overdubParentId = undefined,
    parentStreamUrl = undefined,
    onRecorded,
  }: {
    bandSlug: string;
    songId?: string;
    overdubParentId?: string;
    parentStreamUrl?: string;
    onRecorded: () => void;
  } = $props();

  let parentAudio: HTMLAudioElement | null = null;
  let capturedOffsetMs = 0;

  let recState = $state<"idle" | "recording" | "uploading">("idle");
  let mediaRecorder: MediaRecorder | null = null;
  let chunks: Blob[] = [];
  let elapsed = $state(0);
  let timerInterval: ReturnType<typeof setInterval> | null = null;
  let error = $state("");
  let uploadProgress = $state(0);

  // Mic level visualization
  let level = $state(0);
  let audioCtx: AudioContext | null = null;
  let analyser: AnalyserNode | null = null;
  let animFrame = 0;

  function formatElapsed(s: number): string {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${sec.toString().padStart(2, "0")}`;
  }

  async function startRecording() {
    error = "";
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });

      // Set up level meter
      audioCtx = new AudioContext();
      const source = audioCtx.createMediaStreamSource(stream);
      analyser = audioCtx.createAnalyser();
      analyser.fftSize = 256;
      source.connect(analyser);
      meterLoop();

      // Pick best available format
      const mimeType = MediaRecorder.isTypeSupported("audio/webm;codecs=opus")
        ? "audio/webm;codecs=opus"
        : MediaRecorder.isTypeSupported("audio/mp4")
          ? "audio/mp4"
          : "";

      mediaRecorder = new MediaRecorder(stream, mimeType ? { mimeType } : {});
      chunks = [];

      mediaRecorder.ondataavailable = (e) => {
        if (e.data.size > 0) chunks.push(e.data);
      };

      mediaRecorder.onstop = async () => {
        // Stop all tracks to release mic
        stream.getTracks().forEach((t) => t.stop());
        stopMeter();

        if (chunks.length === 0) {
          recState = "idle";
          return;
        }

        const blob = new Blob(chunks, { type: mediaRecorder!.mimeType });
        const ext = blob.type.includes("mp4") ? ".m4a" : ".webm";
        const now = new Date();
        const stamp = `${now.getFullYear()}-${(now.getMonth() + 1).toString().padStart(2, "0")}-${now.getDate().toString().padStart(2, "0")}_${now.getHours().toString().padStart(2, "0")}${now.getMinutes().toString().padStart(2, "0")}`;
        const filename = `recording_${stamp}${ext}`;

        recState = "uploading";
        uploadProgress = 0;
        try {
          const file = new File([blob], filename, { type: blob.type });

          if (overdubParentId) {
            // Upload as overdub with captured offset
            await uploadFile(
              `/api/bands/${bandSlug}/tracks/${overdubParentId}/overdubs`,
              file,
              { offset_ms: String(capturedOffsetMs) },
              (pct) => (uploadProgress = pct),
            );
          } else {
            // Normal upload
            const track = await uploadFile<{ id: string }>(
              `/api/bands/${bandSlug}/tracks`,
              file,
              {},
              (pct) => (uploadProgress = pct),
            );
            if (songId) {
              await apiPatch(`/api/bands/${bandSlug}/tracks/${track.id}/song`, {
                song_id: songId,
              });
            }
          }
          onRecorded();
        } catch (e: any) {
          error = e.message || "Upload failed";
        } finally {
          recState = "idle";
        }
      };

      mediaRecorder.start(1000); // 1s chunks
      recState = "recording";
      elapsed = 0;
      timerInterval = setInterval(() => elapsed++, 1000);

      // Start parent playback for overdub sync
      if (overdubParentId && parentStreamUrl) {
        parentAudio = new Audio(parentStreamUrl);
        parentAudio.play().then(() => {
          capturedOffsetMs = Math.round((parentAudio?.currentTime ?? 0) * 1000);
        }).catch(() => {
          // If autoplay blocked, offset stays 0
          capturedOffsetMs = 0;
        });
      }
    } catch (e: any) {
      if (e.name === "NotAllowedError") {
        error = "mic access denied";
      } else {
        error = e.message || "Could not start recording";
      }
    }
  }

  function stopRecording() {
    if (mediaRecorder && mediaRecorder.state === "recording") {
      mediaRecorder.stop();
    }
    if (parentAudio) {
      parentAudio.pause();
      parentAudio = null;
    }
    if (timerInterval) {
      clearInterval(timerInterval);
      timerInterval = null;
    }
  }

  function meterLoop() {
    if (!analyser) return;
    const data = new Uint8Array(analyser.frequencyBinCount);
    analyser.getByteFrequencyData(data);
    let sum = 0;
    for (let i = 0; i < data.length; i++) sum += data[i];
    level = sum / data.length / 255;
    animFrame = requestAnimationFrame(meterLoop);
  }

  function stopMeter() {
    cancelAnimationFrame(animFrame);
    level = 0;
    if (audioCtx) {
      audioCtx.close();
      audioCtx = null;
    }
    analyser = null;
  }
</script>

{#if recState === "idle"}
  <button
    onclick={startRecording}
    class="border border-dashed border-border hover:border-red-400/50 p-4 text-center transition-colors w-full flex items-center justify-center gap-2 group"
  >
    <div class="w-3 h-3 rounded-full bg-red-400/60 group-hover:bg-red-400 transition-colors"></div>
    <span class="text-sm text-text-muted font-semibold group-hover:text-red-400 transition-colors">{overdubParentId ? "record overdub" : "record a take"}</span>
  </button>
{:else if recState === "recording"}
  <div class="border border-red-400/40 bg-red-400/5 p-4 text-center">
    <div class="flex items-center justify-center gap-4">
      <div class="flex items-center gap-2">
        <div class="w-3 h-3 rounded-full bg-red-400 animate-pulse"></div>
        <span class="label text-red-400 font-mono">{formatElapsed(elapsed)}</span>
      </div>
      <!-- Level meter -->
      <div class="w-24 h-2 bg-bg-primary overflow-hidden">
        <div
          class="h-full bg-red-400 transition-[width] duration-75"
          style="width: {Math.min(100, level * 150)}%"
        ></div>
      </div>
      <button
        onclick={stopRecording}
        class="px-4 py-1.5 bg-red-400 hover:bg-red-500 text-bg-primary label-sm transition-colors"
      >
        stop
      </button>
    </div>
  </div>
{:else}
  <div class="border border-border p-4 text-center">
    <div class="flex items-center justify-center gap-3">
      <span class="label-sm text-text-muted">uploading</span>
      <div class="w-32 bg-bg-primary h-1">
        <div class="bg-accent h-1 transition-all duration-300" style="width: {uploadProgress}%"></div>
      </div>
      <span class="label-sm font-mono text-text-muted">{uploadProgress}%</span>
    </div>
  </div>
{/if}

{#if error}
  <div class="label-sm text-danger mt-2 text-center">{error}</div>
{/if}
