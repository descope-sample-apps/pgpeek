// Easter eggs for the pgpeek UI.
//
// This module is intentionally excluded from the coverage `include` list in
// vitest.config.js — the eggs are optional fun and their internal branches are
// not worth pinning at 100%. The *integration points* in tracked files (the
// single `if (eg)` branch in sql-tab.js, the konami banner / footer in app.js,
// the hidden-schema block in sidebar.js) are covered by the existing suites
// plus a few added tests.

// ── Konami code: ↑↑↓↓←→←→ B A ───────────────────────────────
const KONAMI = [
  "ArrowUp", "ArrowUp", "ArrowDown", "ArrowDown",
  "ArrowLeft", "ArrowRight", "ArrowLeft", "ArrowRight", "b", "a",
];

export function installKonamiCode(onTrigger) {
  let buf = [];
  const handler = (e) => {
    // Letters arrive as "B"/"A" with Caps Lock or Shift held; fold them so the
    // sequence still matches. Named keys ("ArrowUp") keep their casing.
    buf.push(e.key && e.key.length === 1 ? e.key.toLowerCase() : e.key);
    if (buf.length > KONAMI.length) buf = buf.slice(buf.length - KONAMI.length);
    if (buf.length === KONAMI.length && buf.join(",") === KONAMI.join(",")) {
      onTrigger();
      buf = [];
    }
  };
  // Capture phase on `document`, not bubble phase on `window`: the capture pass
  // runs root→target, so it sees the key even when the focused widget (e.g. the
  // CodeMirror editor) calls stopPropagation, and regardless of whether the
  // event bubbles at all.
  document.addEventListener("keydown", handler, true);
  return () => document.removeEventListener("keydown", handler, true);
}

export function installShuniCode(onTrigger) {
  const code = "iloveshuni";
  let buf = "";
  const handler = (e) => {
    if (!e.key || e.key.length !== 1) return;
    buf = (buf + e.key.toLowerCase()).slice(-code.length);
    if (buf === code) onTrigger();
  };
  document.addEventListener("keydown", handler, true);
  return () => document.removeEventListener("keydown", handler, true);
}

export const KONAMI_BANNER = "🔓 ROOT ACCESS GRANTED";
export const KONAMI_TOAST = "Just kidding — pgpeek is read-only. That's the whole point.";

// ── Magic SQL queries (intercepted client-side, never sent to the DB) ──
const MAGIC = {
  "select * from magic": {
    columns: ["id", "spell", "effect"],
    rows: [[42, "pgpeek", "browse safely ✨"]],
    rowCount: 1,
  },
  "select * from ducks": {
    columns: ["id", "name", "mood"],
    rows: [[1, "Quackers", "ready to debug 🦆"]],
    rowCount: 1,
  },
  "select * from elephants": {
    columns: ["id", "name", "trunk_length"],
    rows: [[1, "Hathi", "very long 🐘"]],
    rowCount: 1,
  },
};

function normalizeSQL(sql) {
  return sql.trim().toLowerCase().replace(/;+\s*$/, "").replace(/\s+/g, " ").trim();
}

export function interceptMagicQuery(sql) {
  const hit = MAGIC[normalizeSQL(sql)];
  if (!hit) return null;
  return { columns: hit.columns, rows: hit.rows, rowCount: hit.rowCount, truncated: false, elapsedMs: 0 };
}

// ── Dangerous / no-op SQL detection ─────────────────────────
const DROP_RE = /^\s*DROP\s+(TABLE|DATABASE|SCHEMA|INDEX|VIEW|MATERIALIZED\s+VIEW)\b/i;
const VACUUM_RE = /^VACUUM\b/i;
const ANALYZE_RE = /^\s*ANALYZE\b/i;
const EXPLAIN_RE = /^\s*EXPLAIN\b/i;

export function detectDangerousSQL(sql) {
  const s = sql.trim().replace(/^(?:(?:--[^\n]*(?:\n|$))|(?:\/\*[^*]*(?:\*(?!\/)[^*]*)*\*\/)|\s)+/, "");
  if (DROP_RE.test(s)) return "drop";
  if (VACUUM_RE.test(s)) return "vacuum";
  if (ANALYZE_RE.test(s) && !EXPLAIN_RE.test(s)) return "analyze";
  return null;
}

export function rewriteDropToSelect(sql) {
  const m = sql.match(
    /^\s*DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?([A-Za-z_][\w]*(?:\.[A-Za-z_][\w]*)?)\s*;?\s*$/i,
  );
  if (m) return "SELECT * FROM " + m[1] + ";";
  return "SELECT * FROM pgpeek_is_read_only;";
}

export const DROP_MESSAGE =
  "Nice try. pgpeek is read-only — your DROP has been converted to a polite SELECT.";

export function whisperMessage(keyword) {
  return "👻 whoosh — " + keyword + " is a no-op here. pgpeek is read-only.";
}

// ── Combined run interceptor: returns a tagged egg or null ──
export function interceptRun(sql) {
  const magic = interceptMagicQuery(sql);
  if (magic) return { type: "magic", result: magic };
  const danger = detectDangerousSQL(sql);
  if (danger === "drop") return { type: "drop", rewrite: rewriteDropToSelect(sql), message: DROP_MESSAGE };
  if (danger === "vacuum") return { type: "whisper", keyword: "VACUUM", message: whisperMessage("VACUUM") };
  if (danger === "analyze") return { type: "whisper", keyword: "ANALYZE", message: whisperMessage("ANALYZE") };
  return null;
}

