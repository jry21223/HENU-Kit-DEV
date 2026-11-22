import { consoleRequest, readConsoleJSON } from "./console-session";

/**
 * QuizCraft 题库工坊 Console 管理契约客户端
 * 对接 services/console-gateway 已注册的 10 条 /api/v1/quizcraft/workshop/* 白名单代理端点
 *（OpenAPI 定义见 packages/api-contracts/openapi/quizcraft-console-management.yaml）。
 */

export interface WorkshopBankVersionSummary {
  id: string;
  bank_id: string;
  version_number: number;
  state: "draft" | "validated" | "published" | "unpublished" | "legacy" | string;
  active?: boolean;
  question_count?: number;
  source_sha256?: string;
  content_sha256?: string;
  validated_at?: string | null;
  published_at?: string | null;
  created_at?: string;
  notes?: string;
}

export interface WorkshopCatalogBank {
  id: string;
  slug?: string;
  name: string;
  description?: string;
  subject?: string;
  version: number;
  active_version_id?: string | null;
  active_version_number?: number | null;
  question_count?: number;
  updated_at?: string;
  created_at?: string;
  versions?: WorkshopBankVersionSummary[];
}

export interface WorkshopQuestionItem {
  id?: string;
  source_question_id: string;
  type: "single" | "multiple" | "judge" | "blank" | string;
  chapter_id?: string;
  chapter_name?: string;
  content: string;
  options?: string[];
  answer?: unknown;
  explanation?: string;
}

export interface WorkshopVersionDetail {
  id: string;
  bank_id: string;
  bank_name?: string;
  version_number: number;
  state: string;
  source_sha256?: string;
  validated_at?: string | null;
  published_at?: string | null;
  created_at?: string;
  questions: WorkshopQuestionItem[];
}

export interface WorkshopImportIssue {
  index?: number;
  source_question_id?: string;
  field?: string;
  message: string;
}

export interface WorkshopImportReport {
  bank_id?: string;
  bank_version_id?: string;
  version_number?: number;
  accepted_count?: number;
  rejected_count?: number;
  issues?: WorkshopImportIssue[];
  summary?: string;
}

export type WorkshopReadResult<T> =
  | { state: "authenticated"; data: T }
  | { state: "signed_out" }
  | { state: "denied" }
  | { state: "disabled" }
  | { state: "not_found" }
  | { state: "unavailable"; message?: string };

export type WorkshopWriteResult<T = Record<string, unknown>> =
  | { state: "succeeded"; data: T }
  | { state: "validation_failed"; report: WorkshopImportReport; message?: string }
  | { state: "conflict"; message?: string }
  | { state: "signed_out" }
  | { state: "denied" }
  | { state: "disabled" }
  | { state: "invalid"; message?: string }
  | { state: "unavailable"; message?: string };

function extractErrorMessage(payload: unknown): string | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  const record = payload as { error?: { message?: unknown }; message?: unknown };
  if (typeof record.error?.message === "string" && record.error.message.trim()) {
    return record.error.message.trim();
  }
  if (typeof record.message === "string" && record.message.trim()) {
    return record.message.trim();
  }
  return undefined;
}

function extractErrorCode(payload: unknown): string | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  const record = payload as { error?: { code?: unknown } };
  return typeof record.error?.code === "string" ? record.error.code : undefined;
}

function unwrapData<T>(payload: unknown): T {
  if (payload && typeof payload === "object" && "data" in payload) {
    return (payload as { data: T }).data;
  }
  return payload as T;
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await readConsoleJSON(response);
  } catch {
    return undefined;
  }
}

