<script lang="ts">
  let {
    text,
    showChords = true,
  }: {
    text: string;
    showChords?: boolean;
  } = $props();

  type Segment = { chord: string; lyric: string };
  type ParsedLine = { segments: Segment[]; hasChords: boolean };

  function parseLine(line: string): ParsedLine {
    const segments: Segment[] = [];
    const regex = /\[([^\]]*)\]/g;
    let lastIndex = 0;
    let match;
    let hasChords = false;

    while ((match = regex.exec(line)) !== null) {
      hasChords = true;
      const textBefore = line.slice(lastIndex, match.index);
      if (textBefore && segments.length > 0) {
        segments[segments.length - 1].lyric += textBefore;
      } else if (textBefore) {
        segments.push({ chord: "", lyric: textBefore });
      }
      segments.push({ chord: match[1], lyric: "" });
      lastIndex = regex.lastIndex;
    }

    if (hasChords) {
      // Remaining text after last chord
      const remaining = line.slice(lastIndex);
      if (remaining && segments.length > 0) {
        segments[segments.length - 1].lyric += remaining;
      } else if (remaining) {
        segments.push({ chord: "", lyric: remaining });
      }
    } else {
      segments.push({ chord: "", lyric: line });
    }

    return { segments, hasChords };
  }

  let lines = $derived(text.split("\n").map(parseLine));
</script>

<div class="chordpro leading-relaxed">
  {#each lines as line}
    {#if line.segments.length === 1 && !line.segments[0].lyric.trim() && !line.segments[0].chord}
      <div class="h-6"></div>
    {:else if line.hasChords && showChords}
      <div>
        {#each line.segments as seg}
          <span class="inline-flex flex-col align-bottom">
            <span class="text-accent font-bold text-sm leading-tight font-mono">{seg.chord || "\u00a0"}</span>
            <span class="text-text-primary">{seg.lyric || "\u00a0"}</span>
          </span>
        {/each}
      </div>
    {:else}
      <div class="text-text-primary">{line.segments.map(s => s.lyric).join("")}</div>
    {/if}
  {/each}
</div>
