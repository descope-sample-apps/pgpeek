export function dbUrl(path, dbId) {
  if (!dbId) return path;
  const sep = path.includes("?") ? "&" : "?";
  return path + sep + "db=" + encodeURIComponent(dbId);
}

export async function getJSON(url, dbId) {
  const r = await fetch(dbUrl(url, dbId));
  if (!r.ok) {
    const body = await r.json().catch(() => ({}));
    throw new Error(body.error || r.statusText);
  }
  const body = await r.json();
  return body;
}

export const tablePath = (t) =>
  "/api/tables/" + encodeURIComponent(t.schema) + "/" + encodeURIComponent(t.name);

export const tableKey = (t) => t.schema + "." + t.name;

// appendDataParams adds search/sort/filter params shared by browse + export.
export function appendDataParams(p, search, sort, filters) {
  if (search) p.set("search", search);
  if (sort) { p.set("sort", sort.col); p.set("dir", sort.dir); }
  for (const f of filters) {
    if (!f.column || !f.op) continue;
    const noVal = f.op === "is_null" || f.op === "is_not_null";
    if (noVal) p.append("f", f.column + ":" + f.op);
    else p.append("f", f.column + ":" + f.op + ":" + (f.value || ""));
  }
}
