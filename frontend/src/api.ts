type ApiErrorPayload = {
  error?: string;
  details?: string[];
};

export class ApiError extends Error {
  details: string[];

  constructor(message: string, details: string[] = []) {
    super(message);
    this.name = "ApiError";
    this.details = details;
  }
}

export async function api<T = unknown>(url: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(url, {
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(init.headers || {})
    },
    ...init
  });
  const text = await response.text();
  if (!response.ok) {
    const payload = parseAPIErrorPayload(text);
    throw new ApiError(payload.error || text || response.statusText, payload.details || []);
  }
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error || "");
}

function parseAPIErrorPayload(text: string): ApiErrorPayload {
  if (!text) return {};
  try {
    const payload = JSON.parse(text) as ApiErrorPayload;
    if (payload && (payload.error || payload.details)) return payload;
  } catch {
    // Plain text errors are still supported by older Zephyr handlers.
  }
  return { error: text };
}
