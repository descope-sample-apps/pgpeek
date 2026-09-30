import { test, expect } from "vitest";
import { formatTSV, formatCSV, formatJSON, formatMarkdown } from "./copy-results.js";

const mockResult = {
  columns: ["id", "name", "email"],
  rows: [
    [1, "Alice", "alice@example.com"],
    [2, "Bob", null],
    [3, "Carol", "carol@example.com"]
  ]
};

const mockWithDuplicateColumns = {
  columns: ["id", "id", "name"],
  rows: [[1, 100, "Alice"], [2, 200, "Bob"]]
};

const mockWithJSON = {
  columns: ["id", "data"],
  rows: [
    [1, { type: "user", status: "active" }],
    [2, ["tag1", "tag2"]],
    [3, null]
  ]
};

test("formatTSV: basic rows", () => {
  const result = formatTSV(mockResult);
  expect(result).toContain("id\tname\temail");
  expect(result).toContain("1\tAlice\talice@example.com");
  expect(result).toContain("2\tBob\t");
  expect(result).toContain("3\tCarol\tcarol@example.com");
});

test("formatTSV: preserves duplicate column names", () => {
  const result = formatTSV(mockWithDuplicateColumns);
  expect(result).toContain("id\tid\tname");
  expect(result).toContain("1\t100\tAlice");
  expect(result).toContain("2\t200\tBob");
});

test("formatTSV: handles JSON objects and arrays", () => {
  const result = formatTSV(mockWithJSON);
  expect(result).toContain('1\t"{""type"":""user"",""status"":""active""}"');
  expect(result).toContain('2\t"[""tag1"",""tag2""]"');
  expect(result).toContain("3\t");
});

test("formatCSV: RFC 4180 quoting", () => {
  const result = formatCSV(mockResult);
  expect(result).toContain("id,name,email");
  expect(result).toContain("1,Alice,alice@example.com");
  expect(result).toContain("2,Bob,");
});

test("formatCSV: quotes fields with commas", () => {
  const result = {
    columns: ["name"],
    rows: [["Smith, Bob"], ["Alice"]]
  };
  const formatted = formatCSV(result);
  expect(formatted).toContain('"Smith, Bob"');
  expect(formatted).toContain("Alice");
});

test("formatCSV: escapes quotes inside quoted fields", () => {
  const result = {
    columns: ["text"],
    rows: [['He said "hi"']]
  };
  const formatted = formatCSV(result);
  expect(formatted).toContain('"He said ""hi"""');
});

test("formatCSV: handles multiline fields", () => {
  const result = {
    columns: ["description"],
    rows: [["Line 1\nLine 2"]]
  };
  const formatted = formatCSV(result);
  expect(formatted).toContain('"Line 1\nLine 2"');
});

test("formatJSON: {columns, rows} structure", () => {
  const result = formatJSON(mockResult);
  const parsed = JSON.parse(result);
  expect(parsed.columns).toEqual(["id", "name", "email"]);
  expect(parsed.rows.length).toBe(3);
});

test("formatJSON: preserves duplicate columns", () => {
  const result = formatJSON(mockWithDuplicateColumns);
  const parsed = JSON.parse(result);
  expect(parsed.columns).toEqual(["id", "id", "name"]);
  expect(parsed.rows[0]).toEqual([1, 100, "Alice"]);
});

test("formatJSON: preserves null values", () => {
  const result = formatJSON(mockResult);
  const parsed = JSON.parse(result);
  expect(parsed.rows[1][2]).toBeNull();
});

test("formatJSON: embeds JSON objects and arrays", () => {
  const result = formatJSON(mockWithJSON);
  const parsed = JSON.parse(result);
  expect(parsed.rows[0][1]).toEqual({ type: "user", status: "active" });
  expect(parsed.rows[1][1]).toEqual(["tag1", "tag2"]);
});

test("formatMarkdown: pipe-delimited table", () => {
  const result = formatMarkdown(mockResult);
  expect(result).toContain("| id | name | email |");
  expect(result).toContain("| --- | --- | --- |");
  expect(result).toContain("| 1 | Alice | alice@example.com |");
});

test("formatMarkdown: handles NULL values", () => {
  const result = formatMarkdown(mockResult);
  const lines = result.split("\n");
  const secondRow = lines[3]; // First data row after header and separator
  expect(secondRow).toContain("| 2 | Bob |  |");
});

test("formatMarkdown: escapes pipes and newlines so the table stays intact", () => {
  const out = formatMarkdown({ columns: ["text"], rows: [["a | b\nc"]] });
  expect(out.split("\n")).toHaveLength(3);
  expect(out).toContain("| a \\| b<br>c |");
});

test("all formats handle empty columns gracefully", () => {
  const emptyResult = { columns: [], rows: [] };
  expect(formatTSV(emptyResult)).toBe("");
  expect(formatCSV(emptyResult)).toBe("");
  expect(JSON.parse(formatJSON(emptyResult))).toEqual({ columns: [], rows: [] });
  expect(formatMarkdown(emptyResult).split("\n")).toHaveLength(2);
});

test("all formats handle single row", () => {
  const oneRow = { columns: ["a", "b"], rows: [[1, 2]] };
  const tsv = formatTSV(oneRow);
  expect(tsv.split("\n").length).toBe(2); // header + 1 row
  const csv = formatCSV(oneRow);
  expect(csv.split("\n").length).toBe(2);
  const json = JSON.parse(formatJSON(oneRow));
  expect(json.rows.length).toBe(1);
});

test("formatTSV: quotes cells containing tab, newline or quote", () => {
  const out = formatTSV({ columns: ["t"], rows: [["a\tb"], ["l1\nl2"], ['say "hi"'], ["plain"]] });
  expect(out).toBe('t\n"a\tb"\n"l1\nl2"\n"say ""hi"""\nplain');
});

test("formatCSV: quotes cells containing CR", () => {
  expect(formatCSV({ columns: ["t"], rows: [["a\rb"]] })).toBe('t\n"a\rb"');
});
