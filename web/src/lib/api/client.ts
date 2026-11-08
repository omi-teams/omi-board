import createClient, { type Middleware } from "openapi-fetch";
import type { paths } from "./schema";

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

// Held in memory only: the session endpoint returns it again after a page reload.
let csrfToken: string | undefined;

const session: Middleware = {
  onRequest({ request }) {
    if (csrfToken && !SAFE_METHODS.has(request.method)) {
      request.headers.set("X-CSRF-Token", csrfToken);
    }
    return request;
  },
  async onResponse({ request, response }) {
    const path = new URL(request.url).pathname;
    if (path.endsWith("/v1/auth/logout") || response.status === 401) {
      csrfToken = undefined;
    } else if (
      response.ok &&
      (path.endsWith("/v1/auth/login") || path.endsWith("/v1/auth/session"))
    ) {
      csrfToken = (await response.clone().json()).csrf_token;
    }
    return response;
  },
};

export const client = createClient<paths>({ credentials: "include" });
client.use(session);
