import { readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { splitCriticalCss } from "./docs-css.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const stylesPath = path.join(root, "docs/styles.css");
const deferredPath = path.join(root, "docs/deferred.css");
const indexPath = path.join(root, "docs/index.html");

const [styles, index] = await Promise.all([
  readFile(stylesPath, "utf8"),
  readFile(indexPath, "utf8"),
]);
const { critical, deferred } = splitCriticalCss(styles);
const criticalTag = `<style data-critical-css>\n${critical}</style>`;
const nextIndex = index.replace(
  /<style data-critical-css>[\s\S]*?<\/style>/,
  criticalTag,
);

if (nextIndex === index && !index.includes(criticalTag)) {
  throw new Error("docs/index.html is missing <style data-critical-css>");
}

await Promise.all([
  writeFile(deferredPath, deferred),
  writeFile(indexPath, nextIndex),
]);

console.log("docs css: synchronized critical and deferred styles");
