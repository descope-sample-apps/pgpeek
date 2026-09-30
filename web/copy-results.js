import { html, useState, useRef, useEffect } from "./vendor/preact-htm.js";

const STORAGE_KEY = "pgpeek_copy_format";
const FORMATS = ["TSV", "CSV", "JSON", "Markdown"];

/**
 * formatTSV: NULL -> empty. Cells containing tab, CR/LF or a quote are quoted
 * (spreadsheet convention) so rows survive a paste.
 */
const tsvField = (s) => /[\t\r\n"]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
export function formatTSV(result) {
  return [result.columns, ...result.rows.map((row) => row.map(cellToString))]
    .map((fields) => fields.map(tsvField).join("\t")).join("\n");
}

/**
 * formatCSV: RFC 4180, NULL -> empty.
 */
const csvField = (s) => /[",\r\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
export function formatCSV(result) {
  return [result.columns, ...result.rows.map((row) => row.map(cellToString))]
    .map((fields) => fields.map(csvField).join(",")).join("\n");
}

/**
 * formatJSON: {columns, rows} (same shape as the JSON view); keeps duplicate column names.
 */
export function formatJSON(result) {
  return JSON.stringify({ columns: result.columns, rows: result.rows }, null, 2);
}

/**
 * formatMarkdown: pipe table; NULL -> empty cell; `|` escaped, newlines become <br>.
 */
const mdCell = (v) => cellToString(v).replace(/\|/g, "\\|").replace(/\r?\n/g, "<br>");
export function formatMarkdown(result) {
  return [
    "| " + result.columns.map(mdCell).join(" | ") + " |",
    "|" + result.columns.map(() => " --- ").join("|") + "|",
    ...result.rows.map((row) => "| " + row.map(mdCell).join(" | ") + " |"),
  ].join("\n");
}

/**
 * cellToString: normalize any value to displayable string.
 * Matches internal/db/pool.go::CellString logic.
 */
function cellToString(value) {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value;
  if (typeof value === "boolean") return String(value);
  if (typeof value === "number") return String(value);
  // JSON objects/arrays or other types
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/**
 * copyToClipboard: copy formatted text using clipboard API or fallback.
 * Returns {success, rowCount, cellsShortened}.
 */
export async function copyToClipboard(text) {
  // Try modern clipboard API first (may fail on HTTP origins)
  if (navigator.clipboard) {
    try {
      await navigator.clipboard.writeText(text);
      return { success: true };
    } catch (err) {
      // Fall through to execCommand
    }
  }

  // Fallback: textarea + execCommand (deprecated but necessary for HTTP origins).
  // Used when clipboard API is unavailable (e.g., Tailscale preview over plain HTTP).
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.left = "-9999px";
  ta.style.top = "-9999px";
  document.body.appendChild(ta);

  try {
    ta.select();
    ta.setSelectionRange(0, 99999); // For iOS
    const success = document.execCommand("copy");
    return { success };
  } catch (err) {
    return { success: false, error: String(err) };
  } finally {
    document.body.removeChild(ta);
  }
}

/**
 * CopyResultsButton: split button + dropdown menu for format selection.
 * Props: {result, dbId, onStatus}
 * onStatus(notice): called with {text, cls} notice object or null.
 */
export function CopyResultsButton({ result, onStatus }) {
  const [activeFormat, setActiveFormat] = useState(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      return FORMATS.includes(saved) ? saved : "TSV";
    } catch {
      return "TSV";
    }
  });
  const [menuOpen, setMenuOpen] = useState(false);
  const [copying, setCopying] = useState(false);
  const menuRef = useRef();
  const formatters = { TSV: formatTSV, CSV: formatCSV, JSON: formatJSON, Markdown: formatMarkdown };

  const handleCopy = async () => {
    if (!result || !result.columns.length || copying) return;

    setCopying(true);
    try {
      const formatter = formatters[activeFormat];
      const text = formatter(result);
      const { success } = await copyToClipboard(text);

      if (success) {
        const rowCount = result.rows.length;
        const cellsShortened = (result.truncatedCells || []).length;
        let message = `✓ Copied ${rowCount} row${rowCount === 1 ? "" : "s"} as ${activeFormat}`;

        if (result.truncated) {
          message += ` (capped)`;
        }
        if (cellsShortened > 0) {
          message += `; ${cellsShortened} cell${cellsShortened === 1 ? "" : "s"} shortened, use Export for full values`;
        }

        onStatus({ text: message, cls: "ok" });
        // Auto-dismiss after 3s
        setTimeout(() => onStatus(null), 3000);
      } else {
        onStatus({ text: "✗ Copy failed; your browser may not support clipboard on this connection.", cls: "error" });
      }
    } finally {
      setCopying(false);
    }
  };

  const selectFormat = (format) => {
    setActiveFormat(format);
    try { localStorage.setItem(STORAGE_KEY, format); } catch {}
    setMenuOpen(false);
  };

  useEffect(() => {
    if (!menuOpen) return undefined;
    const close = (e) => { if (!menuRef.current?.contains(e.target)) setMenuOpen(false); };
    document.addEventListener("click", close);
    return () => document.removeEventListener("click", close);
  }, [menuOpen]);

  const disabled = !result || !result.columns.length || copying;
  return html`
    <div class="copy-split" ref=${menuRef}>
      <button class="copy-primary" disabled=${disabled} onClick=${handleCopy}
        title=${"Copy visible results as " + activeFormat}>Copy ${activeFormat}</button>
      <button class="copy-menu-btn" disabled=${!result || !result.columns.length} onClick=${() => setMenuOpen(!menuOpen)}
        title="Choose format" aria-label="Copy format" aria-haspopup="menu" aria-expanded=${menuOpen}>▾</button>
      <div class=${"copy-menu" + (menuOpen ? " open" : "")} role="menu">
        ${FORMATS.map((fmt) => html`
          <button key=${fmt} class=${"copy-menu-item" + (fmt === activeFormat ? " active" : "")}
            role="menuitemradio" aria-checked=${fmt === activeFormat} onClick=${() => selectFormat(fmt)}>${fmt}</button>`)}
      </div>
    </div>`;
}
