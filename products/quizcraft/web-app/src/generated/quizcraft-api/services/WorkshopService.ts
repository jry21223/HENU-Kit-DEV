/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { BankImportRequest } from '../models/BankImportRequest';
import type { BankListEnvelope } from '../models/BankListEnvelope';
import type { ConsoleSummaryEnvelope } from '../models/ConsoleSummaryEnvelope';
import type { CreateBankVersion } from '../models/CreateBankVersion';
import type { CreateWorkshopBank } from '../models/CreateWorkshopBank';
import type { ImportReportEnvelope } from '../models/ImportReportEnvelope';
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { RollbackCommand } from '../models/RollbackCommand';
import type { VersionCommand } from '../models/VersionCommand';
import type { WorkshopBankListEnvelope } from '../models/WorkshopBankListEnvelope';
import type { WorkshopFeedbackEnvelope } from '../models/WorkshopFeedbackEnvelope';
import type { WorkshopVersionDetailEnvelope } from '../models/WorkshopVersionDetailEnvelope';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class WorkshopService {
    /**
     * Start Platform Core OAuth with state and S256 PKCE
     * @returns void
     * @throws ApiError
     */
    public static startQuizCraftPlatformLogin({
        returnTo,
    }: {
        returnTo?: string,
    }): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/auth/login',
            query: {
                'return_to': returnTo,
            },
            errors: {
                302: `Redirect to the Platform Core authorization endpoint and set an encrypted short-lived state cookie.`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Exchange a single-use Platform Core code server-side and establish an encrypted HttpOnly session
     * @returns void
     * @throws ApiError
     */
    public static finishQuizCraftPlatformLogin({
        code,
        state,
    }: {
        code: string,
        state: string,
    }): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/auth/callback',
            query: {
                'code': code,
                'state': state,
            },
            errors: {
                302: `Secure local QuizCraft session established and redirected to the validated same-origin return path.`,
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Return bounded QuizCraft facts for Console Overview
     * @returns ConsoleSummaryEnvelope QuizCraft summary
     * @throws ApiError
     */
    public static getQuizCraftConsoleSummary(): CancelablePromise<ConsoleSummaryEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/console-summary',
            errors: {
                401: `Missing or invalid actor credentials`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * List workshop banks and active versions
     * @returns BankListEnvelope Published Workshop banks
     * @throws ApiError
     */
    public static listWorkshopBanks(): CancelablePromise<BankListEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/workshop/banks',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Create a stable workshop bank
     * @returns OperationEnvelope Bank write result
     * @throws ApiError
     */
    public static createWorkshopBank({
        idempotencyKey,
        requestBody,
    }: {
        idempotencyKey: string,
        requestBody: CreateWorkshopBank,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/workshop/banks',
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * List Workshop banks with draft, validation, and publication lifecycle state
     * @returns WorkshopBankListEnvelope Workshop lifecycle catalog
     * @throws ApiError
     */
    public static listWorkshopCatalog(): CancelablePromise<WorkshopBankListEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/workshop/catalog',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Create an immutable draft bank version
     * @returns OperationEnvelope Version write result
     * @throws ApiError
     */
    public static createWorkshopBankVersion({
        bankId,
        idempotencyKey,
        requestBody,
    }: {
        bankId: string,
        idempotencyKey: string,
        requestBody: CreateBankVersion,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/workshop/banks/{bank_id}/versions',
            path: {
                'bank_id': bankId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Import and validate an immutable workshop bank version
     * @returns ImportReportEnvelope Import report for an accepted immutable version
     * @throws ApiError
     */
    public static importWorkshopBank({
        bankId,
        idempotencyKey,
        requestBody,
    }: {
        bankId: string,
        idempotencyKey: string,
        requestBody: BankImportRequest,
    }): CancelablePromise<ImportReportEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/workshop/banks/{bank_id}/imports',
            path: {
                'bank_id': bankId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Idempotency payload or optimistic version conflict`,
                422: `Import rejected without content writes`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Read one immutable version with full questions for human validation
     * @returns WorkshopVersionDetailEnvelope Full authorized validation view
     * @throws ApiError
     */
    public static getWorkshopBankVersion({
        bankId,
        bankVersionId,
    }: {
        bankId: string,
        bankVersionId: string,
    }): CancelablePromise<WorkshopVersionDetailEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}',
            path: {
                'bank_id': bankId,
                'bank_version_id': bankVersionId,
            },
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                404: `Resource or operation is unknown to this actor`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Record human validation of a draft before publication
     * @returns OperationEnvelope Validation write result
     * @throws ApiError
     */
    public static validateWorkshopBankVersion({
        bankId,
        bankVersionId,
        idempotencyKey,
        requestBody,
    }: {
        bankId: string,
        bankVersionId: string,
        idempotencyKey: string,
        requestBody: VersionCommand,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}/validate',
            path: {
                'bank_id': bankId,
                'bank_version_id': bankVersionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Publish a human-validated immutable version
     * @returns OperationEnvelope Publish write result
     * @throws ApiError
     */
    public static publishWorkshopBankVersion({
        bankId,
        bankVersionId,
        idempotencyKey,
        requestBody,
    }: {
        bankId: string,
        bankVersionId: string,
        idempotencyKey: string,
        requestBody: VersionCommand,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}/publish',
            path: {
                'bank_id': bankId,
                'bank_version_id': bankVersionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Unpublish a bank version without deleting its facts
     * @returns OperationEnvelope Unpublish write result
     * @throws ApiError
     */
    public static unpublishWorkshopBankVersion({
        bankId,
        bankVersionId,
        idempotencyKey,
        requestBody,
    }: {
        bankId: string,
        bankVersionId: string,
        idempotencyKey: string,
        requestBody: VersionCommand,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}/unpublish',
            path: {
                'bank_id': bankId,
                'bank_version_id': bankVersionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Roll back by republishing a prior immutable version
     * @returns OperationEnvelope Rollback write result
     * @throws ApiError
     */
    public static rollbackWorkshopBank({
        bankId,
        idempotencyKey,
        requestBody,
    }: {
        bankId: string,
        idempotencyKey: string,
        requestBody: RollbackCommand,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/workshop/banks/{bank_id}/rollback',
            path: {
                'bank_id': bankId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Read full product-owned feedback from an Operations Inbox deep link
     * @returns WorkshopFeedbackEnvelope Full feedback remains owned by QuizCraft
     * @throws ApiError
     */
    public static getWorkshopFeedback({
        feedbackId,
    }: {
        feedbackId: string,
    }): CancelablePromise<WorkshopFeedbackEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/workshop/feedback/{feedback_id}',
            path: {
                'feedback_id': feedbackId,
            },
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                404: `Resource or operation is unknown to this actor`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
}
