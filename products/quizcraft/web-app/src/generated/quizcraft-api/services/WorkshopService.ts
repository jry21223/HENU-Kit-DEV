/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { BankImportRequest } from '../models/BankImportRequest';
import type { BankListEnvelope } from '../models/BankListEnvelope';
import type { CreateBankVersion } from '../models/CreateBankVersion';
import type { CreateWorkshopBank } from '../models/CreateWorkshopBank';
import type { ImportReportEnvelope } from '../models/ImportReportEnvelope';
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { RollbackCommand } from '../models/RollbackCommand';
import type { VersionCommand } from '../models/VersionCommand';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class WorkshopService {
    /**
     * List workshop banks and active versions
     * @returns BankListEnvelope Workshop banks
     * @throws ApiError
     */
    public static listWorkshopBanks(): CancelablePromise<BankListEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/workshop/banks',
            errors: {
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
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
                422: `Import rejected without content writes`,
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
            },
        });
    }
}
