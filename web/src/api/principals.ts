// Principals data client — lists the principals (agents) seen in events so the
// Activity and Events views can offer a "filter by principal" dropdown.

import { projectApiPath, requestJSON } from "./task-runs";

export type PrincipalRef = {
  id: string;
  displayName: string;
};

type PrincipalsResponse = {
  principals: PrincipalRef[];
};

// fetchPrincipals lists principals seen in events. When start/end are provided the
// list is scoped to that window (used by Activity); omit them for all-time (Events).
export async function fetchPrincipals(
  projectSlug: string | undefined,
  window: { start?: number; end?: number } = {},
  signal?: AbortSignal,
): Promise<PrincipalRef[]> {
  const params = new URLSearchParams();
  if (window.start) params.set("start", String(window.start));
  if (window.end) params.set("end", String(window.end));
  const query = params.toString();
  const response = await requestJSON<PrincipalsResponse>(
    `${projectApiPath(projectSlug, "/principals")}${query ? `?${query}` : ""}`,
    signal,
  );
  return Array.isArray(response?.principals) ? response.principals : [];
}
