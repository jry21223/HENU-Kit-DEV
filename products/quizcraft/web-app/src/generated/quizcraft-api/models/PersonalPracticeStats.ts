/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { MasterySubject } from './MasterySubject';
export type PersonalPracticeStats = {
    /**
     * Count of immutable scored attempts owned by this user.
     */
    total_answers: number;
    /**
     * Correct immutable scored attempts owned by this user.
     */
    correct_answers: number;
    /**
     * Rounded correct_answers divided by total_answers.
     */
    accuracy: number;
    /**
     * Consecutive Asia/Shanghai calendar days ending today or yesterday with at least one scored attempt.
     */
    streak_days: number;
    mastery: Array<MasterySubject>;
};
