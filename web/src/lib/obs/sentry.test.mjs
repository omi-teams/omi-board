import assert from "node:assert/strict";
import { test } from "node:test";
import { browserDSN } from "./dsn.ts";

test("browser DSN excludes private keys and query secrets", () => {
  assert.equal(browserDSN(undefined), "");
  assert.equal(browserDSN(""), "");
  assert.equal(browserDSN("invalid"), "");
  assert.equal(browserDSN("https://public:private@example.com/1?secret=value#secret"), "https://public@example.com/1");
});

test("disabled Sentry makes no network calls; enabled Sentry reports errors", async () => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  const envelopes = [];
  globalThis.fetch = async (_input, init) => {
    calls++;
    envelopes.push(String(init?.body));
    return new Response("{}", { status: 200 });
  };
  try {
    const { initSentry, reportError } = await import("./sentry.ts");
    assert.equal(initSentry(""), undefined);
    reportError("", new Error("disabled error"));
    await new Promise((resolve) => setTimeout(resolve, 20));
    assert.equal(calls, 0);

    const client = initSentry("https://public@example.com/1");
    assert.ok(client);
    reportError("https://public@example.com/1", new Error("enabled error"));
    assert.equal(await client.flush(2000), true);
    assert.equal(calls, 1);
    assert.ok(envelopes[0].includes("enabled error"));
    await client.close();
  } finally {
    globalThis.fetch = originalFetch;
  }
});
