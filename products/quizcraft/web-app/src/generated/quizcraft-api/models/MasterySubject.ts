/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type MasterySubject = {
    bank_id: string;
    label: string;
    /**
     * Correct stable questions divided by the active bank question count.
     */
    value: number;
    /**
     * Current active stable questions in this bank.
     */
    total_questions: number;
    /**
     * Active stable questions answered correctly at least once by this user.
     */
    correct_questions: number;
};
