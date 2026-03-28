export type TaskRunUIStatus = "active" | "success" | "failed";

// Maps backend task state into the smaller set of visual states used by the UI.
export function getTaskRunUIStatus(status: string, endedAt?: number): TaskRunUIStatus {
  if (endedAt == null) {
    return "active";
  }
  if (status === "failed") {
    return "failed";
  }
  return "success";
}
