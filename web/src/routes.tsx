import { Navigate, Route, Routes } from "react-router-dom";

import { AppShell } from "./ui/app-shell";
import { TaskRunDetailPlaceholderPage } from "./ui/task-run-detail-placeholder-page";
import { TaskRunListPage } from "./ui/task-run-list-page";

export function AppRoutes() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<Navigate to="/tasks" replace />} />
        <Route path="/tasks" element={<TaskRunListPage />} />
        <Route path="/tasks/:runID" element={<TaskRunDetailPlaceholderPage />} />
      </Routes>
    </AppShell>
  );
}
