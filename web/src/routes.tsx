import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";

import { AppShell } from "./ui/app-shell";
import { BenchmarkRunDetailPage } from "./ui/benchmark-run-detail-page";
import { BenchmarkSessionDetailPage } from "./ui/benchmark-session-detail-page";
import { BenchmarkSuiteListPage } from "./ui/benchmark-suite-list-page";
import { BenchmarkSuitePage } from "./ui/benchmark-suite-page";
import { EventListPage } from "./ui/event-list-page";
import { ErrorBoundary } from "./ui/error-boundary";
import { TaskRunDetailPage } from "./ui/task-run-detail-page";
import { TaskRunListPage } from "./ui/task-run-list-page";

export function AppRoutes() {
  return (
    <AppShell>
      <ErrorBoundary>
        <Routes>
          <Route path="/" element={<Navigate to="/default/events" replace />} />
          <Route path="/tasks" element={<LegacySectionRedirect section="tasks" />} />
          <Route path="/tasks/:runID" element={<LegacyTaskRunRedirect />} />
          <Route path="/events" element={<LegacySectionRedirect section="events" />} />
          <Route path="/:projectSlug/tasks" element={<TaskRunListPage />} />
          <Route path="/:projectSlug/tasks/:runID" element={<TaskRunDetailPage />} />
          <Route path="/:projectSlug/events" element={<EventListPage />} />
          <Route path="/benchmarks" element={<BenchmarkSuiteListPage />} />
          <Route path="/benchmarks/:suiteID" element={<BenchmarkSuitePage />} />
          <Route path="/benchmarks/:suiteID/sessions/:sessionID" element={<BenchmarkSessionDetailPage />} />
          <Route path="/benchmarks/:suiteID/runs/:scorecardID" element={<BenchmarkRunDetailPage />} />
        </Routes>
      </ErrorBoundary>
    </AppShell>
  );
}

function LegacySectionRedirect({ section }: { section: "tasks" | "events" }) {
  const location = useLocation();
  return <Navigate to={`/default/${section}${location.search}`} replace />;
}

function LegacyTaskRunRedirect() {
  const { runID } = useParams();
  const location = useLocation();
  return <Navigate to={`/default/tasks/${encodeURIComponent(runID ?? "")}${location.search}`} replace />;
}
