import { afterEach, expect, test, vi } from "vitest";
import { html, render } from "./vendor/preact-htm.js";
import { CopyResultsButton } from "./copy-results.js";

let host;
const result = { columns: ["n"], rows: [[1]] };
async function settle() { await vi.advanceTimersByTimeAsync(50); }
async function mount() {
  vi.useFakeTimers();
  vi.stubGlobal("navigator", { clipboard: { writeText: vi.fn().mockResolvedValue() } });
  host = document.createElement("div"); document.body.append(host);
  const status = vi.fn();
  render(html`<${CopyResultsButton} result=${result} onStatus=${status} />`, host);
  await settle();
  return status;
}
afterEach(() => {
  if (host) { render(null, host); host.remove(); host = null; }
  vi.useRealTimers(); vi.unstubAllGlobals();
});

test("a second copy gets its own three-second notice lifetime", async () => {
  const status = await mount();
  host.querySelector(".copy-primary").click(); await settle();
  await vi.advanceTimersByTimeAsync(2000);
  host.querySelector(".copy-primary").click(); await settle();
  status.mockClear();
  await vi.advanceTimersByTimeAsync(1100);
  expect(status).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(2000);
  expect(status).toHaveBeenCalledExactlyOnceWith(null);
});

test("unmount cancels pending copy-notice dismissal", async () => {
  const status = await mount();
  host.querySelector(".copy-primary").click(); await settle();
  render(null, host); status.mockClear();
  await vi.advanceTimersByTimeAsync(4000);
  expect(status).not.toHaveBeenCalled();
});

test("format disclosure closes with Escape and returns keyboard focus", async () => {
  await mount();
  const toggle = host.querySelector(".copy-menu-btn");
  toggle.click(); await settle();
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  expect(host.querySelector('[role="menu"]')).toBeNull();
  const option = host.querySelector(".copy-menu-item"); option.focus();
  option.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  await settle();
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(document.activeElement).toBe(toggle);
});
