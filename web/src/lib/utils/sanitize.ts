// Small allow-list HTML sanitizer for user-generated markdown output.
// marked does NOT strip HTML by default — anything like <img onerror=…> or
// <script> coming out of marked.parse() would execute on render. We pass the
// parsed HTML through this function before @html-binding it.
//
// Strategy: parse into a detached document, walk every element, drop unknown
// tags and every attribute that isn't in the allowlist. URL-bearing attributes
// are additionally validated for a safe scheme.

const ALLOWED_TAGS = new Set([
  "a", "p", "br", "hr", "span", "div",
  "strong", "em", "b", "i", "u", "s", "del", "mark", "small", "sub", "sup",
  "h1", "h2", "h3", "h4", "h5", "h6",
  "ul", "ol", "li",
  "blockquote", "pre", "code",
  "table", "thead", "tbody", "tfoot", "tr", "th", "td",
  "img",
]);

const ALLOWED_ATTRS: Record<string, Set<string>> = {
  a: new Set(["href", "title", "target", "rel"]),
  img: new Set(["src", "alt", "title", "width", "height"]),
  "*": new Set(["class"]),
};

const URL_ATTRS = new Set(["href", "src"]);

function isSafeURL(raw: string): boolean {
  const v = raw.trim().toLowerCase();
  if (v === "") return false;
  // Relative and fragment URLs are fine
  if (v.startsWith("/") || v.startsWith("#") || v.startsWith("?")) return true;
  // Allow common safe schemes
  return (
    v.startsWith("http://") ||
    v.startsWith("https://") ||
    v.startsWith("mailto:") ||
    v.startsWith("data:image/")
  );
}

function cleanElement(el: Element) {
  const tag = el.tagName.toLowerCase();
  if (!ALLOWED_TAGS.has(tag)) {
    // Replace the element with its text content instead of dropping silently.
    const text = document.createTextNode(el.textContent ?? "");
    el.replaceWith(text);
    return;
  }

  const allowed = ALLOWED_ATTRS[tag] ?? new Set<string>();
  const globalAllowed = ALLOWED_ATTRS["*"]!;

  // Iterate attributes in reverse — we mutate the NamedNodeMap as we go.
  for (let i = el.attributes.length - 1; i >= 0; i--) {
    const attr = el.attributes[i];
    const name = attr.name.toLowerCase();
    if (!allowed.has(name) && !globalAllowed.has(name)) {
      el.removeAttribute(attr.name);
      continue;
    }
    if (URL_ATTRS.has(name) && !isSafeURL(attr.value)) {
      el.removeAttribute(attr.name);
      continue;
    }
  }

  // Force external links to open safely.
  if (tag === "a" && el.getAttribute("href")) {
    el.setAttribute("rel", "noopener noreferrer nofollow");
    if (!el.getAttribute("target")) {
      el.setAttribute("target", "_blank");
    }
  }
}

export function sanitizeHtml(dirty: string): string {
  if (typeof DOMParser === "undefined") return "";
  const doc = new DOMParser().parseFromString(`<div>${dirty}</div>`, "text/html");
  const root = doc.body.firstChild as HTMLElement | null;
  if (!root) return "";

  // Depth-first walk. TreeWalker skips nodes we detach mid-walk, which is
  // exactly what we want here.
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_ELEMENT);
  const toClean: Element[] = [];
  while (walker.nextNode()) {
    toClean.push(walker.currentNode as Element);
  }
  // Clean leaf-first so replaced elements don't get re-visited.
  for (let i = toClean.length - 1; i >= 0; i--) {
    cleanElement(toClean[i]);
  }
  return root.innerHTML;
}
