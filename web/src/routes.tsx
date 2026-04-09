import { Navigate, Route, Routes } from "react-router-dom";

import { AppShell } from "./ui/app-shell";
import { BenchmarkRunDetailPage } from "./ui/benchmark-run-detail-page";
import { BenchmarkSessionDetailPage } from "./ui/benchmark-session-detail-page";
import { BenchmarkSuiteListPage } from "./ui/benchmark-suite-list-page";
import { BenchmarkSuitePage } from "./ui/benchmark-suite-page";
import { ErrorBoundary } from "./ui/error-boundary";
import { TaskRunDetailPage } from "./ui/task-run-detail-page";
import { TaskRunListPage } from "./ui/task-run-list-page";

export function AppRoutes() {
  return (
    <AppShell>
      <ErrorBoundary>
        <Routes>
          <Route path="/" element={<Navigate to="/tasks" replace />} />
          <Route path="/tasks" element={<TaskRunListPage />} />
          <Route path="/tasks/:runID" element={<TaskRunDetailPage />} />
          <Route path="/benchmarks" element={<BenchmarkSuiteListPage />} />
          <Route path="/benchmarks/:suiteID" element={<BenchmarkSuitePage />} />
          <Route path="/benchmarks/:suiteID/sessions/:sessionID" element={<BenchmarkSessionDetailPage />} />
          <Route path="/benchmarks/:suiteID/runs/:scorecardID" element={<BenchmarkRunDetailPage />} />
        </Routes>
      </ErrorBoundary>
    </AppShell>
  );
}
