// Read config at request time so one image works in every environment.
export const dynamic = "force-dynamic";

export default function Home() {
  const apiUrl = process.env.PUBLIC_API_URL ?? "";
  const webUrl = process.env.PUBLIC_WEB_URL ?? "";
  return (
    <main>
      <h1>omi-board</h1>
      <p>API: {apiUrl}</p>
      <p>Web: {webUrl}</p>
    </main>
  );
}
