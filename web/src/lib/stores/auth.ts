import { derived, writable } from "svelte/store";
import { api } from "../api";

export interface User {
  id: string;
  email: string;
  display_name: string;
  avatar_url?: string;
  is_admin: boolean;
  is_demo: boolean;
}

export const user = writable<User | null>(null);
export const authLoading = writable(true);
export const isDemo = derived(user, ($u) => !!$u?.is_demo);

export async function checkAuth() {
  authLoading.set(true);
  try {
    const u = await api<User>("/auth/me");
    user.set(u);
  } catch {
    user.set(null);
  } finally {
    authLoading.set(false);
  }
}

export function login() {
  window.location.href = "/auth/login";
}

export async function sendMagicLink(email: string): Promise<"magic-link" | "oidc"> {
  const res = await api<{ method: string }>("/auth/magic-link", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  });
  return res.method as "magic-link" | "oidc";
}

export async function logout() {
  await api("/auth/logout", { method: "POST" });
  user.set(null);
}
