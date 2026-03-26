export type TaskRunUIStatus = "active" | "success" | "failed";

export function getTaskRunUIStatus(status: string, endedAt?: number): TaskRunUIStatus {
  if (endedAt == null) {
    return "active";
  }
  if (status === "failed") {
    return "failed";
  }
  return "success";
}
