import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { splitCriticalCss } from "./docs-css.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const read = (relativePath) =>
  readFile(path.join(root, relativePath), "utf8");

const [
  html,
  manifestText,
  concise,
  full,
  readme,
  sitemap,
  serverRoutes,
  styles,
  deferredStyles,
] =
  await Promise.all([
    read("docs/index.html"),
    read("docs/agent.json"),
    read("docs/llms.txt"),
    read("docs/llms-full.txt"),
    read("README.md"),
    read("docs/sitemap.xml"),
    read("internal/server/server.go"),
    read("docs/styles.css"),
    read("docs/deferred.css"),
  ]);

const manifest = JSON.parse(manifestText);
const count = (source, pattern) => source.match(pattern)?.length ?? 0;
const routeKey = ({ method, path: routePath }) => `${method} ${routePath}`;

const compareRouteSets = (label, actual, expected) => {
  const actualKeys = actual.map(routeKey).sort();
  const expectedKeys = expected.map(routeKey).sort();
  if (JSON.stringify(actualKeys) !== JSON.stringify(expectedKeys)) {
    throw new Error(
      `${label} routes differ:\nactual: ${actualKeys.join(", ")}\nexpected: ${expectedKeys.join(", ")}`,
    );
  }
};

const assertIncludes = (source, value, label) => {
  if (!source.includes(value)) {
    throw new Error(`${label} is missing exact text: ${value}`);
  }
};

const serverHttpRoutes = [...serverRoutes.matchAll(
  /mux\.Handle(?:Func)?\(\s*"([A-Z]+) ([^"]+)"/g,
)].map(([, method, routePath]) => ({ method, path: routePath }));

const endpointSection = full.match(
  /## Endpoints\n([\s\S]*?)(?=\n## |\n---|$)/,
)?.[1] ?? "";
const fullHttpRoutes = [...endpointSection.matchAll(
  /^\|\s+`(GET|POST|PUT|DELETE) ([^`]+)`/gm,
)].map(([, method, routePath]) => ({
  method,
  path: routePath.split("?")[0],
}));

const configSection = html.match(
  /<section class="section" id="config">([\s\S]*?)<\/section>/,
)?.[1] ?? "";
const expectedSummary = {
  sectionCount: count(html, /<section[^>]*id=/g),
  featureCount: count(html, /<article class="feature\b/g),
  themeCount: count(html, /class="frame themes__card"/g),
  safetyLayerCount: count(html, /<article class="layer\b/g),
  documentedConfigCount: count(configSection, /<tr><td class="td-path">/g),
  httpRouteCount: serverHttpRoutes.length,
};

for (const [key, value] of Object.entries(expectedSummary)) {
  if (manifest.summary[key] !== value) {
    throw new Error(
      `docs/agent.json summary.${key}=${manifest.summary[key]}; expected ${value}`,
    );
  }
}

for (const collection of ["resources", "tasks"]) {
  const value = manifest[collection];
  if (value.count !== value.items.length) {
    throw new Error(
      `docs/agent.json ${collection}.count=${value.count}; expected ${value.items.length}`,
    );
  }
}

if (manifest.httpRoutes.count !== manifest.httpRoutes.items.length) {
  throw new Error(
    `docs/agent.json httpRoutes.count=${manifest.httpRoutes.count}; expected ${manifest.httpRoutes.items.length}`,
  );
}
if (manifest.httpRoutes.count !== expectedSummary.httpRouteCount) {
  throw new Error(
    `docs/agent.json httpRoutes.count=${manifest.httpRoutes.count}; expected ${expectedSummary.httpRouteCount}`,
  );
}
for (const route of manifest.httpRoutes.items) {
  if (!/^(GET|POST|PUT|DELETE)$/.test(route.method) || !route.path.startsWith("/")) {
    throw new Error(`docs/agent.json has invalid route ${JSON.stringify(route)}`);
  }
  if (typeof route.description !== "string" || route.description.length === 0) {
    throw new Error(`docs/agent.json route ${routeKey(route)} needs a description`);
  }
}
compareRouteSets("docs/agent.json", manifest.httpRoutes.items, serverHttpRoutes);
compareRouteSets("docs/llms-full.txt", fullHttpRoutes, serverHttpRoutes);

const htmlIds = new Set(
  [...html.matchAll(/\sid="([^"]+)"/g)].map(([, id]) => id),
);
for (const task of manifest.tasks.items) {
  const fragment = task.href.match(/^\.\/#(.+)$/)?.[1];
  if (!fragment) {
    throw new Error(`docs/agent.json task ${task.id} has non-doc fragment ${task.href}`);
  }
  if (!htmlIds.has(fragment)) {
    throw new Error(
      `docs/agent.json task ${task.id} points to missing fragment #${fragment}`,
    );
  }
}

