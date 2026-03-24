import { writable, derived } from "svelte/store";
import { api, apiPost } from "../api";

export interface UnreadCount {
  band_id: string;
  band_slug: string;
  new_tracks: number;
  new_comments: number;
  new_chats: number;
  new_songs: number;
  total: number;
}

export const unreadCounts = writable<UnreadCount[]>([]);

export const totalUnread = derived(unreadCounts, ($counts) =>
  $counts.reduce((sum, c) => sum + c.total, 0),
);

export async function loadUnread() {
  try {
    const counts = await api<UnreadCount[]>("/api/activity/unread");
    unreadCounts.set(counts);
  } catch {
    unreadCounts.set([]);
  }
}

export async function markBandSeen(slug: string) {
  await apiPost(`/api/bands/${slug}/seen`, {});
  await loadUnread();
}
