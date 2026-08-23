"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";

import { useAccountConsoleUnauthorizedHandler } from "@/components/account/account-console-session";
import { useReveal } from "@/components/account/use-reveal";
import {
  createCareerResumeExtraction,
  formatPortalError,
  getCareerProfile,
  getCareerResumeExtraction,
} from "@/lib/api/client";
import type {
  CareerJobType,
  CareerProfile,
  CareerProfileInput,
  CareerResumeExtraction,
} from "@/lib/api/types";
import {
  isCareerLifetimeRequiredError,
  requestCareerProfileUpdate,
} from "@/lib/career/gateway";
import {
  createExtractionRunner,
  extractionCreateFailedMessage,
  extractionFailedMessage,
  extractionStatusLabel,
} from "@/lib/career/career-extraction-state";

const FIELD_LIMITS = {
  target_roles: 500,
  tech_stack: 1000,
  locations: 500,
  resume_text: 4000,
} as const;

const JOB_TYPE_OPTIONS: Array<{ value: CareerJobType; label: string }> = [
  { value: "", label: "不限" },
  { value: "daily_intern", label: "日常实习" },
  { value: "summer_intern", label: "暑期实习" },
  { value: "campus_recruit", label: "校招" },
];

const RESUME_MAX_BYTES = 10 * 1024 * 1024;
const RESUME_EXTENSIONS = ["pdf", "docx", "txt"] as const;

type ProfileState =
  | { kind: "loading" }
  | { kind: "locked" }
  | { kind: "error"; message: string }
  | { kind: "ready"; profile: CareerProfile };

/** The editor always holds every field; the optional input shape is for PUT. */
type ProfileForm = {
  target_roles: string;
  tech_stack: string;
  locations: string;
  job_type: CareerJobType;
  graduation_year: number | null;
  resume_text: string;
  email_notification_enabled: boolean;
};

type UploadState =
  | { kind: "idle" }
  | { kind: "uploading" }
  | { kind: "active"; extraction: CareerResumeExtraction; label: string }
  | { kind: "done"; extraction: CareerResumeExtraction }
  | { kind: "error"; message: string };

function emptyInput(profile: CareerProfile): ProfileForm {
  return {
    target_roles: profile.target_roles ?? "",
    tech_stack: profile.tech_stack ?? "",
    locations: profile.locations ?? "",
    job_type: profile.job_type ?? "",
    graduation_year: profile.graduation_year ?? null,
    resume_text: profile.resume_text ?? "",
    email_notification_enabled: profile.email_notification_enabled ?? true,
  };
}

function graduationYearError(value: string): string {
  const trimmed = value.trim();
  if (trimmed === "") return "";
  if (!/^\d{4}$/.test(trimmed)) return "毕业年份需为 4 位数字，例如 2027。";
  const year = Number(trimmed);
  if (year < 1900 || year > 2200) return "毕业年份需在 1900 到 2200 之间。";
  return "";
}

