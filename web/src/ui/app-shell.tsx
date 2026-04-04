import type { PropsWithChildren } from "react";
import { Link, useLocation } from "react-router-dom";

// Provides the shared chrome around the task run views.
export function AppShell({ children }: PropsWithChildren) {
  const location = useLocation();
  const benchmarkSection = location.pathname.startsWith("/benchmarks");
  const title = benchmarkSection ? "Benchmarks" : "Task Runs";
  const subtitle = benchmarkSection
    ? "Inspect persisted benchmark suites, sessions, scorecards, and comparisons."
    : "Inspect task workflow progress, current phase, and run volume.";

  return (
    <div className="app-shell">
      <div className="app-shell__grid" />
      <div className="app-shell__noise" />
      <main className="app-shell__frame">
        <header className="app-header">
          <div>
            <p className="app-header__eyebrow">Centian Monitor</p>
            <h1>{title}</h1>
            <nav className="app-header__nav" aria-label="Primary">
              <Link className={!benchmarkSection ? "app-header__nav-link app-header__nav-link--active" : "app-header__nav-link"} to="/tasks">
                Tasks
              </Link>
              <Link className={benchmarkSection ? "app-header__nav-link app-header__nav-link--active" : "app-header__nav-link"} to="/benchmarks">
                Benchmarks
              </Link>
            </nav>
          </div>
          <p className="app-header__subtitle">{subtitle}</p>
        </header>
        <section className="panel">{children}</section>
      </main>
    </div>
  );
}
