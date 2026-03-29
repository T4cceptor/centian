export type TaskRunUIStatus = "active" | "success" | "failed" | "timed_out";

// Maps backend task state into the smaller set of visual states used by the UI.
export function getTaskRunUIStatus(status: string, endedAt?: number): TaskRunUIStatus {
  if (status === "timed_out") {
    return "timed_out";
  }
  if (endedAt == null) {
    return "active";
  }
  if (status === "failed") {
    return "failed";
  }
  return "success";
}
