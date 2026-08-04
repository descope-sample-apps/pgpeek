// @vitest-environment jsdom
import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  installKonamiCode, KONAMI_BANNER, KONAMI_TOAST,
  interceptMagicQuery, interceptRun, runEasterEgg,
  detectDangerousSQL, rewriteDropToSelect,
  emptyCreatureText,
  readTableClicks, bumpTableClicks, footerText,
  SHUNI_SCHEMA, SHUNI_VIEW, SHUNI_COLUMNS, EXPLAIN_JOKE,
} from "./easter-eggs.js";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("konami code", () => {
  const SEQ = ["ArrowUp", "ArrowUp", "ArrowDown", "ArrowDown",
    "ArrowLeft", "ArrowRight", "ArrowLeft", "ArrowRight", "b", "a"];

  // Dispatch from a focused element the way a real browser does. Dispatching on
  // `window` would pass even against a bubble-phase listener and so would hide
  // the propagation bugs these tests exist to catch.
  function press(keys, target = document.body) {
    for (const key of keys) {
      target.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
    }
  }

  it("fires the callback when the full sequence is entered", () => {
    const fired = vi.fn();
    const off = installKonamiCode(fired);
    press(SEQ);
    expect(fired).toHaveBeenCalledTimes(1);
    off();
  });

  it("still fires when the focused widget stops propagation", () => {
    // CodeMirror and friends swallow arrow keys — a bubble-phase listener on
    // window never sees them, so the handler has to run in the capture phase.
    const fired = vi.fn();
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.addEventListener("keydown", (e) => e.stopPropagation());
    const off = installKonamiCode(fired);
    press(SEQ, input);
    expect(fired).toHaveBeenCalledTimes(1);
    off();
    input.remove();
  });

  it("still fires for events that do not bubble", () => {
    const fired = vi.fn();
    const off = installKonamiCode(fired);
    for (const key of SEQ) {
      document.body.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: false }));
    }
    expect(fired).toHaveBeenCalledTimes(1);
    off();
  });

  it("matches the sequence with Caps Lock or Shift held", () => {
    const fired = vi.fn();
    const off = installKonamiCode(fired);
    press([...SEQ.slice(0, 8), "B", "A"]);
    expect(fired).toHaveBeenCalledTimes(1);
    off();
  });

  it("does not fire for a partial or wrong sequence", () => {
    const fired = vi.fn();
    const off = installKonamiCode(fired);
    press(["ArrowUp", "ArrowUp", "x"]);
    expect(fired).not.toHaveBeenCalled();
    off();
  });

  it("removes the listener on cleanup", () => {
    const fired = vi.fn();
    const off = installKonamiCode(fired);
    off();
    press(SEQ);
    expect(fired).not.toHaveBeenCalled();
  });

  it("exports banner and toast strings", () => {
    expect(KONAMI_BANNER).toContain("ROOT ACCESS");
    expect(KONAMI_TOAST).toContain("read-only");
  });
});

describe("interceptMagicQuery", () => {
  it("matches magic, ducks, elephants regardless of case/whitespace", () => {
    expect(interceptMagicQuery("SELECT * FROM magic")).toBeTruthy();
    expect(interceptMagicQuery("  select * from ducks ;  ")).toBeTruthy();
    expect(interceptMagicQuery("SELECT   *   FROM   ELEPHANTS")).toBeTruthy();
  });

  it("returns a result shaped like the API payload", () => {
    const r = interceptMagicQuery("select * from magic");
    expect(r).toMatchObject({ rowCount: 1, truncated: false, elapsedMs: 0 });
    expect(r.columns).toContain("spell");
  });

  it("returns null for ordinary SQL", () => {
    expect(interceptMagicQuery("select 1")).toBeNull();
    expect(interceptMagicQuery("select * from users")).toBeNull();
  });
});

describe("detectDangerousSQL", () => {
  it("flags DROP TABLE / DATABASE / SCHEMA", () => {
    expect(detectDangerousSQL("drop table users")).toBe("drop");
    expect(detectDangerousSQL("DROP DATABASE prod")).toBe("drop");
    expect(detectDangerousSQL("DROP SCHEMA auth")).toBe("drop");
  });
  it("flags VACUUM", () => {
    expect(detectDangerousSQL("vacuum")).toBe("vacuum");
    expect(detectDangerousSQL("VACUUM ANALYZE")).toBe("vacuum");
  });
  it("does not intercept VACUUM inside a SELECT", () => {
    expect(detectDangerousSQL("SELECT 'vacuum'")).toBeNull();
    expect(detectDangerousSQL("SELECT vacuum FROM metrics")).toBeNull();
  });
  it("detects VACUUM after leading SQL comments", () => {
    expect(detectDangerousSQL("-- maintenance note\nVACUUM")).toBe("vacuum");
    expect(detectDangerousSQL("/* maintenance note */ VACUUM")).toBe("vacuum");
  });
  it("flags standalone ANALYZE but not EXPLAIN ANALYZE", () => {
    expect(detectDangerousSQL("analyze users")).toBe("analyze");
    expect(detectDangerousSQL("explain analyze select 1")).toBeNull();
  });
  it("returns null for safe SELECTs", () => {
    expect(detectDangerousSQL("select * from users")).toBeNull();
  });
});

