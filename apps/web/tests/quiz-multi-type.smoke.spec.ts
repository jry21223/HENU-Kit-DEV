import { expect, test, type APIRequestContext, type Locator, type Page } from "@playwright/test";

test.use({ channel: "chrome" });

type Envelope<T> = {
  code: number;
  message: string;
  data: T;
};

type Course = {
  id: string;
  name: string;
  slug: string;
  status: string;
};

type QuizOption = {
  id: string;
  label: string;
  content: string;
  sortOrder: number;
};

type QuizQuestion = {
  id: string;
  courseId: string;
  type: string;
  stem: string;
  difficulty: number;
  options?: QuizOption[];
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  apiBaseURL: string;
  courseID: string;
  multipleChoiceQuestionID: string;
  multipleChoiceAnswer: string;
  freeTextQuestionID: string;
  freeTextAnswer: string;
};

type SelectedQuestion = {
  course: Course;
  question: QuizQuestion;
};

const cfg = readConfig();

test.describe("quiz multi-type browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_QUIZ_MULTI_TYPE_SMOKE=1 plus E2E_* URLs to run the quiz multi-type smoke.");

  test("multiple-choice question can select an answer set through the browser UI", async ({ page, request }) => {
    const selected = await selectQuestion(request, cfg, "multiple_choice", cfg.multipleChoiceQuestionID);
    const labels = splitAnswerSet(await resolveCorrectMultipleChoiceAnswer(request, cfg.apiBaseURL, selected.question, cfg.multipleChoiceAnswer));

    await openQuestionCard(page, cfg.webBaseURL, selected);
    const card = page.locator(`[data-testid="practice-card"][data-question-id="${selected.question.id}"]`);
    for (const label of labels) {
      await card.getByTestId(`quiz-option-${label}`).click();
      await expect(card.getByTestId(`quiz-option-${label}`)).toHaveAttribute("aria-pressed", "true");
    }

    const submitPayload = await submitVisibleCard(page, selected.question.id, card);
    expect(submitPayload.isCorrect, "Smoke must submit the resolved correct multi-choice answer.").toBe(true);
    await expect(card.getByTestId("quiz-result")).toBeVisible();
  });

  test("fill-blank question can submit a free-text answer through the browser UI", async ({ page, request }) => {
    const selected = await selectQuestion(request, cfg, "fill_blank", cfg.freeTextQuestionID);
    const answer = await resolveCorrectFreeTextAnswer(request, cfg.apiBaseURL, selected.question, cfg.freeTextAnswer);

    await openQuestionCard(page, cfg.webBaseURL, selected);
    const card = page.locator(`[data-testid="practice-card"][data-question-id="${selected.question.id}"]`);
    await card.getByTestId("quiz-free-answer").fill(answer);

    const submitPayload = await submitVisibleCard(page, selected.question.id, card);
    expect(submitPayload.isCorrect, "Smoke must submit the resolved correct fill-blank answer.").toBe(true);
    await expect(card.getByTestId("quiz-result")).toBeVisible();
  });
});

async function openQuestionCard(page: Page, webBaseURL: string, selected: SelectedQuestion) {
  await page.goto(joinURL(webBaseURL, `/courses/${selected.course.id}/quiz`), { waitUntil: "networkidle" });
  const card = page.locator(`[data-testid="practice-card"][data-question-id="${selected.question.id}"]`);
  await expect(card).toBeVisible();
  await expect(card.getByText(selected.question.stem).first()).toBeVisible();
}

async function submitVisibleCard(page: Page, questionID: string, card: Locator) {
  const [submitResponse] = await Promise.all([
    page.waitForResponse((response) => response.url().includes(`/questions/${questionID}/submit`) && response.request().method() === "POST"),
    card.getByTestId("quiz-submit").click(),
  ]);
  expect(submitResponse.status()).toBe(200);
  const submitEnvelope = (await submitResponse.json()) as Envelope<{ isCorrect: boolean; score: number }>;
  expect(submitEnvelope.code).toBe(0);
  return submitEnvelope.data;
}

