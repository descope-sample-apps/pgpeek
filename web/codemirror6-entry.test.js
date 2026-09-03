// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { PostgreSQL, sql } from "@codemirror/lang-sql";
import { diagnosticCount } from "@codemirror/lint";
import { completionStatus, startCompletion } from "@codemirror/autocomplete";
import { runnableRange } from "./vendor/src/codemirror6.entry.js";

function state(raw) {
  const marker = raw.indexOf("|");
  const doc = marker < 0 ? raw : raw.slice(0, marker) + raw.slice(marker + 1);
  return EditorState.create({
    doc,
    selection: marker < 0 ? undefined : { anchor: marker },
    extensions: [sql({ dialect: PostgreSQL })],
  });
}

function mount(doc = "SELECT 1", onRun = vi.fn(), onChange = vi.fn()) {
  const parent = document.createElement("div");
  document.body.append(parent);
  return { editor: window.cm6.mount(parent, doc, onRun, onChange), parent, onRun, onChange };
}

function select(parent, anchor, head = anchor) {
  const view = EditorView.findFromDOM(parent);
  if (!view) throw new Error("mounted EditorView not found");
  view.dispatch({ selection: { anchor, head } });
  return view;
}

beforeEach(() => {
  document.body.textContent = "";
  window.Range.prototype.getClientRects = () => [];
  window.Range.prototype.getBoundingClientRect = () => ({ top: 0, right: 0, bottom: 0, left: 0, width: 0, height: 0 });
});

describe("CodeMirror SQL runnable ranges", () => {
  it("prefers and trims a selection", () => {
    const s = state("  SELECT 1; SELECT 2  ");
    const selected = s.update({ selection: { anchor: 11, head: 21 } }).state;
    expect(runnableRange(selected)).toEqual({ sql: "SELECT 2", from: 12, to: 20, kind: "selection" });
  });

  it("finds the middle statement and includes its semicolon", () => {
    expect(runnableRange(state("SELECT 1; |SELECT 2; SELECT 3;"))).toEqual({
      sql: "SELECT 2;", from: 10, to: 19, kind: "statement",
    });
  });

  it("uses the nearest statement from a trailing comment", () => {
    expect(runnableRange(state("SELECT 1; SELECT 2; -- trailing|"))).toEqual({
      sql: "SELECT 2;", from: 10, to: 19, kind: "statement",
    });
  });

  it("does not split semicolons in strings, comments, or dollar-quoted bodies", () => {
    const doc = "SELECT ';' /* ; */; DO $$ BEGIN PERFORM 1; PERFORM 2; END $$; SELECT 3;";
    const s = EditorState.create({
      doc,
      selection: { anchor: doc.indexOf("PERFORM 2") },
      extensions: [sql({ dialect: PostgreSQL })],
    });
    expect(runnableRange(s).sql).toBe("DO $$ BEGIN PERFORM 1; PERFORM 2; END $$;");
  });

  it("falls back to the trimmed document when no statement exists", () => {
    expect(runnableRange(state("  -- comment only|  "))).toEqual({
      sql: "-- comment only", from: 2, to: 17, kind: "document",
    });
  });
});

describe("CodeMirror SQL adapter", () => {
  it("retains setValue notification behavior", () => {
    const { editor, onChange } = mount();
    editor.setValue("SELECT 2");
    editor.setValue("SELECT 3", false);
    expect(editor.getValue()).toBe("SELECT 3");
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith("SELECT 2");
  });

  it("renders one play control per statement and runs the clicked statement", async () => {
    const { editor, parent, onRun } = mount("SELECT 1;\n\nSELECT 2;\nSELECT 3;");
    await vi.waitFor(() => expect(parent.querySelectorAll(".cm-run-statement")).toHaveLength(3));

    const markers = parent.querySelectorAll(".cm-run-statement");
    expect([...markers].map((marker) => marker.title)).toEqual(["Run statement", "Run statement", "Run statement"]);
    expect([...markers].every((marker) => marker.tagName === "BUTTON" && marker.getAttribute("aria-label") === "Run statement")).toBe(true);
    const icon = markers[0].querySelector("svg path");
    expect(icon.getAttribute("d")).toContain("M6.906 4.537");
    expect(icon.getAttribute("stroke")).toBe("currentColor");
    expect(icon.getAttribute("fill")).toContain("#0fdfb9");
    markers[1].dispatchEvent(new MouseEvent("click", { bubbles: true }));

    expect(onRun).toHaveBeenCalledTimes(1);
    expect(editor.getRunnable()).toEqual({ sql: "SELECT 2;", from: 11, to: 20, kind: "statement" });

    editor.setValue("SELECT 4;");
    await vi.waitFor(() => expect(parent.querySelectorAll(".cm-run-statement")).toHaveLength(1));
  });

  it("uses native nested-schema completion with aliases", async () => {
    const { editor, parent } = mount("SELECT u. FROM users AS u");
    editor.setSQLConfig({ schema: { public: { users: ["id", "email"] } }, defaultSchema: "public" });
    const view = select(parent, 9);
    startCompletion(view);
    await vi.waitFor(() => expect(completionStatus(view.state)).toBe("active"));
    const labels = [...parent.querySelectorAll(".cm-completionLabel")].map((node) => node.textContent);
    expect(labels).toEqual(expect.arrayContaining(["id", "email"]));
    expect(labels.filter((label) => label === "id")).toHaveLength(1);
  });

  it("sets, clears, focuses, and clears diagnostics on edits", () => {
    const { editor, parent } = mount();
    const view = select(parent, 0);
    editor.setDiagnostics([{ from: 0, to: 6, severity: "error", message: "bad query" }]);
    expect(diagnosticCount(view.state)).toBe(1);
    editor.focusRange(0, 6);
    expect(view.state.selection.main).toMatchObject({ from: 0, to: 6 });
    expect(view.hasFocus).toBe(true);
    editor.clearDiagnostics();
    expect(diagnosticCount(view.state)).toBe(0);
    editor.setDiagnostics([{ from: 0, to: 1, severity: "warning", message: "warn" }]);
    editor.setValue("SELECT 2");
    expect(diagnosticCount(view.state)).toBe(0);
  });

  it("discards a lazy formatter result after the document changes", async () => {
    const { editor } = mount("select * from users;");

    const formatting = editor.format();
    editor.setValue("select changed");
    await formatting;

    expect(editor.getValue()).toBe("select changed");
  });

  it("formats only the selected runnable range and never runs", async () => {
    const { editor, parent, onRun } = mount("select 1; select * from users;");
    select(parent, 10, 30);
    await editor.format();
    expect(editor.getValue().slice(0, 10)).toBe("select 1; ");
    expect(editor.getValue()).toContain("SELECT\n  *\nFROM\n  users;");
    expect(onRun).not.toHaveBeenCalled();
  });

  it("formats the statement at the cursor without running", async () => {
    const { editor, parent, onRun } = mount("select 1; select * from users;");
    select(parent, 20);
    await editor.format();
    expect(editor.getValue()).toBe("select 1; SELECT\n  *\nFROM\n  users;");
    expect(onRun).not.toHaveBeenCalled();
  });
});
