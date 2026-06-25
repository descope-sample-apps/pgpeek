# DESIGN.md — pgpeek docs

Design system for the pgpeek documentation site. Every color, font size,
spacing value, and motion parameter must trace back to a token defined here.
No component may hardcode a visual value that exists as a token.

---

## 1. Atmosphere

Dark, minimal, engineering-first. Inspired by terminal UIs and modern
developer tooling (Raycast, Linear). The primary surface is near-black;
text is near-white; a single bold blue (`--c-accent`) anchors interactive
and brand moments.

Code is a first-class design element: monospaced type, syntax coloring, and
code-editor chrome define the aesthetic — not imagery or illustration.

---

## 2. Color

All values are CSS custom properties declared in `docs/styles.css → :root`.

### Brand palette
| Token | Value | Use |
|---|---|---|
| `--c-bg` | `#0f1115` | Page background |
| `--c-panel` | `#181b22` | Cards, panels, selected-tab background |
| `--c-border` | `#2a2f3a` | All borders |
| `--c-text` | `#e6e8eb` | Primary text |
| `--c-muted` | `#9aa3b2` | Secondary text, labels, icons |
| `--c-accent` | `#4f8cff` | Brand blue — CTAs, links, active states |
| `--c-success` | `#3ecf8e` | Positive status, string literals |
| `--c-danger` | `#ff5c5c` | Error, DELETE method badge |
| `--c-warn` | `#ffce5c` | Warning, ★ recommended badge |

### Derived (do not override)
| Token | Value | Use |
|---|---|---|
| `--c-panel-hi` | `#1d2230` | Hover state on panel rows |
| `--c-accent-bg` | `rgba(79,140,255,.09)` | Accent tint fill |
| `--c-success-bg` | `rgba(62,207,142,.10)` | Success tint fill |
| `--c-code-bg` | `#12151b` | Code blocks, tab-list background |
| `--c-nav-bg` | `rgba(15,17,21,.84)` | Nav (backdrop-filter surface) |

### Usage rules
- Always reference tokens; never hardcode hex in components.
- Semantic tints: fill `rgba(token-rgb, 0.08–0.12)`, border `rgba(token-rgb, 0.18–0.25)`.
- Surface layer order (back → front): `--c-bg` → `--c-code-bg` → `--c-panel` → `--c-panel-hi`.

---

## 3. Typography

### Font stacks
| Token | Stack | Use |
|---|---|---|
| `--font-sans` | `Sora, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif` | Display, headings, logo |
| `--font-system` | `-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif` | Body copy |
| `--font-mono` | `JetBrains Mono, Fira Code, ui-monospace, Cascadia Code, monospace` | Code, badges, tab buttons, labels |

### Type scale
| Token | Value | Typical use |
|---|---|---|
| `--text-xs` | `0.6875rem` | Micro labels, copy button, table headers |
| `--text-sm` | `0.8125rem` | Table text, secondary copy |
| `--text-base` | `0.9375rem` | Body copy |
| `--text-lg` | `1.0625rem` | Lead paragraphs |
| `--text-xl` | `1.1875rem` | Card titles |
| `--text-2xl` | `1.375rem` | Sub-section headings |
| `--text-3xl` | `1.75rem` | Mobile section titles |
| `--text-4xl` | `2.25rem` | Section titles (desktop) |
| `--text-hero` | `clamp(2.5rem, 5vw + .5rem, 4.25rem)` | Hero headline |

### Rules
- Display headings: `--font-sans`, `font-weight: 700`, `letter-spacing: -.03em`.
- Body: `--font-system`, `line-height: 1.65`.
- Code-adjacent labels (badges, tab buttons, variable names): `--font-mono`.

---

## 4. Spacing & Layout

### Spacing scale (8 px base)
| Token | Value |
|---|---|
| `--sp-1` | `0.25rem` |
| `--sp-2` | `0.5rem` |
| `--sp-3` | `0.75rem` |
| `--sp-4` | `1rem` |
| `--sp-5` | `1.25rem` |
| `--sp-6` | `1.5rem` |
| `--sp-8` | `2rem` |
| `--sp-10` | `2.5rem` |
| `--sp-12` | `3rem` |
| `--sp-16` | `4rem` |
| `--sp-20` | `5rem` |
| `--sp-24` | `6rem` |

### Layout tokens
| Token | Value | Use |
|---|---|---|
| `--max-w` | `1200px` | `.container` max-width |
| `--nav-h` | `60px` | Sticky nav height (56 px at < 480 px) |
| `--sec-py` | `clamp(4rem, 8vw, 6rem)` | Section vertical padding |

### Grid conventions
- Responsive card grids: `grid-template-columns: repeat(auto-fit, minmax(N, 1fr))`.
- Card gap: `var(--sp-4)`. Section gap: `var(--sp-8)`.

---

## 5. Components

