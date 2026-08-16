import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const PROFILE = {
  user_id: "11111111-1111-4111-8111-111111111111",
  target_roles: "后端开发",
  tech_stack: "go,postgres",
  locations: "郑州",
  job_type: "daily_intern" as const,
  graduation_year: 2027,
  resume_text: "校内项目经历",
  email_notification_enabled: true,
  updated_at: "2026-08-15T00:00:00Z",
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("getCareerProfile", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("reads the signed-in user's profile without caching", async () => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({ profile: PROFILE, request_id: "req_career_profile" })
    );
    vi.stubGlobal("fetch", fetch);

    const { getCareerProfile } = await import("./client");
    const response = await getCareerProfile();

    expect(response.profile.job_type).toBe("daily_intern");
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/career/profile");
    expect(init.cache).toBe("no-store");
  });

  it("surfaces the lifetime gate as a PortalForbiddenError instead of a local profile", async () => {
    const fetch = vi.fn().mockImplementation(() =>
      Promise.resolve(
        jsonResponse(
          { error: "lifetime_required", message: "求职雷达需要 Lifetime VIP 会员", request_id: "req_career_gate" },
          403
        )
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { getCareerProfile, PortalForbiddenError } = await import("./client");
    await expect(getCareerProfile()).rejects.toBeInstanceOf(PortalForbiddenError);
    await expect(getCareerProfile()).rejects.toMatchObject({
      status: 403,
      errorCode: "lifetime_required",
    });
  });
});

describe("updateCareerProfile", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("replaces the whole profile with a PUT carrying every editable field", async () => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({ profile: PROFILE, request_id: "req_career_profile_put" })
    );
    vi.stubGlobal("fetch", fetch);

    const { updateCareerProfile } = await import("./client");
    const response = await updateCareerProfile({
      target_roles: PROFILE.target_roles,
      tech_stack: PROFILE.tech_stack,
      locations: PROFILE.locations,
      job_type: PROFILE.job_type,
      graduation_year: PROFILE.graduation_year,
      resume_text: PROFILE.resume_text,
      email_notification_enabled: PROFILE.email_notification_enabled,
    });

    expect(response.profile.updated_at).toBe(PROFILE.updated_at);
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/career/profile");
    expect(init.method).toBe("PUT");
    expect(init.cache).toBe("no-store");
    expect(JSON.parse(String(init.body))).toEqual({
      target_roles: "后端开发",
      tech_stack: "go,postgres",
      locations: "郑州",
      job_type: "daily_intern",
      graduation_year: 2027,
      resume_text: "校内项目经历",
      email_notification_enabled: true,
    });
  });
});

describe("createCareerResumeExtraction", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("uploads the resume as multipart and returns the queued extraction", async () => {
    const extraction = {
      id: "11111111-1111-4111-8111-111111111111",
      status: "queued" as const,
      user_id: PROFILE.user_id,
      file_name: "resume.txt",
      created_at: "2026-08-16T00:00:00Z",
    };
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({ extraction, request_id: "req_career_extract" })
    );
    vi.stubGlobal("fetch", fetch);

    const { createCareerResumeExtraction } = await import("./client");
    const file = new File(["简历内容"], "resume.txt", { type: "text/plain" });
    const response = await createCareerResumeExtraction(file);

    expect(response.extraction.status).toBe("queued");
    expect(response.extraction.file_name).toBe("resume.txt");
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/v1/career/profile/extractions");
    expect(init.method).toBe("POST");
    expect(init.cache).toBe("no-store");
    expect(init.body).toBeInstanceOf(FormData);
    // 浏览器负责 multipart boundary，客户端绝不能手写 Content-Type。
    const headers = (init.headers ?? {}) as Record<string, string>;
    expect(headers["Content-Type"]).toBeUndefined();
    const sent = init.body as FormData;
    expect(sent.get("file")).toBe(file);
  });

  it("surfaces the rate limit as an error instead of a fake queued job", async () => {
    const fetch = vi.fn().mockImplementation(() =>
      Promise.resolve(
        jsonResponse(
          { error: "EXTRACT_RATE_LIMITED", message: "resume extraction rate limit exceeded", request_id: "req_career_extract" },
          429
        )
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createCareerResumeExtraction } = await import("./client");
    const file = new File(["简历内容"], "resume.txt", { type: "text/plain" });
    await expect(createCareerResumeExtraction(file)).rejects.toMatchObject({
      status: 429,
      errorCode: "EXTRACT_RATE_LIMITED",
    });
  });
});

describe("getCareerResumeExtraction", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("reads a completed extraction with the extracted profile draft", async () => {
    const extraction = {
      id: "11111111-1111-4111-8111-111111111111",
      status: "completed" as const,
      user_id: PROFILE.user_id,
      file_name: "resume.txt",
      extracted: {
        target_roles: "后端开发",
        tech_stack: "go,postgres",
        locations: "郑州",
        job_type: "daily_intern",
        graduation_year: 2027,
        resume_text: "校内项目经历",
      },
      created_at: "2026-08-16T00:00:00Z",
    };
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({ extraction, request_id: "req_career_extract" })
    );
    vi.stubGlobal("fetch", fetch);

    const { getCareerResumeExtraction } = await import("./client");
    const response = await getCareerResumeExtraction(extraction.id);

    expect(response.extraction.status).toBe("completed");
    expect(response.extraction.extracted?.target_roles).toBe("后端开发");
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe(`/api/v1/career/profile/extractions/${extraction.id}`);
    expect(init.cache).toBe("no-store");
  });

  it("rejects an empty extraction id without a network call", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    const { getCareerResumeExtraction } = await import("./client");
    await expect(getCareerResumeExtraction("  ")).rejects.toMatchObject({
      code: "PORTAL_INVALID_CAREER_EXTRACTION_ID",
    });
    expect(fetch).not.toHaveBeenCalled();
  });
});
