import * as Sentry from "@sentry/browser";

let client: ReturnType<typeof Sentry.init>;

export function initSentry(dsn: string) {
  // Do not initialize the SDK or install global handlers when disabled.
  if (!dsn) return undefined;
  client ??= Sentry.init({
    dsn,
    sendDefaultPii: false,
    integrations: [
      Sentry.globalHandlersIntegration(),
      Sentry.browserApiErrorsIntegration(),
      Sentry.dedupeIntegration(),
    ],
    defaultIntegrations: false,
    beforeSend(event) {
      delete event.request;
      delete event.breadcrumbs;
      delete event.user;
      return event;
    },
  });
  return client;
}

export function reportError(dsn: string, error: Error) {
  if (initSentry(dsn)) Sentry.captureException(error);
}