for (const href of ["llms.txt", "llms-full.txt", "agent.json"]) {
  if (!html.includes(`href="${href}"`)) {
    throw new Error(`docs/index.html does not advertise ${href}`);
  }
}

const expectedCss = splitCriticalCss(styles);
const inlineCriticalCss = html.match(
  /<style data-critical-css>\n([\s\S]*?)<\/style>/,
)?.[1];
if (inlineCriticalCss !== expectedCss.critical) {
  throw new Error("docs/index.html critical CSS is stale; run make docs-css");
}
if (deferredStyles !== expectedCss.deferred) {
  throw new Error("docs/deferred.css is stale; run make docs-css");
}
assertIncludes(
  html,
  '<link rel="stylesheet" href="deferred.css" media="print" onload="this.media=\'all\'" />',
  "docs/index.html",
);

if (full.includes("](docs/llms.txt)")) {
  throw new Error(
    "docs/llms-full.txt must link to llms.txt in the same published directory",
  );
}
assertIncludes(
  full,
  "[`llms.txt`](llms.txt)",
  "docs/llms-full.txt",
);

if (!concise.includes("full-context:") || !concise.includes("## Help")) {
  throw new Error("docs/llms.txt must disclose full context and contextual help");
}

for (const statement of [
  `- ${expectedSummary.featureCount} documented product features.`,
  `- ${expectedSummary.themeCount} built-in UI themes.`,
  `- ${expectedSummary.safetyLayerCount} read-only enforcement layers.`,
  `- ${expectedSummary.documentedConfigCount} configuration rows in the website reference.`,
  `- ${expectedSummary.httpRouteCount} documented HTTP routes, including both probes and the UI.`,
]) {
  assertIncludes(concise, statement, "docs/llms.txt");
}

for (const statement of [
  `- Website sections: ${expectedSummary.sectionCount}`,
  `- Product features: ${expectedSummary.featureCount}`,
  `- Built-in themes: ${expectedSummary.themeCount}`,
  `- Read-only enforcement layers: ${expectedSummary.safetyLayerCount}`,
  `- Website configuration rows: ${expectedSummary.documentedConfigCount}`,
  `- Documented HTTP routes: ${expectedSummary.httpRouteCount}`,
]) {
  assertIncludes(full, statement, "docs/llms-full.txt");
}

assertIncludes(
  concise,
  `- Default row cap: ${manifest.summary.defaultRowCap} (\`PGPEEK_ROW_CAP\`).`,
  "docs/llms.txt",
);
assertIncludes(
  concise,
  `- Default statement timeout: ${manifest.summary.defaultStatementTimeout} (\`PGPEEK_STATEMENT_TIMEOUT\`).`,
  "docs/llms.txt",
);
assertIncludes(
  full,
  `- Large query results stop at \`PGPEEK_ROW_CAP\`; the default is ${manifest.summary.defaultRowCap} rows.`,
  "docs/llms-full.txt",
);
const timeoutSeconds = manifest.summary.defaultStatementTimeout.match(/^(\d+)s$/)?.[1];
if (!timeoutSeconds) {
  throw new Error("docs/agent.json summary.defaultStatementTimeout must use seconds, for example 30s");
}
assertIncludes(
  full,
  `- The default statement timeout is ${timeoutSeconds} seconds.`,
  "docs/llms-full.txt",
);

if (!full.includes(readme.trim())) {
  throw new Error("docs/llms-full.txt no longer contains the current README.md");
}

for (const resource of ["llms.txt", "llms-full.txt", "agent.json"]) {
  if (!sitemap.includes(`/pgpeek/${resource}`)) {
    throw new Error(`docs/sitemap.xml does not contain ${resource}`);
  }
}

console.log(`agent docs: ok (${Object.values(expectedSummary).join(", ")})`);
