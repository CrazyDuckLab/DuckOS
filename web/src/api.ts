import type { Overview } from "../../shared/api/types";

export class APIError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}

export async function fetchOverview(
  token: string,
  signal: AbortSignal,
): Promise<Overview> {
  const response = await fetch("/api/v1/overview", {
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok) {
    if (response.status === 401 || response.status === 403) {
      throw new APIError(
        response.status,
        "Access was denied. Check your administrator token and reconnect.",
      );
    }
    throw new APIError(
      response.status,
      `The system could not be observed (HTTP ${response.status}). Check the API and Agent services.`,
    );
  }
  const result: Overview = await response.json();
  if (result.schemaVersion !== "1")
    throw new Error(
      "This API schema is not supported by the installed dashboard.",
    );
  return result;
}
