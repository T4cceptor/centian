import type { PropsWithChildren } from "react";

export function AppShell({ children }: PropsWithChildren) {
  return (
    <div className="app-shell">
      <div className="app-shell__grid" />
      <div className="app-shell__noise" />
      <main className="app-shell__frame">
        <header className="app-header">
          <div>
            <p className="app-header__eyebrow">Centian Monitor</p>
            <h1>Task Runs</h1>
          </div>
          <p className="app-header__subtitle">
            Inspect task workflow progress, current phase, and run volume.
          </p>
        </header>
        <section className="panel">{children}</section>
      </main>
    </div>
  );
}
