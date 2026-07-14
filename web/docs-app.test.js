// @vitest-environment node

import { readFileSync } from "node:fs";
import { JSDOM } from "jsdom";
import { describe, expect, it } from "vitest";

const docsApp = readFileSync(new URL("../docs/app.js", import.meta.url), "utf8");

function renderDocsNav() {
  return new JSDOM(
    `<!doctype html>
      <html>
        <body>
          <header class="nav">
            <a class="nav__logo" href="#top">pgpeek</a>
            <button class="nav__hamburger" aria-expanded="false">Menu</button>
          </header>
          <nav class="nav__mobile">
            <a href="#features">Features</a>
            <a href="https://example.com">GitHub</a>
          </nav>
          <main><section id="features"><a href="#top">Hero action</a></section></main>
          <footer><a href="#top">Footer action</a></footer>
          <script>${docsApp}</script>
        </body>
      </html>`,
    {
      runScripts: "dangerously",
      url: "https://example.com/",
      beforeParse(window) {
        window.matchMedia = () => ({
          matches: false,
          addEventListener() {},
        });
        window.scrollTo = () => {};
      },
    },
  );
}

describe("docs mobile navigation", () => {
  it("moves focus to the first menu link when opened", () => {
    const dom = renderDocsNav();
    const { document } = dom.window;
    const hamburger = document.querySelector(".nav__hamburger");
    const firstLink = document.querySelector(".nav__mobile a");

    hamburger.click();

    expect(document.activeElement).toBe(firstLink);
    dom.window.close();
  });

  it("keeps forward Tab inside the open menu", () => {
    const dom = renderDocsNav();
    const { document, KeyboardEvent } = dom.window;
    const hamburger = document.querySelector(".nav__hamburger");
    const lastLink = document.querySelector(".nav__mobile a:last-of-type");

    hamburger.click();
    lastLink.focus();
    const event = new KeyboardEvent("keydown", {
      key: "Tab",
      bubbles: true,
      cancelable: true,
    });
    document.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(hamburger);
    dom.window.close();
  });

  it("keeps reverse Tab inside the open menu", () => {
    const dom = renderDocsNav();
    const { document, KeyboardEvent } = dom.window;
    const hamburger = document.querySelector(".nav__hamburger");
    const lastLink = document.querySelector(".nav__mobile a:last-of-type");

    hamburger.click();
    hamburger.focus();
    const event = new KeyboardEvent("keydown", {
      key: "Tab",
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    });
    document.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(lastLink);
    dom.window.close();
  });

  it("makes background regions inert only while the menu is open", () => {
    const dom = renderDocsNav();
    const { document } = dom.window;
    const hamburger = document.querySelector(".nav__hamburger");
    const main = document.querySelector("main");
    const footer = document.querySelector("footer");

    hamburger.click();
    expect(main.hasAttribute("inert")).toBe(true);
    expect(footer.hasAttribute("inert")).toBe(true);

    hamburger.click();
    expect(main.hasAttribute("inert")).toBe(false);
    expect(footer.hasAttribute("inert")).toBe(false);
    dom.window.close();
  });

  it("restores focus and background state when Escape closes the menu", () => {
    const dom = renderDocsNav();
    const { document, KeyboardEvent } = dom.window;
    const hamburger = document.querySelector(".nav__hamburger");
    const mobileNav = document.querySelector(".nav__mobile");
    const main = document.querySelector("main");
    const footer = document.querySelector("footer");

    hamburger.click();
    document.dispatchEvent(new KeyboardEvent("keydown", {
      key: "Escape",
      bubbles: true,
    }));

    expect(hamburger.getAttribute("aria-expanded")).toBe("false");
    expect(mobileNav.classList.contains("open")).toBe(false);
    expect(main.hasAttribute("inert")).toBe(false);
    expect(footer.hasAttribute("inert")).toBe(false);
    expect(document.body.style.overflow).toBe("");
    expect(document.activeElement).toBe(hamburger);
    dom.window.close();
  });
});
