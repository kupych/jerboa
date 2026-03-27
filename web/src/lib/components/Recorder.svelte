<script lang="ts">
  import { onMount } from "svelte";
  import { uploadFile } from "../api";
  import { apiPatch } from "../api";

  let {
    bandSlug,
    songId = undefined,
    overdubParentId = undefined,
    parentStreamUrl = undefined,
    latencyCompensation = 0,
    punchInMs = 0,
    bpm = 0,
    onRecorded,
  }: {
    bandSlug: string;
    songId?: string;
    overdubParentId?: string;
    parentStreamUrl?: string;
    latencyCompensation?: number;
    punchInMs?: number;
    bpm?: number;
    onRecorded: () => void;
  } = $props();

  // Parent audio: raw data (survives context resets) + decoded buffer
  let parentRawData: ArrayBuffer | null = null;
  let parentBuffer: AudioBuffer | null = null;
  let parentSource: AudioBufferSourceNode | null = null;
  let parentLoading = $state(false);

  let recState = $state<"idle" | "warming" | "counting" | "recording" | "uploading">("idle");
  let mediaRecorder: MediaRecorder | null = null;
  let micStream: MediaStream | null = null;
  let recordStream: MediaStream | null = null;
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

  // Pre-fetch parent audio data when URL is available
  $effect(() => {
    if (parentStreamUrl && overdubParentId && !parentRawData) {
      prefetchParent();
    }
  });

  async function prefetchParent() {
    parentLoading = true;
    try {
      const resp = await fetch(parentStreamUrl!);
      parentRawData = await resp.arrayBuffer();
    } catch {
      error = "couldn't load parent track";
    } finally {
      parentLoading = false;
    }
  }

  onMount(() => {
    return () => cleanup();
  });

  async function handleRecord() {
    if (recState === "recording") return;
    error = "";
    recState = "warming";

    try {
      // Request mic on first use
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

        // Recording destination captures ONLY mic, not parent playback
        const dest = audioCtx.createMediaStreamDestination();
        merger.connect(dest);
        recordStream = dest.stream;

        analyser = audioCtx.createAnalyser();
        analyser.fftSize = 256;
        merger.connect(analyser);
        meterLoop();
      }

      // Decode parent audio for this AudioContext if needed
      if (overdubParentId && parentRawData && !parentBuffer && audioCtx) {
        // .slice(0) clones so we can re-decode if context is recreated
        parentBuffer = await audioCtx.decodeAudioData(parentRawData.slice(0));
      }

      await startRecording();
    } catch (e: any) {
      if (e.name === "NotAllowedError") {
        error = "mic access denied";
      } else {
        error = e.message || "Could not access mic";
      }
      recState = "idle";
    }
  }

  let countBeat = $state(0);

  function playCountIn(ctx: AudioContext): Promise<void> {
    if (!bpm || bpm <= 0) return Promise.resolve();
    const beatInterval = 60 / bpm;
    const beats = 4;
    return new Promise((resolve) => {
      for (let i = 0; i < beats; i++) {
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.connect(gain);
        gain.connect(ctx.destination);
        // High click: first beat accented
        osc.frequency.value = i === 0 ? 1200 : 800;
        osc.type = "sine";
        const startTime = ctx.currentTime + i * beatInterval;
        gain.gain.setValueAtTime(0.5, startTime);
        gain.gain.exponentialRampToValueAtTime(0.001, startTime + 0.08);
        osc.start(startTime);
        osc.stop(startTime + 0.08);
        // Update beat counter for UI
        setTimeout(() => { countBeat = i + 1; }, i * beatInterval * 1000);
      }
      setTimeout(() => {
        countBeat = 0;
        resolve();
      }, beats * beatInterval * 1000);
    });
  }

  async function startRecording() {
    const mimeType = MediaRecorder.isTypeSupported("audio/webm;codecs=opus")
      ? "audio/webm;codecs=opus"
      : MediaRecorder.isTypeSupported("audio/mp4")
        ? "audio/mp4"
        : "";

    mediaRecorder = new MediaRecorder(recordStream!, mimeType ? { mimeType } : {});
    chunks = [];

    mediaRecorder.ondataavailable = (e) => {
      if (e.data.size > 0) chunks.push(e.data);
    };

    mediaRecorder.onstop = async () => {
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
          const adjustedOffset = punchInMs - latencyCompensation;
          await uploadFile(
            `/api/bands/${bandSlug}/tracks/${overdubParentId}/overdubs`,
            file,
            { offset_ms: String(adjustedOffset) },
            (pct) => (uploadProgress = pct),
          );
        } else {
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

    // Count-in clicks before recording
    if (bpm > 0 && audioCtx) {
      recState = "counting";
      await playCountIn(audioCtx);
    }

    // Wait for encoder to be truly active before starting parent
    await new Promise<void>((resolve) => {
      mediaRecorder!.onstart = () => resolve();
      mediaRecorder!.start();
    });

    recState = "recording";
    elapsed = 0;
    timerInterval = setInterval(() => elapsed++, 1000);

    // Start parent playback via Web Audio API — sample-accurate sync
    // Routed to ctx.destination (speakers) but NOT to recordStream (mic only)
    if (overdubParentId && parentBuffer && audioCtx) {
      parentSource = audioCtx.createBufferSource();
      parentSource.buffer = parentBuffer;
      parentSource.connect(audioCtx.destination);
      parentSource.start(0, punchInMs / 1000);
    }
  }

  function stopRecording() {
    if (parentSource) {
      try { parentSource.stop(); } catch {}
      parentSource = null;
    }
    if (timerInterval) {
      clearInterval(timerInterval);
      timerInterval = null;
    }
    if (mediaRecorder && mediaRecorder.state === "recording") {
      mediaRecorder.requestData();
      setTimeout(() => {
        if (mediaRecorder && mediaRecorder.state === "recording") {
          mediaRecorder.stop();
        }
      }, 200);
    }
  }

  function cleanup() {
    stopRecording();
    stopMeter();
    if (micStream) {
      micStream.getTracks().forEach((t) => t.stop());
      micStream = null;
    }
    parentBuffer = null;
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
    onclick={handleRecord}
    disabled={parentLoading}
    class="border border-dashed border-border hover:border-red-400/50 p-3 md:py-2 text-center transition-colors w-full flex items-center justify-center gap-3 group"
  >
    <div class="w-3 h-3 rounded-full bg-red-400/60 group-hover:bg-red-400 animate-pulse transition-colors"></div>
    <span class="text-sm text-text-muted font-semibold group-hover:text-red-400 transition-colors">
      {parentLoading ? "loading track..." : (overdubParentId ? "record overdub" : "record a take")}
    </span>
  </button>
{:else if recState === "warming"}
  <div class="border border-dashed border-border p-3 text-center">
    <span class="text-sm text-text-muted font-semibold">warming up mic...</span>
  </div>
{:else if recState === "counting"}
  <div class="border border-amber-400/40 bg-amber-400/5 p-3 text-center">
    <div class="flex items-center justify-center gap-4">
      <div class="flex items-center gap-2">
        {#each [1, 2, 3, 4] as beat}
          <div class="w-4 h-4 rounded-full transition-colors {countBeat >= beat ? 'bg-amber-400' : 'bg-amber-400/20'}"></div>
        {/each}
      </div>
      <span class="label text-amber-400 font-mono">{bpm} bpm</span>
    </div>
  </div>
{:else if recState === "recording"}
  <div class="border border-red-400/40 bg-red-400/5 p-3 text-center">
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