async function selectQuestion(
  request: APIRequestContext,
  config: SmokeConfig,
  questionType: "multiple_choice" | "fill_blank",
  questionID: string,
): Promise<SelectedQuestion> {
  if (questionID) {
    const detail = await apiGET<{ question: QuizQuestion }>(request, config.apiBaseURL, `/questions/${questionID}`);
    if (detail.question.type !== questionType) {
      throw new Error(`Question ${questionID} is ${detail.question.type}; expected ${questionType}.`);
    }
    return {
      course: await courseForQuestion(request, config.apiBaseURL, detail.question.courseId),
      question: detail.question,
    };
  }

  const courses = config.courseID
    ? [await courseForQuestion(request, config.apiBaseURL, config.courseID)]
    : (await apiGET<{ courses: Course[] }>(request, config.apiBaseURL, "/courses")).courses;

  for (const course of courses) {
    const result = await apiGET<{ questions: QuizQuestion[] }>(request, config.apiBaseURL, `/courses/${course.id}/questions`);
    const question = result.questions.find((item) => item.type === questionType);
    if (question) {
      return { course, question };
    }
  }
  throw new Error(`No published ${questionType} question was found. Set the matching E2E_QUIZ_*_QUESTION_ID override.`);
}

async function courseForQuestion(request: APIRequestContext, apiBaseURL: string, courseID: string): Promise<Course> {
  const result = await apiGET<{ course: Course }>(request, apiBaseURL, `/courses/${courseID}`);
  return result.course;
}

async function resolveCorrectMultipleChoiceAnswer(
  request: APIRequestContext,
  apiBaseURL: string,
  question: QuizQuestion,
  configuredAnswer: string,
) {
  if (configuredAnswer) {
    await expectCorrectAnswer(request, apiBaseURL, question.id, configuredAnswer);
    return configuredAnswer;
  }

  const labels = (question.options ?? []).map((option) => option.label);
  if (!labels.length) {
    throw new Error(`Question ${question.id} has no options.`);
  }

  for (const answer of answerCombinations(labels.slice(0, 8))) {
    const correct = await isCorrectAnswer(request, apiBaseURL, question.id, answer);
    if (correct) {
      return answer;
    }
  }
  throw new Error(`No correct multi-choice answer set could be resolved for question ${question.id}.`);
}

async function resolveCorrectFreeTextAnswer(
  request: APIRequestContext,
  apiBaseURL: string,
  question: QuizQuestion,
  configuredAnswer: string,
) {
  const candidates = configuredAnswer ? [configuredAnswer] : ["P(A)P(B)", "P(A) P(B)", "golang", "Go Lang", "answer"];
  for (const answer of candidates) {
    const correct = await isCorrectAnswer(request, apiBaseURL, question.id, answer);
    if (correct) {
      return answer;
    }
  }
  throw new Error(`No correct free-text answer candidate worked for question ${question.id}; set E2E_QUIZ_FREE_TEXT_ANSWER.`);
}

async function expectCorrectAnswer(request: APIRequestContext, apiBaseURL: string, questionID: string, answer: string) {
  const correct = await isCorrectAnswer(request, apiBaseURL, questionID, answer);
  if (!correct) {
    throw new Error(`Configured answer "${answer}" is not correct for question ${questionID}.`);
  }
}

async function isCorrectAnswer(request: APIRequestContext, apiBaseURL: string, questionID: string, answer: string) {
  const response = await request.post(joinURL(apiBaseURL, `/questions/${questionID}/submit`), {
    data: { answer },
  });
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<{ isCorrect: boolean }>;
  expect(payload.code).toBe(0);
  return payload.data.isCorrect;
}

async function apiGET<T>(request: APIRequestContext, apiBaseURL: string, path: string): Promise<T> {
  const response = await request.get(joinURL(apiBaseURL, path));
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<T>;
  expect(payload.code).toBe(0);
  return payload.data;
}

function answerCombinations(labels: string[]) {
  const combinations: string[] = [];
  const total = 1 << labels.length;
  for (let mask = 1; mask < total; mask += 1) {
    const selected = labels.filter((_, index) => (mask & (1 << index)) !== 0);
    combinations.push(selected.join(","));
  }
  return combinations;
}

function splitAnswerSet(value: string) {
  return value
    .split(/[,\s;]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function readConfig(): SmokeConfig {
  return {
    enabled: process.env.E2E_QUIZ_MULTI_TYPE_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    courseID: process.env.E2E_QUIZ_MULTI_TYPE_COURSE_ID ?? "",
    multipleChoiceQuestionID: process.env.E2E_QUIZ_MULTI_CHOICE_QUESTION_ID ?? "",
    multipleChoiceAnswer: process.env.E2E_QUIZ_MULTI_CHOICE_ANSWER ?? "",
    freeTextQuestionID: process.env.E2E_QUIZ_FREE_TEXT_QUESTION_ID ?? "",
    freeTextAnswer: process.env.E2E_QUIZ_FREE_TEXT_ANSWER ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}
