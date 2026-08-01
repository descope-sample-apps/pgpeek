// Sidebar (table list) and Tabs bar components.
import { html, useEffect, useRef, useState } from "./vendor/preact-htm.js";
import { tableKey } from "./api.js";

export function Sidebar({ tables, loaded, currentKey, onSelect }) {
  const [filter, setFilter] = useState("");
  const [expanded, setExpanded] = useState({});
  const listRef = useRef();
  const f = filter.toLowerCase();
  const items = [];
  let schema = null;

  useEffect(() => {
    const active = listRef.current && listRef.current.querySelector(".tbl.active");
    if (active && active.scrollIntoView) active.scrollIntoView({ block: "nearest" });
  }, [currentKey, filter]);

  const byKey = new Map(tables.map((t) => [tableKey(t), t]));
  const children = new Map();

  const rootKey = (table) => {
    if (!table.isPartition || !table.parentSchema || !table.parentName) return null;
    let key = table.parentSchema + "." + table.parentName;
    while (byKey.has(key) && byKey.get(key).isPartition) {
      const parent = byKey.get(key);
      key = parent.parentSchema + "." + parent.parentName;
    }
    return key;
  };

  for (const t of tables) {
    const root = rootKey(t);
    if (!root || !byKey.has(root)) continue;
    children.set(root, [...(children.get(root) || []), t]);
  }

  const tableButton = (t, child = false) => {
    const label = tableKey(t);
    const active = label === currentKey;
    const cls = "tbl" + (child ? " partition" : "") + (t.type === "view" ? " view" : "") + (active ? " active" : "");
    return html`<button class=${cls} key=${label}
      title=${label + (t.estRows >= 0 ? " (~" + t.estRows + " rows)" : "")}
      aria-current=${active ? "true" : undefined}
      onClick=${() => onSelect(t)}>${t.name}</button>`;
  };

  for (const t of tables) {
    const label = tableKey(t);
    const root = rootKey(t);
    if (root && byKey.has(root)) continue;
    const group = children.get(label) || [];
    const matchingChildren = f ? group.filter((child) => tableKey(child).toLowerCase().includes(f)) : group;
    if (f && !label.toLowerCase().includes(f) && matchingChildren.length === 0) continue;
    if (t.schema !== schema) {
      schema = t.schema;
      items.push(html`<div class="schema" key=${"s:" + schema}>${schema}</div>`);
    }
    if (group.length === 0) {
      items.push(tableButton(t));
      continue;
    }
    const open = Boolean(f ? matchingChildren.length : (expanded[label] ?? group.some((child) => tableKey(child) === currentKey)));
    items.push(html`<div class="table-group" key=${"g:" + label}>
      <div class="table-row">
        ${tableButton(t)}
        <button class="partition-toggle" type="button" aria-expanded=${open ? "true" : "false"} disabled=${Boolean(f)}
          title=${f ? "Clear filter to hide partitions" : (open ? "Hide partitions" : "Show partitions")}
          onClick=${() => setExpanded((value) => ({ ...value, [label]: !open }))}>
          <span aria-hidden="true">${open ? "▾" : "▸"}</span> ${group.length} partitions
        </button>
      </div>
      ${open ? html`<div class="partition-list">${matchingChildren.map((child) => tableButton(child, true))}</div>` : ""}
    </div>`);
  }
  return html`
    <aside class="sidebar" aria-label="Database tables">
      <div class="side-head"><span>Tables</span><span>${tables.length}</span></div>
      <label class="sr-only" for="tbl-filter">Filter tables</label>
      <input id="tbl-filter" type="search" placeholder="Filter tables…" autocomplete="off"
        value=${filter} onInput=${(e) => setFilter(e.target.value)} />
      <div id="tables" ref=${listRef}>${items.length
        ? items
        : html`<div class="empty">${tables.length
            ? "No tables match."
            : (loaded ? "No tables." : "Loading tables…")}</div>`}</div>
    </aside>`;
}

export function Tabs({ tab, setTab, title }) {
  const btn = (id, label) => html`<button id=${"tab-" + id} role="tab"
    aria-selected=${tab === id ? "true" : "false"} aria-controls=${"panel-" + id}
    class=${tab === id ? "active" : ""} onClick=${() => setTab(id)}>${label}</button>`;
  return html`
    <div class="tabs" role="tablist" aria-label="Table views">
      ${btn("data", "Data")} ${btn("structure", "Structure")} ${btn("sql", "SQL")}
      <span class="title" id="tab-title">${title}</span>
    </div>`;
}
