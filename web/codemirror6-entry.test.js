// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { EditorState } from "@codemirror/state";
import { CompletionContext } from "@codemirror/autocomplete";
import { columnCompletionSource } from "./vendor/src/codemirror6.entry.js";

function labels(raw, columnsByRelation = { access_key_roles: ["id", "role_name"] }) {
  const pos = raw.indexOf("|");
  const doc = raw.slice(0, pos) + raw.slice(pos + 1);
  const state = EditorState.create({ doc });
  const result = columnCompletionSource(columnsByRelation)(new CompletionContext(state, pos, true));
  return result ? result.options.map((o) => o.label) : [];
}

describe("CodeMirror SQL column completion source", () => {
  it("suggests columns from the query relation in select lists", () => {
    expect(labels("SELECT | FROM access_key_roles")).toEqual(["id", "role_name"]);
    expect(labels("SELECT id, | FROM access_key_roles")).toEqual(["id", "role_name"]);
  });

  it("does not replace table suggestions while typing a relation", () => {
    expect(labels("SELECT id FROM access_key_|")).toEqual([]);
  });

  it("resolves schema-qualified and quoted relations", () => {
    expect(labels("SELECT | FROM public.access_key_roles", { "public.access_key_roles": ["id"] })).toEqual(["id"]);
    expect(labels('SELECT | FROM "access_key_roles"')).toEqual(["id", "role_name"]);
  });
});
