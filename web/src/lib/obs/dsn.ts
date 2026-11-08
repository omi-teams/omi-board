// Sentry's public DSN is safe for the browser; strip legacy private keys and
// query parameters before crossing the server/client component boundary.
export function browserDSN(dsn: string | undefined): string {
  if (!dsn) return "";
  try {
    const url = new URL(dsn);
    if (!["http:", "https:"].includes(url.protocol) || !url.username) return "";
    url.password = "";
    url.search = "";
    url.hash = "";
    return url.toString();
  } catch {
    return "";
  }
}
