export type TaskRunUIStatus = "active" | "completed" | "failed";

export function getTaskRunUIStatus(status: string, endedAt?: number): TaskRunUIStatus {
  if (endedAt == null) {
    return "active";
  }
  if (status === "failed") {
    return "failed";
  }
  return "completed";
}