function normalizeQuestionItem(raw: Record<string, unknown>): WorkshopQuestionItem {
  const options = Array.isArray(raw.options)
    ? raw.options.map((o) => String(o))
    : undefined;
  return {
    id: typeof raw.question_id === "string"
      ? raw.question_id
      : typeof raw.id === "string"
        ? raw.id
        : undefined,
    source_question_id: String(raw.source_question_id ?? raw.id ?? ""),
    type: String(raw.type ?? "single"),
    chapter_id: typeof raw.chapter_id === "string" ? raw.chapter_id : undefined,
    chapter_name:
      typeof raw.chapter_name === "string"
        ? raw.chapter_name
        : typeof raw.chapter === "string"
          ? raw.chapter
          : undefined,
    content: String(raw.content ?? ""),
    options,
    answer: raw.answer,
    explanation:
      typeof raw.explanation === "string" && raw.explanation.trim()
        ? raw.explanation
        : typeof raw.analysis === "string" && raw.analysis.trim()
          ? raw.analysis
          : undefined,
  };
}

function normalizeCatalogBank(raw: Record<string, unknown>): WorkshopCatalogBank {
  const bankId = String(raw.bank_id ?? raw.id ?? "");
  const activeVersionId =
    typeof raw.active_version_id === "string" && raw.active_version_id
      ? raw.active_version_id
      : null;
  const rawVersions = Array.isArray(raw.versions) ? raw.versions : [];
  const totalVersions = rawVersions.length;
  const versions: WorkshopBankVersionSummary[] = rawVersions
    .filter((v): v is Record<string, unknown> => Boolean(v && typeof v === "object"))
    .map((v, idx) => {
      const versionId = String(v.bank_version_id ?? v.id ?? "");
      const isActive =
        Boolean(v.active) || (activeVersionId !== null && versionId === activeVersionId);
      const rawState = String(v.state ?? "draft");
      const effectiveState = isActive && rawState !== "published" ? "published" : rawState;
      const versionNum =
        typeof v.version_number === "number" && Number.isFinite(v.version_number)
          ? v.version_number
          : totalVersions - idx;
      return {
        id: versionId,
        bank_id: String(v.bank_id ?? bankId),
        version_number: versionNum,
        state: effectiveState,
        active: isActive,
        question_count: typeof v.question_count === "number" ? v.question_count : undefined,
        source_sha256:
          typeof v.source_sha256 === "string"
            ? v.source_sha256
            : typeof v.content_sha256 === "string"
              ? v.content_sha256
              : undefined,
        content_sha256: typeof v.content_sha256 === "string" ? v.content_sha256 : undefined,
        validated_at: typeof v.validated_at === "string" ? v.validated_at : null,
        published_at: typeof v.published_at === "string" ? v.published_at : null,
        created_at:
          typeof v.created_at === "string"
            ? v.created_at
            : typeof v.imported_at === "string"
              ? v.imported_at
              : typeof v.validated_at === "string"
                ? v.validated_at
                : undefined,
        notes: typeof v.notes === "string" ? v.notes : undefined,
      };
    });

  const activeVersionObj = versions.find((v) => v.active || v.id === activeVersionId);
  const activeVersionNumber =
    typeof raw.active_version_number === "number"
      ? raw.active_version_number
      : activeVersionObj?.version_number ?? null;

  return {
    id: bankId,
    slug:
      typeof raw.bank_key === "string"
        ? raw.bank_key
        : typeof raw.slug === "string"
          ? raw.slug
          : undefined,
    name: String(raw.name ?? ""),
    description: typeof raw.description === "string" ? raw.description : undefined,
    subject: typeof raw.subject === "string" ? raw.subject : undefined,
    version: Number(raw.lifecycle_version ?? raw.version ?? 1),
    active_version_id: activeVersionId,
    active_version_number: activeVersionNumber,
    question_count:
      typeof raw.question_count === "number"
        ? raw.question_count
        : activeVersionObj?.question_count,
    updated_at:
      typeof raw.updated_at === "string"
        ? raw.updated_at
        : activeVersionObj?.validated_at ?? activeVersionObj?.created_at ?? undefined,
    created_at: typeof raw.created_at === "string" ? raw.created_at : undefined,
    versions,
  };
}

