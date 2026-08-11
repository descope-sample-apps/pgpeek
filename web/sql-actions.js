import { dbUrl } from "./api.js";
import { responseError } from "./sql-helpers.js";

const EXPORT_CSRF_COOKIE = "pgpeek_export_csrf";
const EXPORT_DONE_COOKIE_PREFIX = "pgpeek_export_done_";

export async function countQuery(sql, dbId) {
  const response = await fetch(dbUrl("/api/query/count", dbId), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sql }),
  });
  if (!response.ok) throw new Error(await responseError(response, "count failed"));
  return response.json();
}

export function exportQuery(sql, dbId, onComplete) {
  const csrf = Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, "0")).join("");
  const doneCookie = `${EXPORT_DONE_COOKIE_PREFIX}${csrf}`;
  document.cookie = `${EXPORT_CSRF_COOKIE}=${csrf}; Path=/; SameSite=Strict${location.protocol === "https:" ? "; Secure" : ""}`;
  document.cookie = `${doneCookie}=; Path=/; Max-Age=0; SameSite=Strict`;
  const frame = document.createElement("iframe");
  frame.name = `pgpeek-export-${csrf}`;
  frame.hidden = true;
  let submitted = false;
  let completionTimer;
  const finish = (error = "") => {
    clearTimeout(completionTimer);
    frame.removeEventListener("load", handleLoad);
    frame.remove();
    document.cookie = `${doneCookie}=; Path=/; Max-Age=0; SameSite=Strict`;
    onComplete(error);
  };
  const submit = () => {
    const form = document.createElement("form");
    form.method = "POST"; form.action = dbUrl("/api/export", dbId); form.target = frame.name;
    const input = document.createElement("input"); input.type = "hidden"; input.name = "sql"; input.value = sql;
    const token = document.createElement("input"); token.type = "hidden"; token.name = "csrf"; token.value = csrf;
    form.append(input, token); document.body.append(form); form.submit(); setTimeout(() => form.remove(), 0);
  };
  const handleLoad = () => {
    if (!submitted) {
      submitted = true;
      submit();
      return;
    }
    const text = frame.contentDocument?.body.textContent?.trim();
    if (!text) {
      finish("export failed");
      return;
    }
    try {
      const body = JSON.parse(text);
      finish(body.error || "");
    } catch {
      finish("export failed");
    }
  };
  frame.addEventListener("load", handleLoad);
  document.body.append(frame);
  const waitForCompletion = () => {
    const done = document.cookie.split("; ").some((part) => part === `${doneCookie}=1`);
    if (done) finish();
    else if (frame.isConnected) completionTimer = setTimeout(waitForCompletion, 100);
  };
  completionTimer = setTimeout(waitForCompletion, 100);
}
