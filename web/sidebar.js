// Sidebar (table list) and Tabs bar components.
import { html, useEffect, useRef, useState } from "./vendor/preact-htm.js";
import { tableKey } from "./api.js";

export function Sidebar({ tables, loaded, currentKey, onSelect }) {
  const [filter, setFilter] = useState("");
  const listRef = useRef();
  const f = filter.toLowerCase();
  const items = [];
  let schema = null;

  useEffect(() => {
    const active = listRef.current && listRef.current.querySelector(".tbl.active");
    if (active && active.scrollIntoView) active.scrollIntoView({ block: "nearest" });
  }, [currentKey, filter]);

  for (const t of tables) {
    const label = tableKey(t);
    if (f && !label.toLowerCase().includes(f)) continue;
    if (t.schema !== schema) {
      schema = t.schema;
      items.push(html`<div class="schema" key=${"s:" + schema}>${schema}</div>`);
    }
    const active = label === currentKey;
    const cls = "tbl" + (t.type === "view" ? " view" : "") + (active ? " active" : "");
    items.push(html`<button class=${cls} key=${label}
      title=${label + (t.estRows >= 0 ? " (~" + t.estRows + " rows)" : "")}
      aria-current=${active ? "true" : undefined}
      onClick=${() => onSelect(t)}>${t.name}</button>`);
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
