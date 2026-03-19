import { Outlet } from "react-router-dom";

export function Layout() {
  return (
    <div className="min-h-screen bg-background text-on-background">
      <header className="glass sticky top-0 z-50 h-16 flex items-center border-b border-outline-variant px-6">
        <div className="flex items-center gap-3">
          <svg
            className="h-5 w-5 text-primary"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M21 12V7H5a2 2 0 0 1 0-4h14v4" />
            <path d="M3 5v14a2 2 0 0 0 2 2h16v-5" />
            <path d="M18 12a2 2 0 0 0 0 4h4v-4Z" />
          </svg>
          <h1 className="font-headline text-lg font-semibold tracking-tight text-on-surface">
            CAPZ Prow Dashboard
          </h1>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-6 py-6">
        <Outlet />
      </main>
    </div>
  );
}
