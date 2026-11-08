import type { ReactNode } from "react";
import { connection } from "next/server";
import { browserDSN } from "../lib/obs/dsn";
import { Observability } from "../lib/obs/provider";

export const metadata = {
  title: "omi-board",
};

export default async function RootLayout({ children }: { children: ReactNode }) {
  // Read the runtime environment, including in the standalone Docker image.
  await connection();
  const dsn = browserDSN(process.env.SENTRY_DSN);
  return (
    <html lang="en">
      <body><Observability dsn={dsn}>{children}</Observability></body>
    </html>
  );
}
