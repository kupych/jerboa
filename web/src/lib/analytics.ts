// Lightweight in-house analytics: captures clicks (including "dead" clicks on
// non-interactive elements), route changes, and page visibility. Events are
// batched in memory + sessionStorage and flushed to /api/events.

type EventPayload = {
  session_id: string;
  kind: string;
  path: string;
  metadata: Record<string, unknown>;
  client_ts: number;
};

const SESSION_KEY = "jb_analytics_session";
const QUEUE_KEY = "jb_analytics_queue";
const FLUSH_INTERVAL_MS = 10_000;
const MAX_QUEUE = 200;
const MAX_TEXT = 80;

function newSessionId(): string {
  // 16 random bytes -> base36
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  let n = 0n;
  for (const b of bytes) n = (n << 8n) | BigInt(b);
  return n.toString(36).slice(0, 24);
}

function getSessionId(): string {
  try {
    let id = sessionStorage.getItem(SESSION_KEY);
    if (!id) {
      id = newSessionId();
      sessionStorage.setItem(SESSION_KEY, id);
    }
    return id;
  } catch {
    return "nostorage";
  }
}

let queue: EventPayload[] = [];

function loadPersistedQueue() {
  try {
    const raw = sessionStorage.getItem(QUEUE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) queue = parsed.slice(0, MAX_QUEUE);
    }
  } catch {}
}

function persistQueue() {
  try {
    sessionStorage.setItem(QUEUE_KEY, JSON.stringify(queue));
  } catch {}
}

function clearPersistedQueue() {
  try {
    sessionStorage.removeItem(QUEUE_KEY);
  } catch {}
}

function describeElement(el: Element): {
  tag: string;
  id?: string;
  classes?: string;
  text?: string;
  role?: string;
  ariaLabel?: string;
  href?: string;
} {
  const out: ReturnType<typeof describeElement> = { tag: el.tagName.toLowerCase() };
  const id = el.id;
  if (id) out.id = id.slice(0, 64);
  const cls = (el.getAttribute("class") || "").trim();
  if (cls) out.classes = cls.slice(0, 160);
  const role = el.getAttribute("role");
  if (role) out.role = role;
  const aria = el.getAttribute("aria-label");
  if (aria) out.ariaLabel = aria.slice(0, MAX_TEXT);
  if (el instanceof HTMLAnchorElement && el.href) {
    try {
      out.href = new URL(el.href, location.href).pathname;
    } catch {
      out.href = el.href.slice(0, 160);
    }
  }
  const text = (el.textContent || "").trim().replace(/\s+/g, " ");
  if (text) out.text = text.slice(0, MAX_TEXT);
  return out;
}

// Walks up the tree to find the nearest element that *behaves* interactive.
function findInteractiveAncestor(start: Element): Element | null {
  let el: Element | null = start;
  while (el && el !== document.documentElement) {
    if (isInteractive(el)) return el;
    el = el.parentElement;
  }
  return null;
}

function isInteractive(el: Element): boolean {
  const tag = el.tagName;
  if (tag === "A" || tag === "BUTTON" || tag === "INPUT" || tag === "SELECT" || tag === "TEXTAREA" || tag === "LABEL" || tag === "SUMMARY") {
    return true;
  }
  const role = el.getAttribute("role");
  if (role && ["button", "link", "checkbox", "menuitem", "tab", "switch", "option", "radio"].includes(role)) {
    return true;
  }
  if (el.hasAttribute("onclick")) return true;
  if ((el as HTMLElement).tabIndex >= 0 && tag !== "DIV" && tag !== "SPAN") return true;
  return false;
}

function enqueue(kind: string, metadata: Record<string, unknown>) {
  const evt: EventPayload = {
    session_id: getSessionId(),
    kind,
    path: location.pathname + location.search,
    metadata,
    client_ts: Date.now(),
  };
  queue.push(evt);
  if (queue.length > MAX_QUEUE) queue.splice(0, queue.length - MAX_QUEUE);
  persistQueue();
}

async function flush(useBeacon = false): Promise<void> {
  if (queue.length === 0) return;
  const batch = queue.slice();
  queue = [];
  persistQueue();

  const body = JSON.stringify({ events: batch });

  if (useBeacon && navigator.sendBeacon) {
    const blob = new Blob([body], { type: "application/json" });
    const ok = navigator.sendBeacon("/api/events", blob);
    if (ok) {
      clearPersistedQueue();
      return;
    }
  }

  try {
    await fetch("/api/events", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body,
      keepalive: useBeacon,
    });
    clearPersistedQueue();
  } catch {
    // Restore events so next flush retries them.
    queue = batch.concat(queue).slice(0, MAX_QUEUE);
    persistQueue();
  }
}

function trackClick(e: MouseEvent) {
  const target = e.target as Element | null;
  if (!target || !(target instanceof Element)) return;

  const rect = (target as HTMLElement).getBoundingClientRect?.();
  const vw = window.innerWidth || 1;
  const vh = window.innerHeight || 1;

  const interactiveAncestor = findInteractiveAncestor(target);
  const dead = interactiveAncestor === null;

  const metadata: Record<string, unknown> = {
    target: describeElement(target),
    dead,
    vx: Math.round((e.clientX / vw) * 1000) / 10,
    vy: Math.round((e.clientY / vh) * 1000) / 10,
    vw,
    vh,
  };
  if (interactiveAncestor && interactiveAncestor !== target) {
    metadata.interactive = describeElement(interactiveAncestor);
  }

  enqueue(dead ? "click_dead" : "click", metadata);
}

let lastPath = "";
function trackPageview() {
  const path = location.pathname + location.search;
  if (path === lastPath) return;
  lastPath = path;
  enqueue("pageview", {
    referrer: document.referrer || "",
    title: document.title,
  });
}

export function initAnalytics() {
  loadPersistedQueue();

  // Capture-phase click so we get *every* click, including ones on
  // non-interactive elements and clicks that bubble into handlers that stop propagation.
  document.addEventListener("click", trackClick, { capture: true });

  // Route changes: listen to popstate + wrap pushState/replaceState.
  const origPush = history.pushState.bind(history);
  const origReplace = history.replaceState.bind(history);
  history.pushState = function (...args) {
    const r = origPush(...args);
    queueMicrotask(trackPageview);
    return r;
  };
  history.replaceState = function (...args) {
    const r = origReplace(...args);
    queueMicrotask(trackPageview);
    return r;
  };
  window.addEventListener("popstate", trackPageview);

  // First pageview after a small delay so initial route is resolved.
  queueMicrotask(trackPageview);

  // Flush on tab hide / unload via sendBeacon (survives page close).
  window.addEventListener("pagehide", () => flush(true));
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") flush(true);
  });

  setInterval(() => {
    flush(false);
  }, FLUSH_INTERVAL_MS);

  // Retry any events carried over from a previous tab that crashed.
  if (queue.length > 0) flush(false);
}

export function analyticsSessionId(): string {
  return getSessionId();
}