### Buttons
Two variants: `.btn--primary` (accent fill) · `.btn--ghost` (outlined/muted).
Sizes: default · `.btn--sm` · `.btn--lg`.
Active state: `transform: translateY(1px)`.

### Badges & Pills
- `.badge--success` / `.badge--accent` / `.badge--muted` — semantic inline chips.
- `.pill-required` (warn yellow) / `.pill-default` (muted) — table default column.

### Code blocks
- `pre`: `--c-code-bg` fill, `--c-border` border, `--r-lg` radius, JetBrains Mono, `--text-sm`.
- Inline `code`: accent-tinted fill, accent border, `--r-sm` radius.
- Syntax tokens: `.tok-comment` `#4d5566` · `.tok-string` success green · `.tok-key` accent
  blue · `.tok-var` warn yellow · `.tok-flag` danger red · `.tok-kw` purple.

### Tabs (Quick Start)
Tab group = `.tab-list` (top bar, no bottom border, `--c-code-bg` fill) +
`.tab-panels` (body, no top border, `--c-panel` fill, `var(--sp-5)` padding).
Together they form one bordered container rounded with `--r-lg` on all outer corners.
Selected tab shows an accent inset top shadow (`box-shadow: inset 0 2px 0 var(--c-accent)`).
Copy button is always visible so mouse, keyboard, and touch users get the same affordance.

### Cards
Background `--c-panel`, border `--c-border`, radius `--r-xl`, inner padding `--sp-6`.

### Data tables
- Scrollable wrapper (`.table-wrap`): `overflow-x: auto`, `--r-xl` radius, `--c-border` border.
- Column 1: monospaced (`.td-path`). All text: `--c-muted`. Row hover: `--c-panel-hi`.

---

## 6. Motion & Interaction

### Transition tokens
| Token | Value | Use |
|---|---|---|
| `--t-fast` | `120ms ease` | Hover colours, opacity fades |
| `--t-base` | `220ms ease` | Tab switching, button states |
| `--t-slow` | `380ms ease` | Scroll reveal enter animation |

### Scroll reveal
`.reveal` starts `opacity: 0; transform: translateY(18px)`. An intersection-observer
adds `.revealed` to transition to visible. Stagger: `.reveal-delay-1` through
`.reveal-delay-4` add 60 ms increments.

### Rules
- No layout-property animations (`width`, `height`, `top`, `left`).
- GPU-composited only: `transform`, `opacity`, `filter`.
- Respect `prefers-reduced-motion: reduce` — all durations collapse to `.01ms`.
- `.reveal` becomes instant (no opacity/transform) under reduced motion.

---

## 7. Depth & Surface

### Shadow tokens
| Token | Value | Use |
|---|---|---|
| `--sh-sm` | `0 1px 3px rgba(0,0,0,.45)` | Subtle lift |
| `--sh-md` | `0 4px 16px rgba(0,0,0,.55)` | Standard card elevation |
| `--sh-lg` | `0 8px 40px rgba(0,0,0,.65)` | Elevated panels |
| `--sh-xl` | `0 20px 72px rgba(0,0,0,.75)` | Hero mockup, deepest layer |

### Border radius system
| Token | Value | Use |
|---|---|---|
| `--r-sm` | `4px` | Inline code, copy button, chips, method badges |
| `--r-md` | `8px` | Buttons, pre inside tab panels |
| `--r-lg` | `12px` | Tab group outer corners, standalone pre |
| `--r-xl` | `16px` | Deploy cards, table wrappers, stat cards |
| `--r-2xl` | `20px` | Reserved for hero / featured elements |
| `--r-full` | `9999px` | Pills, badges, dot indicators |

### Surface layering (back → front)
1. `--c-bg` — page background
2. `--c-code-bg` — code surfaces, tab-list header
3. `--c-panel` — cards, tab panel body, selected tab
4. `--c-panel-hi` — hover rows / highlighted surfaces
5. Nav (sticky, `backdrop-filter: blur(16px)`, `--c-nav-bg`)

---

## 8. App Console Addendum

The embedded `web/` app reuses the same dark developer-tool atmosphere, but its
tokens live in `web/index.html` because the app ships as static embedded files
without a build step.

- Use `--bg`, `--panel`, `--elev`, `--elev2`, `--border`, `--hover`, `--text`,
  `--muted`, `--accent`, `--accent-hover`, `--on-accent`, and semantic status
  tokens for all console surfaces.
- Current database/table context must be visible in the main pane, not only in a
  scrollable sidebar selection.
- Data-export actions use an accent-outline `.action.secondary` treatment for
  readable contrast across themes.
- Every interactive control needs hover, pressed, disabled, and visible
  `:focus-visible` states.
- Sidebar table selection uses `aria-current="true"` and must scroll into view
  when changed.
- Mobile layout stacks the sidebar above the main pane and must avoid horizontal
  page overflow; result tables may scroll inside their own results container.