function normalizeCatalog(raw: unknown): WorkshopCatalogBank[] {
  const data = unwrapData<unknown>(raw);
  let list: unknown[] = [];
  if (Array.isArray(data)) {
    list = data;
  } else if (data && typeof data === "object") {
    const obj = data as Record<string, unknown>;
    if (Array.isArray(obj.banks)) list = obj.banks;
    else if (Array.isArray(obj.items)) list = obj.items;
  }
  return list
    .filter((item): item is Record<string, unknown> => Boolean(item && typeof item === "object"))
    .map(normalizeCatalogBank);
}

function normalizeVersionDetail(raw: unknown): WorkshopVersionDetail | undefined {
  const data = unwrapData<unknown>(raw);
  if (!data || typeof data !== "object") return undefined;
  const obj = data as Record<string, unknown>;
  const versionObj = (obj.version && typeof obj.version === "object"
    ? obj.version
    : obj) as Record<string, unknown>;
  const rawQuestions = Array.isArray(obj.questions)
    ? obj.questions
    : Array.isArray(versionObj.questions)
      ? versionObj.questions
      : [];
  const questions = rawQuestions
    .filter((q): q is Record<string, unknown> => Boolean(q && typeof q === "object"))
    .map(normalizeQuestionItem);
  return {
    id: String(versionObj.bank_version_id ?? versionObj.id ?? ""),
    bank_id: String(versionObj.bank_id ?? ""),
    bank_name: typeof versionObj.bank_name === "string" ? versionObj.bank_name : undefined,
    version_number: Number(versionObj.version_number ?? versionObj.version ?? 1),
    state: String(versionObj.state ?? "draft"),
    source_sha256:
      typeof versionObj.source_sha256 === "string"
        ? versionObj.source_sha256
        : typeof versionObj.content_sha256 === "string"
          ? versionObj.content_sha256
          : undefined,
    validated_at: typeof versionObj.validated_at === "string" ? versionObj.validated_at : null,
    published_at: typeof versionObj.published_at === "string" ? versionObj.published_at : null,
    created_at: typeof versionObj.created_at === "string" ? versionObj.created_at : undefined,
    questions,
  };
}

function isBridgeDisabled(status: number, code?: string): boolean {
  if (status === 404 && !code) return true;
  if (status === 503 && code && /DISABLED|NOT_CONFIGURED|NOT_ENABLED/i.test(code)) return true;
  return code === "QUIZCRAFT_CONSOLE_DISABLED" || code === "NOT_ENABLED";
}

export async function fetchWorkshopCatalog(options: { signal?: AbortSignal } = {}): Promise<WorkshopReadResult<WorkshopCatalogBank[]>> {
  try {
    const response = await consoleRequest("/api/v1/quizcraft/workshop/catalog", {
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal: options.signal,
    });
    const payload = await safeJSON(response);
    const code = extractErrorCode(payload);
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (isBridgeDisabled(response.status, code)) return { state: "disabled" };
    if (!response.ok) return { state: "unavailable", message: extractErrorMessage(payload) };
    return { state: "authenticated", data: normalizeCatalog(payload) };
  } catch {
    return { state: "unavailable" };
  }
}

export async function fetchWorkshopVersionDetail(
  bankID: string,
  versionID: string,
): Promise<WorkshopReadResult<WorkshopVersionDetail>> {
  try {
    const response = await consoleRequest(
      `/api/v1/quizcraft/workshop/banks/${encodeURIComponent(bankID)}/versions/${encodeURIComponent(versionID)}`,
      {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      },
    );
    const payload = await safeJSON(response);
    const code = extractErrorCode(payload);
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (isBridgeDisabled(response.status, code)) return { state: "disabled" };
    if (response.status === 404) return { state: "not_found" };
    if (!response.ok) return { state: "unavailable", message: extractErrorMessage(payload) };
    const detail = normalizeVersionDetail(payload);
    return detail ? { state: "authenticated", data: detail } : { state: "unavailable" };
  } catch {
    return { state: "unavailable" };
  }
}

