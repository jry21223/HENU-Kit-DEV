import { getStoredToken } from "./api";
import type { components } from "./generated/admin";

const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";

export type IntegrationStatus = components["schemas"]["IntegrationStatus"];
export type Urgency = components["schemas"]["Urgency"];

export type AdminEnvelope<T> = {
  data: T;
  request_id: string;
};

export type AdminMetric = components["schemas"]["Metric"];
export type DashboardCard = components["schemas"]["DashboardCard"];
export type DashboardSnapshot = components["schemas"]["DashboardSnapshot"];
export type ActionItem = components["schemas"]["ActionItem"];
export type MetricSeries = components["schemas"]["MetricSeriesEnvelope"]["data"];

export type UIConfig = {
  shell_version: "legacy" | "v2";
  dashboard_v2_enabled: boolean;
  environment: string;
  capabilities: {
    global_search: boolean;
    notification_center: boolean;
    legacy_shell: boolean;
  };
};

export async function adminRequest<T>(path: string, init: RequestInit = {}): Promise<AdminEnvelope<T>> {
  const headers = new Headers(init.headers);
  const token = getStoredToken();
  headers.set("Accept", "application/json");
  headers.set("X-Request-Id", createRequestId());
  if (!(init.body instanceof FormData) && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const response = await fetch(`${baseUrl}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Partial<AdminEnvelope<T>> & {
    error?: { message?: string };
    message?: string;
  };
  if (!response.ok || payload.data === undefined) {
    const requestId = payload.request_id ?? response.headers.get("X-Request-Id") ?? "unknown";
    const message = payload.error?.message ?? payload.message ?? `请求失败（${response.status}）`;
    throw new Error(`${message} · request_id: ${requestId}`);
  }
  return payload as AdminEnvelope<T>;
}

function createRequestId() {
  const id = typeof crypto.randomUUID === "function" ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
  return `req_${id}`;
}
