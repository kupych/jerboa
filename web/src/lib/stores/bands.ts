import { writable } from "svelte/store";
import { api } from "../api";

export interface BandSummary {
  id: string;
  name: string;
  slug: string;
  role: string;
  color_scheme: string;
}

export const bands = writable<BandSummary[]>([]);

export async function loadBands() {
  try {
    const list = await api<BandSummary[]>("/api/bands");
    bands.set(list);
    return list;
  } catch {
    bands.set([]);
    return [];
  }
}