async function postWorkshop<T = Record<string, unknown>>(
  path: string,
  body: unknown,
  idempotencyKey: string,
): Promise<WorkshopWriteResult<T>> {
  try {
    const response = await consoleRequest(path, {
      method: "POST",
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        "Idempotency-Key": idempotencyKey,
      },
      body: JSON.stringify(body),
    });
    const payload = await safeJSON(response);
    const code = extractErrorCode(payload);
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (isBridgeDisabled(response.status, code)) return { state: "disabled" };
    if (response.status === 409) return { state: "conflict", message: extractErrorMessage(payload) };
    if (response.status === 422) {
      return {
        state: "validation_failed",
        report: unwrapData<WorkshopImportReport>(payload) ?? {},
        message: extractErrorMessage(payload),
      };
    }
    if (response.status === 400 || response.status === 413) {
      return { state: "invalid", message: extractErrorMessage(payload) };
    }
    if (!response.ok) return { state: "unavailable", message: extractErrorMessage(payload) };
    return { state: "succeeded", data: unwrapData<T>(payload) };
  } catch {
    return { state: "unavailable" };
  }
}

export function createWorkshopBank(
  input: { name: string; slug: string; description?: string; subject?: string },
  idempotencyKey: string,
) {
  return postWorkshop(
    "/api/v1/quizcraft/workshop/banks",
    { bank_key: input.slug, name: input.name },
    idempotencyKey,
  );
}

export function importWorkshopBankVersion(
  bankID: string,
  input: {
    expected_version: number;
    source_sha256: string;
    questions: WorkshopQuestionItem[];
    notes?: string;
  },
  idempotencyKey: string,
) {
  const mappedQuestions = input.questions.map((q) => ({
    source_question_id: q.source_question_id,
    type: q.type === "multiple" ? "multi" : q.type,
    chapter_id: q.chapter_id ?? "ch-01",
    chapter: q.chapter_name ?? "默认章节",
    content: q.content,
    ...(q.options ? { options: q.options } : {}),
    answer: q.answer,
    ...(q.explanation ? { analysis: q.explanation } : {}),
  }));
  return postWorkshop<WorkshopImportReport>(
    `/api/v1/quizcraft/workshop/banks/${encodeURIComponent(bankID)}/imports`,
    {
      expected_version: input.expected_version,
      source_sha256: input.source_sha256,
      questions: mappedQuestions,
    },
    idempotencyKey,
  );
}

export function validateWorkshopBankVersion(
  bankID: string,
  versionID: string,
  input: { expected_version: number; note?: string },
  idempotencyKey: string,
) {
  return postWorkshop(
    `/api/v1/quizcraft/workshop/banks/${encodeURIComponent(bankID)}/versions/${encodeURIComponent(versionID)}/validate`,
    input,
    idempotencyKey,
  );
}

export function publishWorkshopBankVersion(
  bankID: string,
  versionID: string,
  input: { expected_version: number; note?: string },
  idempotencyKey: string,
) {
  return postWorkshop(
    `/api/v1/quizcraft/workshop/banks/${encodeURIComponent(bankID)}/versions/${encodeURIComponent(versionID)}/publish`,
    input,
    idempotencyKey,
  );
}

export function unpublishWorkshopBankVersion(
  bankID: string,
  versionID: string,
  input: { expected_version: number; note?: string },
  idempotencyKey: string,
) {
  return postWorkshop(
    `/api/v1/quizcraft/workshop/banks/${encodeURIComponent(bankID)}/versions/${encodeURIComponent(versionID)}/unpublish`,
    input,
    idempotencyKey,
  );
}

export function rollbackWorkshopBank(
  bankID: string,
  input: { expected_version: number; target_version_id: string; note?: string },
  idempotencyKey: string,
) {
  return postWorkshop(
    `/api/v1/quizcraft/workshop/banks/${encodeURIComponent(bankID)}/rollback`,
    {
      expected_version: input.expected_version,
      target_bank_version_id: input.target_version_id,
      note: input.note ?? "",
    },
    idempotencyKey,
  );
}

export async function sha256Hex(text: string): Promise<string> {
  const bytes = new TextEncoder().encode(text);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
