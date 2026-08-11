import { dbUrl } from "./api.js";
import { responseError } from "./sql-helpers.js";

const EXPORT_CSRF_COOKIE = "pgpeek_export_csrf";

export async function countQuery(sql, dbId) {
  const response = await fetch(dbUrl("/api/query/count", dbId), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sql }),
  });
  if (!response.ok) throw new Error(await responseError(response, "count failed"));
  return response.json();
}

export function exportQuery(sql, dbId, onError) {
  const csrf = Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, "0")).join("");
  document.cookie = `${EXPORT_CSRF_COOKIE}=${csrf}; Path=/; SameSite=Strict${location.protocol === "https:" ? "; Secure" : ""}`;
  const frame = document.createElement("iframe");
  frame.name = `pgpeek-export-${csrf}`;
  frame.hidden = true;
  frame.addEventListener("load", () => {
    const text = frame.contentDocument?.body.textContent?.trim();
    if (!text) return;
    try {
      const body = JSON.parse(text);
      if (body.error) onError(body.error);
    } catch {
      return;
    } finally {
      frame.remove();
    }
  });
  document.body.append(frame);
  const form = document.createElement("form");
  form.method = "POST"; form.action = dbUrl("/api/export", dbId); form.target = frame.name;
  const input = document.createElement("input"); input.type = "hidden"; input.name = "sql"; input.value = sql;
  const token = document.createElement("input"); token.type = "hidden"; token.name = "csrf"; token.value = csrf;
  form.append(input, token); document.body.append(form); form.submit(); form.remove();
  setTimeout(() => frame.remove(), 60_000);
}
