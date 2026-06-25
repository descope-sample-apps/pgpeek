// API helpers that inject the active db id as ?db=<id> on every DB-bound
// GET request and include it in JSON bodies for POST requests.

export function dbUrl(path, dbId) {
  if (!dbId) return path;
  const sep = path.includes("?") ? "&" : "?";
  return path + sep + "db=" + encodeURIComponent(dbId);
}

export async function getJSON(url, dbId) {
  const r = await fetch(dbUrl(url, dbId));
  const body = await r.json();
  if (!r.ok) throw new Error(body.error || r.statusText);
  return body;
}

export const tablePath = (t) =>
  "/api/tables/" + encodeURIComponent(t.schema) + "/" + encodeURIComponent(t.name);

export const tableKey = (t) => t.schema + "." + t.name;

// appendDataParams adds search/sort/filter params shared by browse + export.
export function appendDataParams(p, search, sort, filters) {
  if (search) p.set("search", search);
  if (sort) { p.set("sort", sort.col); p.set("dir", sort.dir); }
  for (const col of Object.keys(filters)) {
    const f = filters[col];
    if (!f.op) continue;
    const noVal = f.op === "is_null" || f.op === "is_not_null";
    if (noVal) p.append("f", col + ":" + f.op);
    else p.append("f", col + ":" + f.op + ":" + (f.value || ""));
  }
}
