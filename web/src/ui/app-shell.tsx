import { useEffect, useMemo, useState, type ChangeEvent, type PropsWithChildren } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";

import { fetchProjects, type ProjectSummary } from "../api/projects";
import { defaultProjectSlug } from "../api/task-runs";

// Provides the shared chrome around the task run views.
export function AppShell({ children }: PropsWithChildren) {
  const location = useLocation();
  const navigate = useNavigate();
  const [projects, setProjects] = useState<ProjectSummary[]>([defaultProject()]);
  const benchmarkSection = location.pathname.startsWith("/benchmarks");
  const currentProjectSlug = benchmarkSection ? defaultProjectSlug : projectSlugFromPath(location.pathname);
  const eventSection = pathSection(location.pathname) === "events";
  const taskDetailSection = /^\/[^/]+\/tasks\/[^/]+/.test(location.pathname);
  const title = benchmarkSection ? "Benchmarks" : eventSection ? "Events" : "Task Runs";
  const subtitle = benchmarkSection
    ? "Inspect persisted benchmark suites, sessions, scorecards, and comparisons."
    : eventSection
      ? "Inspect all persisted MCP action events across the proxy."
      : "Inspect task workflow progress, current phase, and run volume.";
  const projectExists = benchmarkSection || projects.some((project) => project.slug === currentProjectSlug);
  const showProjectSelector = !benchmarkSection && projects.length > 1;

  useEffect(() => {
    const controller = new AbortController();
    void fetchProjects(controller.signal)
      .then((result) => {
        setProjects(result.length > 0 ? result : [defaultProject()]);
      })
      .catch(() => {
        setProjects([defaultProject()]);
      });
    return () => controller.abort();
  }, []);

  const projectOptions = useMemo(
    () => projects.filter((project) => project.uiEnabled && project.eventStorageEnabled),
    [projects],
  );

  const handleProjectChange = (event: ChangeEvent<HTMLSelectElement>) => {
    const nextProject = event.target.value;
    const section = pathSection(location.pathname);
    const targetSection = section === "events" ? "events" : "tasks";
    navigate(`/${nextProject}/${targetSection}`);
  };

  return (
    <div className="app-shell">
      <div className="app-shell__grid" />
      <div className="app-shell__noise" />
      <main className="app-shell__frame">
        {taskDetailSection ? null : (
          <header className="app-header">
            <div>
              <p className="app-header__eyebrow">Centian Monitor</p>
              <h1>{title}</h1>
              <nav className="app-header__nav" aria-label="Primary">
                <Link className={!benchmarkSection && !eventSection ? "app-header__nav-link app-header__nav-link--active" : "app-header__nav-link"} to={`/${currentProjectSlug}/tasks`}>
                  Tasks
                </Link>
                <Link className={eventSection ? "app-header__nav-link app-header__nav-link--active" : "app-header__nav-link"} to={`/${currentProjectSlug}/events`}>
                  Events
                </Link>
                {currentProjectSlug === defaultProjectSlug ? (
                  <Link className={benchmarkSection ? "app-header__nav-link app-header__nav-link--active" : "app-header__nav-link"} to="/benchmarks">
                    Benchmarks
                  </Link>
                ) : null}
              </nav>
            </div>
            <div className="app-header__side">
              {showProjectSelector ? (
                <label className="app-header__project">
                  <span>Project</span>
                  <select value={currentProjectSlug} onChange={handleProjectChange}>
                    {projectOptions.map((project) => (
                      <option key={project.slug} value={project.slug}>
                        {project.name || project.slug}
                      </option>
                    ))}
                  </select>
                </label>
              ) : null}
              <p className="app-header__subtitle">{subtitle}</p>
            </div>
          </header>
        )}
        <section className="panel">
          {projectExists ? children : (
            <div className="state-card">
              <p className="state-card__eyebrow">Project Missing</p>
              <h2>Project not found</h2>
              <p>The requested project is not configured for this Centian server.</p>
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

function defaultProject(): ProjectSummary {
  return {
    slug: defaultProjectSlug,
    name: "default",
    isDefault: true,
    uiEnabled: true,
    eventStorageEnabled: true,
    taskVerificationEnabled: true,
  };
}

function projectSlugFromPath(pathname: string): string {
  const parts = pathname.split("/").filter(Boolean);
  const [firstSegment, secondSegment] = parts;
  if (!firstSegment || firstSegment === "tasks" || firstSegment === "events") {
    return defaultProjectSlug;
  }
  if (secondSegment !== "tasks" && secondSegment !== "events") {
    return defaultProjectSlug;
  }
  return firstSegment || defaultProjectSlug;
}

function pathSection(pathname: string): string {
  const parts = pathname.split("/").filter(Boolean);
  if (parts[0] === "tasks" || parts[0] === "events") {
    return parts[0];
  }
  return parts.length >= 2 ? parts[1] : parts[0] || "tasks";
}
