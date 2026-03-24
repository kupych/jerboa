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

export interface ActivityItem {
  type: "track" | "comment" | "chat" | "song";
  actor_name: string;
  subject: string;
  band_slug: string;
  band_name: string;
  link_id: string;
  created_at: string;
}

export const unreadCounts = writable<UnreadCount[]>([]);
export const activityFeed = writable<ActivityItem[]>([]);

export const totalUnread = derived(unreadCounts, ($counts) =>
  $counts.reduce((sum, c) => sum + c.total, 0),
);

export async function loadUnread() {
  try {
    const [counts, feed] = await Promise.all([
      api<UnreadCount[]>("/api/activity/unread"),
      api<ActivityItem[]>("/api/activity/feed"),
    ]);
    unreadCounts.set(counts);
    activityFeed.set(feed);
  } catch {
    unreadCounts.set([]);
    activityFeed.set([]);
  }
}

export async function markBandSeen(slug: string) {
  await apiPost(`/api/bands/${slug}/seen`, {});
  await loadUnread();
}
