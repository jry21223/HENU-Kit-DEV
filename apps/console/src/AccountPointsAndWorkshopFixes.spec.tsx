import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { App as AntApp, ConfigProvider } from "antd";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import App from "./App";
import { consoleTheme } from "./theme";

const session = {
  user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
  access_context: {
    permissions: ["account.points.adjust", "quizcraft.workshop.read", "quizcraft.workshop.write"],
    scopes: [
      { kind: "product", product_code: "account-portfolio" },
      { kind: "product", product_code: "quizcraft" },
    ],
    verified_at: "2026-11-22T04:00:00Z",
  },
  expires_at: "2026-11-22T06:00:00Z",
};

function jsonResponse(data: unknown, status = 200) {
  return Response.json({ data, request_id: "req_regression" }, { status });
}

function renderAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ConfigProvider theme={{ ...consoleTheme, token: { ...consoleTheme.token, motion: false } }}>
      <AntApp>
        <QueryClientProvider client={client}>
          <MemoryRouter initialEntries={[path]}>
            <App />
          </MemoryRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.sessionStorage.clear();
});

it("discards a pending account lookup when the operator edits the email", async () => {
  const first = {
    id: "44444444-4444-4444-4444-444444444444",
    display_name: "李同学",
    email: "li@henu.edu.cn",
    status: "active",
  };
  const second = {
    id: "55555555-5555-5555-5555-555555555555",
    display_name: "王同学",
    email: "wang@henu.edu.cn",
    status: "active",
  };
  let finishFirstLookup!: (response: Response) => void;
  const firstLookup = new Promise<Response>((resolve) => {
    finishFirstLookup = resolve;
  });
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input);
    if (path === "/api/v1/session") return jsonResponse(session);
    if (path === "/api/v1/operations/account-lookups") {
      const { email } = JSON.parse(String(init?.body));
      if (email === first.email) return firstLookup;
      if (email === second.email) return jsonResponse({ account: second });
    }
    if (path.startsWith("/api/v1/account/points/")) return jsonResponse({ balance: 0, entries: [] });
    return jsonResponse({}, 404);
  });

  renderAt("/account/points");
  const input = await screen.findByPlaceholderText("student@henu.edu.cn");
  const lookup = screen.getByRole("button", { name: /查\s*找$/ });
  fireEvent.change(input, { target: { value: first.email } });
  fireEvent.click(lookup);
  expect(fetchMock).toHaveBeenCalledWith(
    "/api/v1/operations/account-lookups",
    expect.objectContaining({ body: JSON.stringify({ email: first.email }) }),
  );

  fireEvent.change(input, { target: { value: second.email } });
  await act(async () => {
    finishFirstLookup(jsonResponse({ account: first }));
    await firstLookup;
  });

  expect(screen.queryByText(first.email)).toBeNull();
  expect((screen.getByRole("spinbutton") as HTMLInputElement).disabled).toBe(true);

  fireEvent.click(lookup);
  expect((await screen.findAllByText(second.display_name)).length).toBeGreaterThan(0);
  expect((screen.getByRole("spinbutton") as HTMLInputElement).disabled).toBe(false);
}, 20_000);

it("imports QuizCraft chapters and analyses using canonical fields and compatible aliases", async () => {
  vi.stubGlobal("crypto", {
    randomUUID: crypto.randomUUID.bind(crypto),
    subtle: { digest: async () => new Uint8Array(32).buffer },
  });
  const bank = { id: "22222222-2222-2222-2222-222222222222", name: "计算机网络题库", version: 2 };
  const requests: unknown[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input);
    if (path === "/api/v1/session") return jsonResponse(session);
    if (path === "/api/v1/quizcraft/workshop/catalog") return jsonResponse({ banks: [bank] });
    if (path === `/api/v1/quizcraft/workshop/banks/${bank.id}/imports`) {
      requests.push(JSON.parse(String(init?.body)));
      return jsonResponse({ bank_id: bank.id });
    }
    return jsonResponse({}, 404);
  });

  renderAt("/practice");
  fireEvent.click(await screen.findByRole("button", { name: /导入新版本/ }));
  const fields = { type: "judge", chapter_id: "network", content: "TCP 提供可靠传输。", answer: true };
  const questions = [
    { ...fields, source_question_id: "canonical", chapter: "传输层", analysis: "TCP 会重传丢失的数据。" },
    { ...fields, source_question_id: "alias", chapter_name: "应用层", explanation: "应用层协议定义消息格式。" },
    { ...fields, source_question_id: "mixed", chapter: "网络层", explanation: "IP 提供寻址。" },
    {
      ...fields,
      source_question_id: "both",
      chapter: "链路层",
      chapter_name: "旧章节",
      analysis: "以太网使用帧。",
      explanation: "旧解析",
    },
    {
      ...fields,
      source_question_id: "blank-canonical",
      chapter: "  ",
      chapter_name: "物理层",
      analysis: null,
      explanation: "物理层传送比特。",
    },
    { ...fields, source_question_id: "missing" },
  ];
  fireEvent.change(screen.getByPlaceholderText(/粘贴题目 JSON 数组/), {
    target: { value: JSON.stringify({ questions }) },
  });
  fireEvent.click(screen.getByRole("button", { name: "校验并导入草稿" }));

  await screen.findByText("新版本草稿导入并校验通过。");
  expect(requests).toEqual([
    {
      expected_version: 2,
      source_sha256: expect.stringMatching(/^[a-f0-9]{64}$/),
      questions: [
        { ...fields, source_question_id: "canonical", chapter: "传输层", analysis: "TCP 会重传丢失的数据。" },
        { ...fields, source_question_id: "alias", chapter: "应用层", analysis: "应用层协议定义消息格式。" },
        { ...fields, source_question_id: "mixed", chapter: "网络层", analysis: "IP 提供寻址。" },
        { ...fields, source_question_id: "both", chapter: "链路层", analysis: "以太网使用帧。" },
        { ...fields, source_question_id: "blank-canonical", chapter: "物理层", analysis: "物理层传送比特。" },
        { ...fields, source_question_id: "missing", chapter: "默认章节" },
      ],
    },
  ]);
}, 20_000);