export default function CareerProfilePage() {
  const [state, setState] = useState<ProfileState>({ kind: "loading" });
  const [form, setForm] = useState<ProfileForm | null>(null);
  const [yearValue, setYearValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [upload, setUpload] = useState<UploadState>({ kind: "idle" });
  const requestVersion = useRef(0);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const handleUnauthorized = useAccountConsoleUnauthorizedHandler();
  useReveal();

  const applyProfile = useCallback((profile: CareerProfile) => {
    setForm(emptyInput(profile));
    setYearValue(profile.graduation_year == null ? "" : String(profile.graduation_year));
  }, []);

  /** 提取结果回填表单；画像仍是可编辑草稿，用户确认后才保存。 */
  const applyExtracted = useCallback((profile: CareerProfileInput) => {
    setForm((current) =>
      current
        ? {
            ...current,
            target_roles: profile.target_roles ?? current.target_roles,
            tech_stack: profile.tech_stack ?? current.tech_stack,
            locations: profile.locations ?? current.locations,
            job_type: profile.job_type ?? current.job_type,
            resume_text: profile.resume_text ?? current.resume_text,
          }
        : current
    );
    setYearValue(
      profile.graduation_year == null ? "" : String(profile.graduation_year)
    );
  }, []);

  const loadProfile = useCallback(() => {
    const version = ++requestVersion.current;
    void getCareerProfile().then(
      (response) => {
        if (version !== requestVersion.current) return;
        setState({ kind: "ready", profile: response.profile });
        applyProfile(response.profile);
      },
      (error: unknown) => {
        if (version !== requestVersion.current) return;
        if (handleUnauthorized(error)) return;
        // 403 lifetime_required means the account is signed in but not a
        // Lifetime member: the page shows the gate instead of a retry loop.
        if (isCareerLifetimeRequiredError(error)) {
          setState({ kind: "locked" });
          return;
        }
        setState({ kind: "error", message: formatPortalError(error) });
      }
    );
  }, [applyProfile, handleUnauthorized]);

  useEffect(() => {
    loadProfile();
    return () => {
      requestVersion.current += 1;
    };
  }, [loadProfile]);

  const setField = useCallback(
    (patch: Partial<ProfileForm>) => {
      setForm((current) => (current ? { ...current, ...patch } : current));
      setSaveSuccess(false);
      setSaveError("");
    },
    []
  );

  /** 轮询识别任务直到终态；完成即回填表单，失败给出可读中文错误。 */
  const activeExtraction =
    upload.kind === "active" ? upload.extraction : null;
  useEffect(() => {
    if (!activeExtraction) return;
    const runner = createExtractionRunner({
      fetchStatus: async (id) => (await getCareerResumeExtraction(id)).extraction,
      onState: (next) => {
        if (next.kind === "completed") {
          if (next.extraction.extracted) applyExtracted(next.extraction.extracted);
          setUpload({ kind: "done", extraction: next.extraction });
          return;
        }
        if (next.kind === "failed") {
          setUpload({
            kind: "error",
            message: extractionFailedMessage(next.extraction.error_code),
          });
          return;
        }
        setUpload({
          kind: "active",
          extraction: next.extraction,
          label: extractionStatusLabel(next.extraction.status),
        });
      },
    });
    runner.start(activeExtraction);
    return () => runner.stop();
  }, [activeExtraction, applyExtracted]);

  const handleFilePick = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0] ?? null;
    // 允许连续选择同一个文件。
    event.target.value = "";
    if (!file) return;
    const ext = file.name.toLowerCase().split(".").pop() ?? "";
    if (!(RESUME_EXTENSIONS as readonly string[]).includes(ext)) {
      setSelectedFile(null);
      setUpload({
        kind: "error",
        message: "简历文件无法识别，请上传 PDF、DOCX 或 TXT 格式",
      });
      return;
    }
    if (file.size > RESUME_MAX_BYTES) {
      setSelectedFile(null);
      setUpload({
        kind: "error",
        message: "简历文件超过 10 MB 上限，请压缩后重试",
      });
      return;
    }
    setSelectedFile(file);
    setUpload({ kind: "idle" });
    setSaveSuccess(false);
    setSaveError("");
  };

  const uploadResume = async () => {
    if (!selectedFile || upload.kind === "uploading" || upload.kind === "active") return;
    setUpload({ kind: "uploading" });
    setSaveError("");
    setSaveSuccess(false);
    try {
      const response = await createCareerResumeExtraction(selectedFile);
      setUpload({
        kind: "active",
        extraction: response.extraction,
        label: extractionStatusLabel(response.extraction.status),
      });
    } catch (error) {
      if (handleUnauthorized(error)) return;
      if (isCareerLifetimeRequiredError(error)) {
        setState({ kind: "locked" });
        return;
      }
      setUpload({ kind: "error", message: extractionCreateFailedMessage(error) });
    }
  };

  const save = async () => {
    if (!form || saving) return;
    const targetRoles = form.target_roles.trim();
    if (!targetRoles) {
      setSaveError("请填写目标岗位 / 方向后再保存画像。");
      return;
    }
    const yearError = graduationYearError(yearValue);
    if (yearError) {
      setSaveError(yearError);
      return;
    }
    setSaving(true);
    setSaveError("");
    setSaveSuccess(false);
    try {
      const input: CareerProfileInput = {
        ...form,
        target_roles: targetRoles,
        graduation_year: yearValue.trim() === "" ? null : Number(yearValue),
      };
      const response = await requestCareerProfileUpdate(input);
      setState({ kind: "ready", profile: response.profile });
      applyProfile(response.profile);
      setSaveSuccess(true);
    } catch (error) {
      if (handleUnauthorized(error)) return;
      if (isCareerLifetimeRequiredError(error)) {
        setState({ kind: "locked" });
        return;
      }
      setSaveError(formatPortalError(error));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <section data-enter className="border-b border-ink pb-5">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/55">
          <span className="text-accent">A-08</span>
          <span className="mx-2">/</span>
          CAREER PROFILE
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">求职画像</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/60">
          画像用于求职雷达匹配受控招聘来源；简历文件仅在识别任务期间临时保存，任务完成或失败后删除原文件字节，不保存招聘站账号或密码。
        </p>
      </section>

      {state.kind === "loading" ? (
        <section
          data-account-career-profile-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          CAREER PROFILE LOADING<span className="animate-pulse text-accent">…</span>
        </section>
      ) : null}

      {state.kind === "locked" ? (
        <section data-account-career-profile-state="locked" className="mt-6 border border-accent px-5 py-8">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">LIFETIME REQUIRED</p>
          <h2 className="mt-3 font-display text-2xl font-bold tracking-tight">求职雷达需要 Lifetime VIP 会员</h2>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/70">
            求职画像、匹配扫描与结果简报属于终身会员权益；免费会员无法查看或编辑画像。
          </p>
          <Link
            href="/account/membership"
            className="mt-6 inline-flex min-h-11 items-center justify-center border border-ink px-5 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            前往会员权益开通
          </Link>
        </section>
      ) : null}

      {state.kind === "error" ? (
        <section data-account-career-profile-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">CAREER PROFILE UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
          <p className="mt-3 text-sm leading-6 text-ink/60">画像加载不出来时，不会以本地或会话数据替代真实画像。</p>
          <button
            type="button"
            onClick={() => {
              setState({ kind: "loading" });
              loadProfile();
            }}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {state.kind === "ready" && form ? (
        <section data-account-career-profile-state="ready" data-enter className="mt-6">
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void save();
            }}
          >
            <div data-account-career-extraction className="border border-ink/20 px-5 py-6 md:px-6">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <p className="font-mono text-[10px] tracking-[0.25em] text-ink/60">
                  上传简历 · 自动识别填写（默认方式）
                </p>
                <p className="font-mono text-[10px] text-ink/40">PDF ≤10 页 · DOCX / TXT · 全部 ≤10 MB</p>
              </div>
              <p className="mt-2 text-sm leading-6 text-ink/55">
                上传简历后由后台 AI 识别并自动填入下方画像字段，识别结果可核对修改后再保存；原文件字节在任务完成或失败后删除，只保留提取结果。
              </p>
              <div className="mt-4 flex flex-wrap items-center gap-3">
                <input
                  ref={fileInputRef}
                  id="career-resume-upload"
                  type="file"
                  accept=".pdf,.docx,.txt"
                  className="hidden"
                  onChange={handleFilePick}
                />
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  className="inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
                >
                  选择简历文件
                </button>
                {selectedFile ? (
                  <span className="max-w-56 truncate font-mono text-xs text-ink/70">
                    {selectedFile.name}
                  </span>
                ) : (
                  <span className="font-mono text-[10px] tracking-[0.15em] text-ink/35">
                    未选择文件
                  </span>
                )}
                <button
                  type="button"
                  disabled={!selectedFile || upload.kind === "uploading" || upload.kind === "active"}
                  onClick={() => void uploadResume()}
                  className="inline-flex min-h-11 items-center justify-center border border-ink bg-ink px-4 py-2 font-mono text-xs tracking-widest text-paper transition-colors hover:bg-paper hover:text-ink disabled:cursor-wait disabled:opacity-50"
                >
                  {upload.kind === "uploading" ? "上传中…" : "上传并识别"}
                </button>
              </div>
              {upload.kind === "active" ? (
                <p data-account-career-extraction="active" aria-live="polite" className="mt-4 border border-line px-4 py-3 text-sm leading-6 text-ink/70">
                  识别任务{upload.label}…通常几十秒，识别完成自动填入表单
                </p>
              ) : null}
              {upload.kind === "done" ? (
                <p data-account-career-extraction="done" aria-live="polite" className="mt-4 border border-ink px-4 py-3 text-sm leading-6 text-ink/75">
                  已识别并填入画像字段，请核对修改后点击「保存画像」。
                </p>
              ) : null}
              {upload.kind === "error" ? (
                <p data-account-career-extraction="error" role="alert" className="mt-4 border border-accent px-4 py-3 text-sm leading-6 text-ink/75">
                  {upload.message}
                </p>
              ) : null}
            </div>

            <div className="grid gap-10 border-t border-ink pt-8 md:grid-cols-2">
              <div className="space-y-8 md:col-span-2">
                <div>
                  <label htmlFor="career-target-roles" className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                    目标岗位 / 方向（≤500 字）
                  </label>
                  <textarea
                    id="career-target-roles"
                    value={form.target_roles}
                    onChange={(e) => setField({ target_roles: e.target.value })}
                    required
                    maxLength={FIELD_LIMITS.target_roles}
                    rows={3}
                    placeholder="例如：后端开发、数据分析、产品运营"
                    className="w-full resize-y border-b border-ink/30 bg-transparent py-2 font-mono text-sm leading-6 outline-none placeholder:text-ink/30 focus:border-ink"
                  />
                  <p className="mt-1 text-right font-mono text-[10px] text-ink/40">
                    {form.target_roles.length} / {FIELD_LIMITS.target_roles}
                  </p>
                </div>

                <div>
                  <label htmlFor="career-tech-stack" className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                    技术栈关键词（≤1000 字）
                  </label>
                  <textarea
                    id="career-tech-stack"
                    value={form.tech_stack}
                    onChange={(e) => setField({ tech_stack: e.target.value })}
                    maxLength={FIELD_LIMITS.tech_stack}
                    rows={3}
                    placeholder="例如：Go、Vue、PostgreSQL"
                    className="w-full resize-y border-b border-ink/30 bg-transparent py-2 font-mono text-sm leading-6 outline-none placeholder:text-ink/30 focus:border-ink"
                  />
                  <p className="mt-1 text-right font-mono text-[10px] text-ink/40">
                    {form.tech_stack.length} / {FIELD_LIMITS.tech_stack}
                  </p>
                </div>

                <div>
                  <label htmlFor="career-locations" className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                    目标城市（≤500 字）
                  </label>
                  <textarea
                    id="career-locations"
                    value={form.locations}
                    onChange={(e) => setField({ locations: e.target.value })}
                    maxLength={FIELD_LIMITS.locations}
                    rows={2}
                    placeholder="例如：郑州、北京、远程"
                    className="w-full resize-y border-b border-ink/30 bg-transparent py-2 font-mono text-sm leading-6 outline-none placeholder:text-ink/30 focus:border-ink"
                  />
                  <p className="mt-1 text-right font-mono text-[10px] text-ink/40">
                    {form.locations.length} / {FIELD_LIMITS.locations}
                  </p>
                </div>

                <div className="grid gap-8 sm:grid-cols-2">
                  <div>
                    <label htmlFor="career-job-type" className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                      求职类型
                    </label>
                    <select
                      id="career-job-type"
                      value={form.job_type}
                      onChange={(e) => setField({ job_type: e.target.value as CareerJobType })}
                      className="w-full border-b border-ink/30 bg-paper py-2 font-mono text-sm outline-none focus:border-ink"
                    >
                      {JOB_TYPE_OPTIONS.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label htmlFor="career-graduation-year" className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                      毕业年份（可空）
                    </label>
                    <input
                      id="career-graduation-year"
                      type="text"
                      inputMode="numeric"
                      value={yearValue}
                      onChange={(e) => {
                        setYearValue(e.target.value.replace(/\D/g, "").slice(0, 4));
                        setSaveSuccess(false);
                        setSaveError("");
                      }}
                      maxLength={4}
                      placeholder="例如 2027"
                      className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                    />
                  </div>
                </div>

                <div>
                  <label htmlFor="career-resume-text" className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                    经历摘要（≤4000 字）
                  </label>
                  <textarea
                    id="career-resume-text"
                    value={form.resume_text}
                    onChange={(e) => setField({ resume_text: e.target.value })}
                    maxLength={FIELD_LIMITS.resume_text}
                    rows={6}
                    placeholder="简述项目、竞赛或实习经历，用于匹配命中原因说明。不上传文件。"
                    className="w-full resize-y border-b border-ink/30 bg-transparent py-2 font-mono text-sm leading-6 outline-none placeholder:text-ink/30 focus:border-ink"
                  />
                  <p className="mt-1 text-right font-mono text-[10px] text-ink/40">
                    {form.resume_text.length} / {FIELD_LIMITS.resume_text}
                  </p>
                </div>

                <div className="flex items-start gap-3 border border-line px-4 py-4">
                  <input
                    id="career-email-notification"
                    type="checkbox"
                    checked={form.email_notification_enabled}
                    onChange={(e) => setField({ email_notification_enabled: e.target.checked })}
                    className="mt-1 size-4 shrink-0 accent-ink"
                  />
                  <div>
                    <label htmlFor="career-email-notification" className="block font-mono text-[10px] tracking-[0.25em] text-ink/60">
                      扫描结果邮件通知
                    </label>
                    <p className="mt-1 text-sm leading-6 text-ink/55">
                      开启后，求职雷达扫描完成时向当前账户邮箱发送结果简报；关闭后仅站内查看。
                    </p>
                  </div>
                </div>
              </div>
            </div>

            {saveError ? (
              <p role="alert" data-account-career-profile-save="error" className="mt-6 border border-accent px-4 py-3 text-sm leading-6 text-ink/75">
                {saveError}
              </p>
            ) : null}
            {saveSuccess ? (
              <p data-account-career-profile-save="success" aria-live="polite" className="mt-6 border border-ink px-4 py-3 text-sm leading-6 text-ink/75">
                求职画像已保存，将用于下一次求职雷达匹配。
              </p>
            ) : null}

            <div className="mt-8 flex items-center gap-4">
              <button
                type="submit"
                disabled={saving}
                className="inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper disabled:cursor-wait disabled:opacity-50"
              >
                {saving ? "保存中…" : "保存画像"}
              </button>
              <p className="font-mono text-[10px] tracking-[0.15em] text-ink/40">
                画像由服务端保存，可跨设备读取
              </p>
            </div>
          </form>
        </section>
      ) : null}
    </div>
  );
}