describe("rewriteDropToSelect", () => {
  it("rewrites a plain DROP TABLE to SELECT *", () => {
    expect(rewriteDropToSelect("drop table users;")).toBe("SELECT * FROM users;");
  });
  it("preserves schema-qualified names and IF EXISTS", () => {
    expect(rewriteDropToSelect("DROP TABLE IF EXISTS auth.sessions")).toBe("SELECT * FROM auth.sessions;");
  });
  it("falls back to a read-only reminder for non-table drops", () => {
    expect(rewriteDropToSelect("drop database prod")).toBe("SELECT * FROM pgpeek_is_read_only;");
  });
});

describe("interceptRun", () => {
  it("returns a magic egg for magic queries", () => {
    expect(interceptRun("select * from magic").type).toBe("magic");
  });
  it("returns a drop egg for DROP TABLE", () => {
    const eg = interceptRun("drop table users");
    expect(eg.type).toBe("drop");
    expect(eg.rewrite).toBe("SELECT * FROM users;");
    expect(eg.message).toContain("read-only");
  });
  it("returns a whisper egg for VACUUM", () => {
    const eg = interceptRun("vacuum");
    expect(eg.type).toBe("whisper");
    expect(eg.keyword).toBe("VACUUM");
  });
  it("returns null for ordinary SELECT", () => {
    expect(interceptRun("select 1")).toBeNull();
  });
});

describe("runEasterEgg", () => {
  function ctx() {
    return {
      sql: "x",
      setLastSQL: vi.fn(),
      setResult: vi.fn(),
      setError: vi.fn(),
      setSQL: vi.fn(),
      setStatus: vi.fn(),
      wrapRef: { current: null },
    };
  }
  it("sets a result + status for magic eggs", () => {
    const c = ctx();
    runEasterEgg(interceptRun("select * from magic"), c);
    expect(c.setResult).toHaveBeenCalled();
    expect(c.setLastSQL).toHaveBeenCalledWith("");
    expect(c.setError).toHaveBeenCalledWith("");
    expect(c.setStatus.mock.calls[0][0].cls).toBe("ok");
  });
  it("rewrites SQL and errors for drop eggs", () => {
    const c = ctx();
    runEasterEgg(interceptRun("drop table users"), c);
    expect(c.setError).toHaveBeenCalled();
    expect(c.setSQL).toHaveBeenCalledWith("SELECT * FROM users;");
    expect(c.setStatus.mock.calls[0][0].cls).toBe("error");
  });
  it("whispers for vacuum eggs", () => {
    const c = ctx();
    runEasterEgg(interceptRun("vacuum"), c);
    expect(c.setError).toHaveBeenCalledWith(expect.stringContaining("👻"));
    expect(c.setStatus.mock.calls[0][0].cls).toBe("ok");
  });
});

describe("empty-state creatures", () => {
  it("is deterministic for a given seed", () => {
    expect(emptyCreatureText("users")).toBe(emptyCreatureText("users"));
  });
  it("includes an emoji and a quip", () => {
    const t = emptyCreatureText("orders");
    expect(t).toMatch(/\S/);
    expect(t.length).toBeGreaterThan(5);
  });
  it("handles empty/numeric seeds without crashing", () => {
    expect(emptyCreatureText("")).toBeTruthy();
    expect(emptyCreatureText(0)).toBeTruthy();
  });
  it("varies across different seeds", () => {
    const seen = new Set();
    for (const s of ["a", "b", "c", "d", "e", "f", "g", "h"]) seen.add(emptyCreatureText(s));
    expect(seen.size).toBeGreaterThan(1);
  });
});

describe("footer counter", () => {
  it("reads zero when nothing stored", () => {
    expect(readTableClicks()).toBe(0);
  });
  it("bumps and persists", () => {
    expect(bumpTableClicks()).toBe(1);
    expect(readTableClicks()).toBe(1);
    expect(bumpTableClicks()).toBe(2);
  });
  it("footerText scales messages by milestone", () => {
    expect(footerText(0)).toBe("");
    expect(footerText(1)).toContain("1 table");
    expect(footerText(2)).toContain("2 tables");
    expect(footerText(100)).toContain("table champion");
    expect(footerText(250)).toContain("screen break");
  });
  it("survives a throwing sessionStorage", () => {
    const orig = globalThis.sessionStorage;
    Object.defineProperty(globalThis, "sessionStorage", {
      configurable: true,
      get() { throw new Error("denied"); },
    });
    expect(readTableClicks()).toBe(0);
    expect(bumpTableClicks()).toBe(1);
    Object.defineProperty(globalThis, "sessionStorage", { configurable: true, value: orig });
  });
});

describe("sidebar + explain exports", () => {
  it("exports the shuni schema, view, and columns", () => {
    expect(SHUNI_SCHEMA).toContain("shuni");
    expect(SHUNI_SCHEMA).toContain("🐕");
    expect(SHUNI_VIEW.type).toBe("view");
    expect(SHUNI_COLUMNS).toEqual(["dog_name", "tail_wags", "fetched_prs", "good_girl_rating"]);
  });
  it("exports an EXPLAIN joke", () => {
    expect(EXPLAIN_JOKE).toContain("EXPLAIN");
  });
});
