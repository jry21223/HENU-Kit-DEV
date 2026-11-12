import { apiFetchRequired, PortalApiError } from "./client";
import type {
  TrainingAnswerReceiptEnvelope,
  TrainingAnswerSubmission,
  TrainingCatalogEnvelope,
  TrainingCreateSession,
  TrainingLayerDisclosureEnvelope,
  TrainingRevealRequest,
  TrainingReviewEnvelope,
  TrainingReviewItemsEnvelope,
  TrainingSessionEnvelope,
} from "./generated/training";

const prefix = "/api/v1/practice/training";
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[1-5][a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/i;

function sessionPath(sessionID: string): string {
  if (!uuid.test(sessionID)) throw new PortalApiError("无法识别这次训练，请从听力目录重新开始。");
  return `${prefix}/sessions/${sessionID}`;
}

function command(key: string, input: TrainingCreateSession | TrainingAnswerSubmission | TrainingRevealRequest): RequestInit {
  if (!/^[A-Za-z0-9._:-]{16,160}$/.test(key)) throw new PortalApiError("无法提交这次操作，请重新打开训练。");
  return {
    method: "POST", cache: "no-store", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "Idempotency-Key": key },
    body: JSON.stringify(input),
  };
}

export const fetchTrainingCatalog = (signal?: AbortSignal) =>
  apiFetchRequired<TrainingCatalogEnvelope>(`${prefix}/catalog`, { cache: "no-store", credentials: "same-origin", signal });

export const createTrainingSession = (input: TrainingCreateSession, key: string) =>
  apiFetchRequired<TrainingSessionEnvelope>(`${prefix}/sessions`, command(key, input));

export const fetchTrainingSession = (sessionID: string, signal?: AbortSignal) =>
  apiFetchRequired<TrainingSessionEnvelope>(sessionPath(sessionID), { cache: "no-store", credentials: "same-origin", signal });

export const submitTrainingAnswer = (sessionID: string, input: TrainingAnswerSubmission, key: string) =>
  apiFetchRequired<TrainingAnswerReceiptEnvelope>(`${sessionPath(sessionID)}/answers`, command(key, input));

export const fetchTrainingReview = (sessionID: string, signal?: AbortSignal) =>
  apiFetchRequired<TrainingReviewEnvelope>(`${sessionPath(sessionID)}/review`, { cache: "no-store", credentials: "same-origin", signal });

export const revealTrainingLayer = (sessionID: string, input: TrainingRevealRequest, key: string) =>
  apiFetchRequired<TrainingLayerDisclosureEnvelope>(`${sessionPath(sessionID)}/reveals`, command(key, input));

export function fetchTrainingReviewItems(cursor?: string, signal?: AbortSignal) {
  if (cursor !== undefined && !/^[A-Za-z0-9_-]{1,512}$/.test(cursor)) throw new PortalApiError("复习列表无法继续读取，请刷新列表后重试。");
  return apiFetchRequired<TrainingReviewItemsEnvelope>(`${prefix}/review-items${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`, { cache: "no-store", credentials: "same-origin", signal });
}
