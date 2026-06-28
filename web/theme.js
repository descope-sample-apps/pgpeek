// Theme selector component + theme application helpers.
import { html, useState, useEffect } from "./vendor/preact-htm.js";

const THEME_KEY = "pgpeek-theme";
const THEMES = [
  ["", "Default"], ["dark-plus", "Dark+"], ["light-plus", "Light+"],
  ["monokai", "Monokai"], ["dracula", "Dracula"], ["one-dark", "One Dark Pro"],
  ["nord", "Nord"], ["solarized-dark", "Solarized Dark"], ["solarized-light", "Solarized Light"],
  ["github-dark", "GitHub Dark"], ["github-light", "GitHub Light"],
  ["catppuccin-mocha", "Catppuccin Mocha"], ["catppuccin-latte", "Catppuccin Latte"],
  ["tokyo-night", "Tokyo Night"], ["ayu-dark", "Ayu Dark"], ["ayu-mirage", "Ayu Mirage"],
  ["night-owl", "Night Owl"], ["houston", "Houston"], ["matcha", "Matcha"], ["dainty", "Dainty"],
];

export function getStoredTheme() {
  try { return localStorage.getItem(THEME_KEY) || ""; } catch { return ""; }
}

export function applyTheme(id) {
  const root = document.documentElement;
  if (id) root.setAttribute("data-theme", id);
  else root.removeAttribute("data-theme");
}

// Apply the saved theme at import time to avoid a flash of the default palette.
applyTheme(getStoredTheme());

export function ThemeSelect() {
  const [theme, setTheme] = useState(getStoredTheme);
  useEffect(() => {
    applyTheme(theme);
    try { localStorage.setItem(THEME_KEY, theme); } catch { /* best-effort */ }
  }, [theme]);
  return html`
    <label class="theme-select" title="Color theme">Theme
      <select id="theme-select" value=${theme} onChange=${(e) => setTheme(e.target.value)}>
        ${THEMES.map(([id, label]) => html`<option value=${id}>${label}</option>`)}
      </select>
    </label>`;
}
