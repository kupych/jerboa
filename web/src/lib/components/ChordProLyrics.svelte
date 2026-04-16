<script lang="ts">
  let {
    text,
    showChords = true,
    large = false,
  }: {
    text: string;
    showChords?: boolean;
    large?: boolean;
  } = $props();

  type Segment = { chord: string; lyric: string };
  type ParsedLine = { segments: Segment[]; hasChords: boolean; sectionLabel?: string };

  const sectionMap: Record<string, string> = {
    start_of_verse: "Verse", sov: "Verse",
    start_of_chorus: "Chorus", soc: "Chorus",
    start_of_bridge: "Bridge", sob: "Bridge",
    start_of_tab: "Tab", sot: "Tab",
  };

  function parseLine(line: string): ParsedLine {
    const trimmed = line.trim();

    // Section directives: {start_of_verse}, {soc}, {comment: Intro}, etc.
    const directiveMatch = trimmed.match(/^\{(\w+)(?::\s*(.+))?\}$/);
    if (directiveMatch) {
      const key = directiveMatch[1].toLowerCase();
      // Skip end directives
      if (key.startsWith("end_of") || key === "eov" || key === "eoc" || key === "eob" || key === "eot") {
        return { segments: [{ chord: "", lyric: "" }], hasChords: false };
      }
      const label = directiveMatch[2] || sectionMap[key];
      if (label) {
        return { segments: [], hasChords: false, sectionLabel: label };
      }
      // comment directive
      if (key === "comment" || key === "c") {
        return { segments: [], hasChords: false, sectionLabel: directiveMatch[2] || "" };
      }
    }
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
    {#if line.sectionLabel}
      <div class="mt-7 mb-2.5 first:mt-0">
        <span class="label text-accent/70">{line.sectionLabel}</span>
      </div>
    {:else if line.segments.length === 1 && !line.segments[0].lyric.trim() && !line.segments[0].chord}
      <div class="h-5"></div>
    {:else if line.hasChords && showChords}
      <div class="mb-0.5">
        {#each line.segments as seg}
          <span class="inline-flex flex-col align-bottom" style="gap: 0;">
            <span class="text-accent font-bold font-mono {large ? 'text-sm' : 'text-xs'}" style="line-height: 1; margin-bottom: 2px;">{seg.chord || "\u00a0"}</span>
            <span class="text-text-primary whitespace-pre-wrap {large ? 'text-xl' : 'text-base'}" style="line-height: 1.5; padding-bottom: 3px;">{seg.lyric || "\u00a0"}</span>
          </span>
        {/each}
      </div>
    {:else}
      <div class="text-text-primary {large ? 'text-xl leading-[1.5]' : 'text-base leading-[1.45]'}">{line.segments.map(s => s.lyric).join("")}</div>
    {/if}
  {/each}
</div>
