"use client";

import { Component, useEffect, type ReactNode } from "react";
import { initSentry, reportError } from "./sentry";

type Props = { dsn: string; children: ReactNode };

class ErrorBoundary extends Component<Props, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error) {
    reportError(this.props.dsn, error);
  }

  render() {
    return this.state.failed ? (
      <p role="alert">Something went wrong. Please reload the page.</p>
    ) : this.props.children;
  }
}

export function Observability({ dsn, children }: Props) {
  useEffect(() => { initSentry(dsn); }, [dsn]);
  return <ErrorBoundary dsn={dsn}>{children}</ErrorBoundary>;
}
