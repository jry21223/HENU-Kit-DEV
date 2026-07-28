import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const quiz = fs.readFileSync(path.join(root, 'src/pages/Quiz.tsx'), 'utf8');
if (!quiz.includes('if (!QUIZCRAFT_GO_SHADOW_ENABLED) {') || !quiz.includes('await feedbackApi.submit({')) {
  throw new Error('the default FastAPI feedback path must remain available before QuizCraft V2 write cutover');
}
const v2Start = quiz.indexOf('const idempotencyKey = feedbackRequestKey || createQuizcraftIdempotencyKey();');
const v2End = quiz.indexOf('const retryFeedbackStatus = async () => {');
if (v2Start < 0 || v2End < v2Start || !quiz.slice(v2Start, v2End).includes('shadowFeedbackApi.submit') || quiz.slice(v2Start, v2End).includes('feedbackApi')) {
  throw new Error('the V2 feedback path must use only the generated QuizCraft client and never fall back to FastAPI');
}
const headerStart = quiz.indexOf('function QuizProgressHeader');
const feedbackButton = quiz.indexOf('aria-label="反馈本题"', headerStart);
if (headerStart < 0 || feedbackButton < headerStart || quiz.slice(headerStart, feedbackButton).includes('QUIZCRAFT_GO_SHADOW_ENABLED && (')) {
  throw new Error('the in-quiz FastAPI feedback entry must remain visible before QuizCraft V2 write cutover');
}

const feedback = fs.readFileSync(path.join(root, 'src/pages/Feedback.tsx'), 'utf8');
if (!feedback.includes('if (!QUIZCRAFT_GO_SHADOW_ENABLED) {') || !feedback.includes('return <LegacyFeedbackForm />;') || !feedback.includes('shadowFeedbackApi.listStatuses')) {
  throw new Error('the feedback route must keep the legacy form before cutover and the durable V2 history after cutover');
}

const routes = fs.readFileSync(path.join(root, 'src/main.tsx'), 'utf8');
const expectedRoute = 'path="feedback" element={<Feedback />}';
if (!routes.includes(expectedRoute)) {
  throw new Error('the legacy feedback route must remain reachable until the real QuizCraft write cutover');
}
