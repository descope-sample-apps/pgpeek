export const DEFAULT_SQL = "SELECT now();";

export async function responseError(response, fallback) {
  const body = await response.json().catch(() => null);
  return (body && body.error) || response.statusText || fallback;
}

export function queryStatusText(rowCount, elapsedMs) {
  return "✓ " + rowCount + " row" + (rowCount === 1 ? "" : "s") + " in " + elapsedMs + " ms";
}
