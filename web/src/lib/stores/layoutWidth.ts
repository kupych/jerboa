import { writable } from 'svelte/store';

export type LayoutWidth = 'reading' | 'index' | 'workspace' | 'stage';

/** Set this in a page's onMount / reset in onDestroy to opt into a width variant. */
export const layoutWidth = writable<LayoutWidth>('index');

/** CSS max-width class for each variant. */
export const widthClass: Record<LayoutWidth, string> = {
  reading:   'max-w-[900px]',
  index:     'max-w-[1280px]',   // current baseline — no visible change for existing pages
  workspace: 'max-w-[1600px]',   // Track, Set
  stage:     'max-w-none',       // Perform
};
