<script module lang="ts">
  export interface SongMarker {
    index: number;
    songName: string;
    songId?: string;
    startMs: number;
  }
</script>

<script lang="ts">
  import { onDestroy } from "svelte";
  import { uploadFile } from "../api";

  let {
    bandSlug,
    onStopped,
  }: {
    bandSlug: string;
    onStopped: (result: { trackId: string; markers: SongMarker[]; durationMs: number }) => void;
  } = $props();

  type RecState = "idle" | "warming" | "recording" | "uploading";

  let recState = $state<RecState>("idle");
  let elapsed = $state(0);
  let level = $state(0);
  let uploadProgress = $state(0);
  let error = $state("");

  let micStream: MediaStream | null = null;
  let recordStream: MediaStream | null = null;
  let mediaRecorder: MediaRecorder | null = null;
  let audioCtx: AudioContext | null = null;
  let analyser: AnalyserNode | null = null;
  let chunks: Blob[] = [];
  let timerInterval: ReturnType<typeof setInterval> | null = null;
  let meterRaf = 0;

  let recordingStartTime = 0;   // performance.now() at recording start
  let markers: SongMarker[] = [];

  export function isRecording(): boolean {
    return recState === "recording";
  }

  export async function startRecording(): Promise<void> {
    if (recState !== "idle") return;
    error = "";
    recState = "warming";

    try {
      if (!micStream) {
        micStream = await navigator.mediaDevices.getUserMedia({
          audio: {
            echoCancellation: false,
            noiseSuppression: false,
            autoGainControl: false,
            channelCount: 1,
          },
        });

        audioCtx = new AudioContext();
        await audioCtx.resume();
        const source = audioCtx.createMediaStreamSource(micStream);
        const merger = audioCtx.createChannelMerger(1);
        source.connect(merger);

        const dest = audioCtx.createMediaStreamDestination();
        merger.connect(dest);
        recordStream = dest.stream;

        analyser = audioCtx.createAnalyser();
        analyser.fftSize = 256;
        merger.connect(analyser);
        meterLoop();
      }

      const mimeType = MediaRecorder.isTypeSupported("audio/webm;codecs=opus")
        ? "audio/webm;codecs=opus"
        : MediaRecorder.isTypeSupported("audio/mp4")
          ? "audio/mp4"
          : "";

      mediaRecorder = new MediaRecorder(recordStream!, mimeType ? { mimeType } : {});
      chunks = [];
      markers = [];

      mediaRecorder.ondataavailable = (e) => {
        if (e.data.size > 0) chunks.push(e.data);
      };

      mediaRecorder.onstop = () => handleStop();

      await new Promise<void>((resolve) => {
        mediaRecorder!.onstart = () => resolve();
        mediaRecorder!.start();
      });

      recordingStartTime = performance.now();
      recState = "recording";
      elapsed = 0;
      timerInterval = setInterval(() => elapsed++, 1000);
    } catch (e: any) {
      error = e.name === "NotAllowedError" ? "mic access denied" : (e.message || "mic error");
      recState = "idle";
    }
  }

  export function stopRecording(): void {
    if (recState !== "recording") return;
    if (timerInterval) { clearInterval(timerInterval); timerInterval = null; }
    if (mediaRecorder?.state === "recording") {
      mediaRecorder.requestData();
      // short delay to flush final chunk
      setTimeout(() => { mediaRecorder?.stop(); }, 200);
    }
  }

  export function markSong(index: number, songName: string, songId?: string): void {
    if (recState !== "recording") return;
    markers.push({ index, songName, songId, startMs: Math.round(performance.now() - recordingStartTime) });
  }

  async function handleStop() {
    if (chunks.length === 0) { recState = "idle"; return; }

    const blob = new Blob(chunks, { type: mediaRecorder!.mimeType });
    const durationMs = Math.round(performance.now() - recordingStartTime);
    const ext = blob.type.includes("mp4") ? ".m4a" : ".webm";
    const now = new Date();
    const dateStr = `${now.getFullYear()}-${(now.getMonth() + 1).toString().padStart(2, "0")}-${now.getDate().toString().padStart(2, "0")}`;
    const timeStr = `${now.getHours().toString().padStart(2, "0")}${now.getMinutes().toString().padStart(2, "0")}`;
    const title = `Rehearsal ${dateStr} ${now.getHours().toString().padStart(2, "0")}:${now.getMinutes().toString().padStart(2, "0")}`;
    const filename = `rehearsal_${dateStr}_${timeStr}${ext}`;

    recState = "uploading";
    uploadProgress = 0;

    try {
      const file = new File([blob], filename, { type: blob.type });
      const track = await uploadFile<{ id: string }>(
        `/api/bands/${bandSlug}/tracks`,
        file,
        { title },
        (pct) => (uploadProgress = pct),
      );
      onStopped({ trackId: track.id, markers, durationMs });
    } catch (e: any) {
      error = e.message || "upload failed";
    } finally {
      recState = "idle";
    }
  }

  function meterLoop() {
    if (!analyser) return;
    const data = new Uint8Array(analyser.frequencyBinCount);
    analyser.getByteFrequencyData(data);
    let sum = 0;
    for (let i = 0; i < data.length; i++) sum += data[i];
    level = sum / data.length / 255;
    meterRaf = requestAnimationFrame(meterLoop);
  }

  function formatElapsed(s: number): string {
    const m = Math.floor(s / 60);
    return `${m}:${(s % 60).toString().padStart(2, "0")}`;
  }

  onDestroy(() => {
    if (timerInterval) clearInterval(timerInterval);
    cancelAnimationFrame(meterRaf);
    if (mediaRecorder?.state === "recording") mediaRecorder.stop();
    micStream?.getTracks().forEach((t) => t.stop());
    audioCtx?.close();
  });
