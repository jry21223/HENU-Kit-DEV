/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type ChoicePracticeQuestion = {
    question_id: string;
    question_version_id: string;
    type: 'single' | 'multi';
    chapter_id: string;
    chapter: string;
    content: string;
    options: Array<string>;
};
