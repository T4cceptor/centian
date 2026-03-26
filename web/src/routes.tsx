import { Navigate, Route, Routes } from "react-router-dom";

import { AppShell } from "./ui/app-shell";
import { TaskRunDetailPage } from "./ui/task-run-detail-page";
import { TaskRunListPage } from "./ui/task-run-list-page";

export function AppRoutes() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<Navigate to="/tasks" replace />} />
        <Route path="/tasks" element={<TaskRunListPage />} />
        <Route path="/tasks/:runID" element={<TaskRunDetailPage />} />
      </Routes>
    </AppShell>
  );
}
