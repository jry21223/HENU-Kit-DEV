/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { AnswerResultEnvelope } from '../models/AnswerResultEnvelope';
import type { AnswerSubmission } from '../models/AnswerSubmission';
import type { BankListEnvelope } from '../models/BankListEnvelope';
import type { CreatePracticeSession } from '../models/CreatePracticeSession';
import type { CutoverEvidenceEnvelope } from '../models/CutoverEvidenceEnvelope';
import type { HealthEnvelope } from '../models/HealthEnvelope';
import type { LearningStateEnvelope } from '../models/LearningStateEnvelope';
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { OperationKind } from '../models/OperationKind';
import type { PersonalPracticeStatsEnvelope } from '../models/PersonalPracticeStatsEnvelope';
import type { PracticeSessionEnvelope } from '../models/PracticeSessionEnvelope';
import type { ReadinessEnvelope } from '../models/ReadinessEnvelope';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class PracticeService {
    /**
     * Check the Practice shadow process
     * @returns HealthEnvelope Process is alive
     * @throws ApiError
     */
    public static getQuizCraftHealth(): CancelablePromise<HealthEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/healthz',
            errors: {
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Check process and database readiness without exposing release metadata
     * @returns ReadinessEnvelope Process and database are ready
     * @throws ApiError
     */
    public static getQuizCraftReadiness(): CancelablePromise<ReadinessEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/readyz',
            errors: {
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Return authenticated migration, shadow, write-gate, and binary release evidence
     * @returns CutoverEvidenceEnvelope Cutover evidence is complete
     * @throws ApiError
     */
    public static getQuizCraftCutoverEvidence({
        runId,
        sourceHead,
    }: {
        runId: string,
        sourceHead: number,
    }): CancelablePromise<CutoverEvidenceEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/cutover-evidence',
            query: {
                'run_id': runId,
                'source_head': sourceHead,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * List published practice banks through the internal Portal catalog contract
     * @returns BankListEnvelope Published banks
     * @throws ApiError
     */
    public static listPracticeBanks(): CancelablePromise<BankListEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/banks',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Dedicated service request nonce was already used`,
                503: `PostgreSQL or a required service is unavailable`,
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
                401: `Missing or invalid actor credentials`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
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
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                404: `Resource or operation is unknown to this actor`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Read signed-in progress and wrong-question state
     * @returns LearningStateEnvelope Persistent account learning state
     * @throws ApiError
     */
    public static getLearningState(): CancelablePromise<LearningStateEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/learning-state',
            errors: {
                401: `Missing or invalid actor credentials`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Read one authenticated user's fact-derived Practice statistics for Portal
     * The trusted Portal actor header is bound as the sixth line of the signed service request and cannot be substituted in transit.
     * @returns PersonalPracticeStatsEnvelope Persistent stats and mastery rebuilt from immutable Practice attempts
     * @throws ApiError
     */
    public static getPersonalPracticeStats({
        xActorUserId,
    }: {
        /**
         * UUID of the Portal Session subject; it is the sixth line of the HMAC canonical request.
         */
        xActorUserId: string,
    }): CancelablePromise<PersonalPracticeStatsEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/stats',
            headers: {
                'X-Actor-User-Id': xActorUserId,
            },
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Dedicated service request nonce was already used`,
                503: `PostgreSQL or a required service is unavailable`,
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
                401: `Missing or invalid actor credentials`,
                404: `Resource or operation is unknown to this actor`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
}
