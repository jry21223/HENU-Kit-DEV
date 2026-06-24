import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

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

type WrongQuestion = {
  id: string;
  questionId: string;
  courseId: string;
  wrongCount: number;
};

type SmokeConfig = {
  enabled: boolean;
  webBaseURL: string;
  apiBaseURL: string;
  studentEmail: string;
  studentCode: string;
  courseID: string;
  questionID: string;
  wrongAnswer: string;
};

type SelectedQuestion = {
  course: Course;
  question: QuizQuestion;
  wrongAnswer: string;
};

const cfg = readConfig();

test.describe("quiz wrong-question browser smoke", () => {
  test.skip(!cfg.enabled, "Set E2E_QUIZ_SMOKE=1 plus E2E_* URLs/account to run the quiz wrong-question smoke.");

  test("authenticated wrong answer is saved and visible in the wrong-question book", async ({ browser }) => {
    const context = await browser.newContext();
    try {
      const selected = await selectQuestion(context.request, cfg);
      const page = await context.newPage();

      await loginStudent(page, cfg);
      const before = await wrongCountForQuestion(context.request, cfg.apiBaseURL, selected.question.id);

      await page.goto(joinURL(cfg.webBaseURL, `/courses/${selected.course.id}/quiz`), { waitUntil: "networkidle" });
      const card = page.locator(`[data-testid="practice-card"][data-question-id="${selected.question.id}"]`);
      await expect(card).toBeVisible();
      await expect(card.getByText(selected.question.stem).first()).toBeVisible();
      await card.getByTestId(`quiz-option-${selected.wrongAnswer}`).click();

      const [submitResponse] = await Promise.all([
        page.waitForResponse(
          (response) => response.url().includes(`/questions/${selected.question.id}/submit`) && response.request().method() === "POST",
        ),
        card.getByTestId("quiz-submit").click(),
      ]);
      expect(submitResponse.status()).toBe(200);
      const submitPayload = (await submitResponse.json()) as Envelope<{ isCorrect: boolean }>;
      expect(submitPayload.code).toBe(0);
      expect(submitPayload.data.isCorrect, "Smoke must submit an intentionally wrong answer.").toBe(false);

      const after = await wrongCountForQuestion(context.request, cfg.apiBaseURL, selected.question.id);
      expect(after, "Wrong-question count should increase for the authenticated student.").toBeGreaterThan(before);

      await page.goto(joinURL(cfg.webBaseURL, "/me/wrong-questions"), { waitUntil: "networkidle" });
      await expect(page.getByText(selected.question.stem).first()).toBeVisible();
    } finally {
      await context.close();
    }
  });
});

async function selectQuestion(request: APIRequestContext, config: SmokeConfig): Promise<SelectedQuestion> {
  if (config.questionID) {
    const detail = await apiGET<{ question: QuizQuestion }>(request, config.apiBaseURL, `/questions/${config.questionID}`);
    if (!detail.question.options?.length) {
      throw new Error(`Question ${config.questionID} has no clickable options; set E2E_QUIZ_QUESTION_ID to a choice question.`);
    }
    const course = config.courseID
      ? { id: config.courseID, name: config.courseID, slug: config.courseID, status: "published" }
      : await courseForQuestion(request, config.apiBaseURL, detail.question.courseId);
    const wrongAnswer = await resolveWrongAnswer(request, config.apiBaseURL, detail.question, config.wrongAnswer);
    return { course, question: detail.question, wrongAnswer };
  }

  const courses = config.courseID
    ? [await courseForQuestion(request, config.apiBaseURL, config.courseID)]
    : (await apiGET<{ courses: Course[] }>(request, config.apiBaseURL, "/courses")).courses;

  for (const course of courses) {
    const result = await apiGET<{ questions: QuizQuestion[] }>(request, config.apiBaseURL, `/courses/${course.id}/questions`);
    for (const question of result.questions) {
      if ((question.options?.length ?? 0) < 2) {
        continue;
      }
      const wrongAnswer = await resolveWrongAnswer(request, config.apiBaseURL, question, config.wrongAnswer);
      return { course, question, wrongAnswer };
    }
  }
  throw new Error("No published choice question with an intentionally wrong option was found.");
}

async function courseForQuestion(request: APIRequestContext, apiBaseURL: string, courseID: string): Promise<Course> {
  const result = await apiGET<{ course: Course }>(request, apiBaseURL, `/courses/${courseID}`);
  return result.course;
}

async function resolveWrongAnswer(request: APIRequestContext, apiBaseURL: string, question: QuizQuestion, configuredAnswer: string) {
  if (configuredAnswer) {
    return configuredAnswer;
  }
  for (const option of question.options ?? []) {
    const response = await request.post(joinURL(apiBaseURL, `/questions/${question.id}/submit`), {
      data: { answer: option.label },
    });
    expect(response.status()).toBe(200);
    const payload = (await response.json()) as Envelope<{ isCorrect: boolean }>;
    expect(payload.code).toBe(0);
    if (!payload.data.isCorrect) {
      return option.label;
    }
  }
  throw new Error(`No wrong option could be found for question ${question.id}.`);
}

async function wrongCountForQuestion(request: APIRequestContext, apiBaseURL: string, questionID: string) {
  const result = await apiGET<{ wrongQuestions: WrongQuestion[] }>(request, apiBaseURL, "/me/wrong-questions");
  return result.wrongQuestions.find((item) => item.questionId === questionID)?.wrongCount ?? 0;
}

async function loginStudent(page: Page, config: SmokeConfig) {
  await page.goto(joinURL(config.webBaseURL, "/login"), { waitUntil: "networkidle" });
  await page.locator('input[type="email"]').first().fill(config.studentEmail);
  await page.locator('button[type="button"]').first().click();
  const codeInput = page.getByPlaceholder("123456").first();
  if (config.studentCode) {
    await codeInput.fill(config.studentCode);
  } else {
    await expect(codeInput, "No E2E_STUDENT_CODE was provided and the API did not return a development code.").not.toHaveValue("");
  }
  await page.locator('button[type="submit"]').first().click();
  await expect(page.getByText(config.studentEmail)).toBeVisible();
}

async function apiGET<T>(request: APIRequestContext, apiBaseURL: string, path: string): Promise<T> {
  const response = await request.get(joinURL(apiBaseURL, path));
  expect(response.status()).toBe(200);
  const payload = (await response.json()) as Envelope<T>;
  expect(payload.code).toBe(0);
  return payload.data;
}

function readConfig(): SmokeConfig {
  return {
    enabled: process.env.E2E_QUIZ_SMOKE === "1",
    webBaseURL: cleanBaseURL(process.env.E2E_WEB_BASE_URL ?? "http://127.0.0.1:3000"),
    apiBaseURL: cleanBaseURL(process.env.E2E_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1"),
    studentEmail: process.env.E2E_STUDENT_EMAIL ?? "smoke-quiz@stu.henu.edu.cn",
    studentCode: process.env.E2E_STUDENT_CODE ?? "",
    courseID: process.env.E2E_QUIZ_COURSE_ID ?? "",
    questionID: process.env.E2E_QUIZ_QUESTION_ID ?? "",
    wrongAnswer: process.env.E2E_QUIZ_WRONG_ANSWER ?? "",
  };
}

function cleanBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function joinURL(baseURL: string, path: string) {
  return `${cleanBaseURL(baseURL)}/${path.replace(/^\/+/, "")}`;
}