</script>

{#if recState === "recording"}
  <div class="flex items-center gap-3 px-4 py-2 bg-red-400/5 border-b border-red-400/20 shrink-0">
    <div class="w-2.5 h-2.5 rounded-full bg-red-400 animate-pulse shrink-0"></div>
    <span class="label-sm text-red-400 font-mono tabular-nums w-10">{formatElapsed(elapsed)}</span>
    <div class="flex-1 h-1.5 bg-bg-primary overflow-hidden rounded-full">
      <div
        class="h-full bg-red-400/70 rounded-full transition-[width] duration-75"
        style="width: {Math.min(100, level * 180)}%"
      ></div>
    </div>
    <button
      onclick={stopRecording}
      class="label-sm text-red-400 hover:text-red-300 transition-colors border border-red-400/40 hover:border-red-400 px-3 py-1 shrink-0"
    >stop</button>
  </div>
{:else if recState === "uploading"}
  <div class="flex items-center gap-3 px-4 py-2 bg-bg-surface border-b border-border shrink-0">
    <span class="label-sm text-text-muted">uploading recording</span>
    <div class="flex-1 h-1.5 bg-bg-primary overflow-hidden rounded-full">
      <div class="h-full bg-accent rounded-full transition-all duration-300" style="width: {uploadProgress}%"></div>
    </div>
    <span class="label-sm font-mono text-text-muted shrink-0">{uploadProgress}%</span>
  </div>
{:else if recState === "warming"}
  <div class="flex items-center gap-3 px-4 py-2 bg-bg-surface border-b border-border shrink-0">
    <div class="w-2.5 h-2.5 rounded-full bg-red-400/40 animate-pulse shrink-0"></div>
    <span class="label-sm text-text-muted">mic warming up...</span>
  </div>
{/if}

{#if error}
  <div class="px-4 py-1.5 label-sm text-danger border-b border-danger/20 shrink-0">{error}</div>
{/if}
