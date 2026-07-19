/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { QuestionType } from './QuestionType';
export type ImportedQuestionInput = {
    source_question_id: string;
    type: QuestionType;
    chapter_id: string;
    chapter?: string;
    content: string;
    options?: Array<string>;
    answer: any;
    analysis?: string;
};
