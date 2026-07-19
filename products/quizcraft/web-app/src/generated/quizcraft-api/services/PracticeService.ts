/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { AnswerResultEnvelope } from '../models/AnswerResultEnvelope';
import type { AnswerSubmission } from '../models/AnswerSubmission';
import type { BankListEnvelope } from '../models/BankListEnvelope';
import type { CreatePracticeSession } from '../models/CreatePracticeSession';
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { OperationKind } from '../models/OperationKind';
import type { PracticeSessionEnvelope } from '../models/PracticeSessionEnvelope';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class PracticeService {
    /**
     * List published practice banks
     * @returns BankListEnvelope Published banks
     * @throws ApiError
     */
    public static listPracticeBanks(): CancelablePromise<BankListEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/banks',
            errors: {
                400: `Invalid request`,
            },
        });
    }
    /**
     * Create a random, difficult, chapter, or favorites practice session
     * @returns PracticeSessionEnvelope Session pinned to immutable content
     * @throws ApiError
     */
    public static createPracticeSession({
        idempotencyKey,
        requestBody,
    }: {
        idempotencyKey: string,
        requestBody: CreatePracticeSession,
    }): CancelablePromise<PracticeSessionEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/practice/sessions',
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Invalid request`,
                409: `Idempotency payload or optimistic version conflict`,
            },
        });
    }
    /**
     * Submit an answer for server-side evaluation
     * @returns AnswerResultEnvelope Server-confirmed result and explanation
     * @throws ApiError
     */
    public static submitPracticeAnswer({
        sessionId,
        idempotencyKey,
        requestBody,
    }: {
        sessionId: string,
        idempotencyKey: string,
        requestBody: AnswerSubmission,
    }): CancelablePromise<AnswerResultEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/practice/sessions/{session_id}/answers',
            path: {
                'session_id': sessionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Invalid request`,
                409: `Idempotency payload or optimistic version conflict`,
            },
        });
    }
    /**
     * Resolve an idempotent write after an unknown outcome
     * @returns OperationEnvelope Original write state and correlated request
     * @throws ApiError
     */
    public static getQuizCraftOperation({
        operationKind,
        idempotencyKey,
    }: {
        operationKind: OperationKind,
        idempotencyKey: string,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/operations/{operation_kind}',
            path: {
                'operation_kind': operationKind,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                404: `Resource or operation is unknown to this actor`,
            },
        });
    }
}