// Apply an intercepted egg. Branches live here (untracked) so sql-tab.js only
// carries a single `if (eg)` branch.
export function runEasterEgg(eg, ctx) {
  if (eg.type === "magic") {
    ctx.setLastSQL("");
    ctx.setError("");
    ctx.setResult(eg.result);
    const n = eg.result.rowCount;
    ctx.setStatus({ text: "✓ " + n + " row" + (n === 1 ? "" : "s") + " in 0 ms ✨", cls: "ok" });
  } else if (eg.type === "drop") {
    // setSQL first: it calls onEdit → invalidate, which clears error/result.
    // Then setError/status re-arm them so the message survives.
    ctx.setSQL(eg.rewrite);
    ctx.setError(eg.message);
    ctx.setStatus({ text: "✗ " + eg.message, cls: "error" });
    shake(ctx.wrapRef);
  } else if (eg.type === "whisper") {
    ctx.setError(eg.message);
    ctx.setStatus({ text: eg.message, cls: "ok" });
    ghostDrift(ctx.wrapRef);
  }
}

function retrigger(el, cls, ms) {
  if (!el) return;
  el.classList.remove(cls);
  // Force a reflow so the animation can replay.
  void el.offsetWidth;
  el.classList.add(cls);
  setTimeout(() => el.classList.remove(cls), ms);
}

function shake(ref) {
  retrigger(ref && ref.current, "egg-shake", 500);
}

function ghostDrift(ref) {
  retrigger(ref && ref.current, "egg-ghost", 2000);
}

// ── Empty-state creatures ───────────────────────────────────
const CREATURES = [
  { emoji: "🦫", quip: "Nothing here yet. This table is as empty as a beaver's lunch log." },
  { emoji: "🦥", quip: "No rows. Sloth-paced growth in here." },
  { emoji: "🪵", quip: "Zero rows. Just logs and silence." },
  { emoji: "🐚", quip: "Empty. Listen — you can almost hear the database echo." },
  { emoji: "🌵", quip: "No rows in sight. A little sparse, even for a desert." },
];

export function pickCreature(seed) {
  let h = 0;
  const s = String(seed || "");
  for (let i = 0; i < s.length; i += 1) h = (h * 31 + s.charCodeAt(i)) | 0;
  return CREATURES[Math.abs(h) % CREATURES.length];
}

export function emptyCreatureText(seed) {
  const c = pickCreature(seed);
  return c.emoji + "  " + c.quip;
}

// ── Footer table-click counter (sessionStorage) ─────────────
const CLICK_KEY = "pgpeek-table-clicks";

export function readTableClicks() {
  try { return parseInt(sessionStorage.getItem(CLICK_KEY) || "0", 10) || 0; } catch { return 0; }
}

export function bumpTableClicks() {
  const n = readTableClicks() + 1;
  try { sessionStorage.setItem(CLICK_KEY, String(n)); } catch { /* best-effort */ }
  return n;
}

export function footerText(n) {
  if (n <= 0) return "";
  if (n >= 250) return "You have browsed " + n + " tables. Maybe take a screen break? 🌿";
  if (n >= 100) return "You have browsed " + n + " tables today. Sláine, table champion. 🏆";
  return "You have browsed " + n + " table" + (n === 1 ? "" : "s") + " this session.";
}

// ── Hidden sidebar schema ───────────────────────────────────
// Shuni is Descope's AI issue-fetcher bot (github.com/descope/shuni) — point
// her at a problem and she brings back a PR. 🦴 good girl. good code. 🦴
export const SHUNI_SCHEMA = "🐕 shuni";
export const SHUNI_VIEW = { name: "belly_rubs", type: "view", estRows: -1 };
export const SHUNI_COLUMNS = ["dog_name", "tail_wags", "fetched_prs", "good_girl_rating"];

// The relation is fictional, so every tab has to serve it locally — hitting the
// API would just 500. Returns null for real relations so callers fall through.
export function isShuniRelation(table) {
  return Boolean(table) && table.schema === SHUNI_SCHEMA && table.name === SHUNI_VIEW.name;
}

const SHUNI_ROWS = [
  ["Shuni", 9999, 42, "11/10 🦴"],
  ["Shuni", 9999, 43, "still 11/10 🎾"],
];

export function shuniData(table) {
  if (!isShuniRelation(table)) return null;
  return {
    columns: SHUNI_COLUMNS,
    rows: SHUNI_ROWS,
    rowCount: SHUNI_ROWS.length,
    truncated: false,
    elapsedMs: 0,
  };
}

export function shuniColumns(table) {
  if (!isShuniRelation(table)) return null;
  return [
    { name: "dog_name", type: "text", nullable: false, default: "'Shuni'" },
    { name: "tail_wags", type: "bigint", nullable: false, default: "9999" },
    { name: "fetched_prs", type: "integer", nullable: false, default: null },
    { name: "good_girl_rating", type: "text", nullable: false, default: "'11/10'" },
  ];
}

export const SHUNI_STATUS = "🦴 good girl. good code. 🦴";

// ── EXPLAIN joke (appended to the SQL hint) ─────────────────
export const EXPLAIN_JOKE = "Need to EXPLAIN this query? So do we, every time.";
