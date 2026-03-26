import { Link, useParams } from "react-router-dom";

export function TaskRunDetailPlaceholderPage() {
  const { runID } = useParams();

  return (
    <div className="state-card state-card--detail">
      <p className="state-card__eyebrow">Reserved Channel</p>
      <h2>Task run detail view</h2>
      <p>
        Timeline rendering lands in the next ticket. Selected run:{" "}
        <code>{runID ?? "unknown"}</code>
      </p>
      <Link className="back-link" to="/tasks">
        Back to task runs
      </Link>
    </div>
  );
}
