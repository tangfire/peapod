type ApiErrorPayload = {
  error?: string;
  details?: string[];
};

export class ApiError extends Error {
  details: string[];
  status: number;

  constructor(message: string, details: string[] = [], status = 0) {
    super(message);
    this.name = "ApiError";
    this.details = details;
    this.status = status;
  }
}

export async function api<T = unknown>(url: string, init: RequestInit = {}): Promise<T> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 30000);
  const { headers, signal, ...rest } = init;
  const response = await fetch(url, {
    ...rest,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(headers || {})
    },
    signal: signal || controller.signal
  }).finally(() => window.clearTimeout(timeout));
  const text = await response.text();
  if (!response.ok) {
    const payload = parseAPIErrorPayload(text);
    throw new ApiError(payload.error || text || response.statusText, payload.details || [], response.status);
  }
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

export function errorText(error: unknown): string {
  if (error instanceof DOMException && error.name === "AbortError") return "请求超时，请检查 Pedpod 服务和网络";
  return error instanceof Error ? error.message : String(error || "");
}

export function isUnauthorized(error: unknown): boolean {
  if (error instanceof ApiError) return error.status === 401 || /unauthorized|未登录|登录/i.test(error.message);
  return /unauthorized|未登录|登录/i.test(String(error || ""));
}

function parseAPIErrorPayload(text: string): ApiErrorPayload {
  if (!text) return {};
  try {
    const payload = JSON.parse(text) as ApiErrorPayload;
    if (payload && (payload.error || payload.details)) return payload;
  } catch {
    // Plain text errors are still supported by older handlers.
  }
  return { error: text };
}

import type { DiskDiagnosisResponse, DiskCleanupPreviewResponse, DiskCleanupRequest, DiskCleanupResponse } from "./types";

export async function fetchDiskDiagnosis(): Promise<DiskDiagnosisResponse> {
  return api<DiskDiagnosisResponse>("/api/system/disk-diagnosis");
}

export async function fetchDiskCleanupPreview(): Promise<DiskCleanupPreviewResponse> {
  return api<DiskCleanupPreviewResponse>("/api/system/disk-cleanup-preview");
}

export async function executeDiskCleanup(req: DiskCleanupRequest): Promise<DiskCleanupResponse> {
  return api<DiskCleanupResponse>("/api/system/disk-cleanup", {
    method: "POST",
    body: JSON.stringify(req)
  });
}
