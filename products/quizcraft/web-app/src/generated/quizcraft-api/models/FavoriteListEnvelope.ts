/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { FavoriteQuestion } from './FavoriteQuestion';
import type { RequestID } from './RequestID';
export type FavoriteListEnvelope = {
    request_id: RequestID;
    data: Array<FavoriteQuestion>;
};
