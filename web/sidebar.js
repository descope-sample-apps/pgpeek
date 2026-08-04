// Sidebar (table list) and Tabs bar components.
import { html, useEffect, useRef, useState } from "./vendor/preact-htm.js";
import { tableKey } from "./api.js";
import { SHUNI_SCHEMA, SHUNI_VIEW, SHUNI_COLUMNS } from "./easter-eggs.js";

export function Sidebar({ tables, loaded, current, onSelect, showShuni }) {
  const [filter, setFilter] = useState("");
  const [expanded, setExpanded] = useState({});
  const listRef = useRef();
  const f = filter.toLowerCase();
  const items = [];
  let schema = null;

  useEffect(() => {
    const active = listRef.current && listRef.current.querySelector(".tbl.active");
    if (active && active.scrollIntoView) active.scrollIntoView({ block: "nearest" });
  }, [current, filter]);

  const relationKey = (table) => JSON.stringify([table.schema, table.name]);
  const byKey = new Map(tables.map((t) => [relationKey(t), t]));
  const children = new Map();
  const roots = new Map();
  const currentKey = current && relationKey(current);

  const rootKey = (table) => {
    const id = relationKey(table);
    if (roots.has(id)) return roots.get(id);
    if (!table.isPartition || !table.parentSchema || !table.parentName) {
      roots.set(id, null);
      return null;
    }
    const parentKey = relationKey({ schema: table.parentSchema, name: table.parentName });
    const parent = byKey.get(parentKey);
    const root = parent && parent.isPartition ? rootKey(parent) : parentKey;
    roots.set(id, root);
    return root;
  };

  for (const t of tables) {
    const root = rootKey(t);
    if (!root || !byKey.has(root)) continue;
    if (!children.has(root)) children.set(root, []);
    children.get(root).push(t);
  }

  const tableButton = (t, child = false) => {
    const label = tableKey(t);
    const active = relationKey(t) === currentKey;
    const cls = "tbl" + (child ? " partition" : "") + (t.type === "view" ? " view" : "") + (active ? " active" : "");
    return html`<button class=${cls} key=${relationKey(t)}
      title=${label + (t.estRows >= 0 ? " (~" + t.estRows + " rows)" : "")}
      aria-current=${active ? "true" : undefined}
      onClick=${() => onSelect(t)}>${t.name}</button>`;
  };

  for (const t of tables) {
    const label = tableKey(t);
    const id = relationKey(t);
    const root = rootKey(t);
    if (root && byKey.has(root)) continue;
    const group = children.get(relationKey(t)) || [];
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
    const open = Boolean(f ? matchingChildren.length : (expanded[id] ?? group.some((child) => relationKey(child) === currentKey)));
    const count = f ? matchingChildren.length : group.length;
    items.push(html`<div class="table-group" key=${"g:" + id}>
      <div class="table-row">
        ${tableButton(t)}
        <button class="partition-toggle" type="button" aria-expanded=${open ? "true" : "false"} disabled=${Boolean(f)}
          title=${f ? "Clear filter to hide partitions" : (open ? "Hide partitions" : "Show partitions")}
          onClick=${() => setExpanded((value) => ({ ...value, [id]: !open }))}>
          <span aria-hidden="true">${open ? "▾" : "▸"}</span> ${count} ${count === 1 ? "partition" : "partitions"}
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
            : (loaded ? "No tables." : "Loading tables…")}</div>`}
        ${showShuni && tables.length > 0 && !f
          ? html`<div class="egg-schema" key="egg-schema">${SHUNI_SCHEMA}</div>
            <button class="egg-tbl" key="egg-tbl" type="button"
              title=${SHUNI_SCHEMA + "." + SHUNI_VIEW.name + " · " + SHUNI_COLUMNS.join(", ")}
              onClick=${() => onSelect({ schema: SHUNI_SCHEMA, name: SHUNI_VIEW.name, type: SHUNI_VIEW.type, estRows: SHUNI_VIEW.estRows })}>
              ${SHUNI_VIEW.name}
            </button>`
          : null}</div>
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
