import type { ChangeEvent, RefObject } from "react";
import type { CareerResumeExtraction } from "@/lib/api/types";

export type UploadState =
  | { kind: "idle" }
  | { kind: "uploading" }
  | { kind: "active"; extraction: CareerResumeExtraction; label: string }
  | { kind: "done"; extraction: CareerResumeExtraction }
  | { kind: "error"; message: string };

export type SuificationState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "preview"; original: string; draft: string }
  | { kind: "applied"; original: string }
  | { kind: "error"; message: string };

export function CareerResumeUpload({ fileInputRef, handleFilePick, selectedFile, upload, uploadResume }: {
  fileInputRef: RefObject<HTMLInputElement | null>;
  handleFilePick: (event: ChangeEvent<HTMLInputElement>) => void;
  selectedFile: File | null;
  upload: UploadState;
  uploadResume: () => Promise<void>;
}) {
  return (
    <div data-account-career-extraction className="border border-ink/20 px-5 py-6 md:px-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="font-mono text-xs text-ink/60">
          上传简历 · 自动识别填写（默认方式）
        </p>
        <p className="font-mono text-xs text-ink/60">PDF ≤10 页 · DOCX / TXT · 全部 ≤10 MB</p>
      </div>
      <p className="mt-2 text-sm leading-6 text-ink/60">
        上传简历后由后台 AI 识别并自动填入下方画像字段，识别结果可核对修改后再保存；简历文件在识别完成或失败后即删除，保留文件名、文件校验值和识别出的内容。
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
          className="inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs transition-colors hover:bg-ink hover:text-paper"
        >
          选择简历文件
        </button>
        {selectedFile ? (
          <span className="max-w-56 truncate font-mono text-xs text-ink/70">
            {selectedFile.name}
          </span>
        ) : (
          <span className="font-mono text-xs text-ink/60">
            未选择文件
          </span>
        )}
        <button
          type="button"
          disabled={!selectedFile || upload.kind === "uploading" || upload.kind === "active"}
          onClick={() => void uploadResume()}
          className={`inline-flex min-h-11 items-center justify-center border border-ink bg-ink px-4 py-2 font-mono text-xs text-paper transition-colors hover:bg-paper hover:text-ink disabled:opacity-50 ${upload.kind === "uploading" ? "disabled:cursor-wait" : "disabled:cursor-not-allowed"}`}
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
  );
}

export function CareerResumeSummary({
  resumeText, maxLength, onChange, upload, suification, suifyResume, onDismiss, applySuification, restoreSuificationOriginal,
}: {
  resumeText: string;
  maxLength: number;
  onChange: (value: string) => void;
  upload: UploadState;
  suification: SuificationState;
  suifyResume: () => Promise<void>;
  onDismiss: () => void;
  applySuification: () => void;
  restoreSuificationOriginal: () => void;
}) {
  return (
    <div>
      <div className="mb-1 flex flex-wrap items-center justify-between gap-3">
        <label htmlFor="career-resume-text" className="block font-mono text-xs text-ink/60">
          经历摘要（≤4000 字）
        </label>
        <button
          type="button"
          aria-busy={suification.kind === "loading"}
          disabled={
            !resumeText.trim() ||
            suification.kind === "loading" ||
            upload.kind === "uploading" ||
            upload.kind === "active"
          }
          onClick={() => void suifyResume()}
          className={`inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs text-ink transition-colors enabled:hover:bg-ink enabled:hover:text-paper disabled:border-ink/40 disabled:bg-transparent disabled:text-ink/60 ${suification.kind === "loading" ? "disabled:cursor-wait" : "disabled:cursor-not-allowed"}`}
        >
          {suification.kind === "loading" ? "AI 润色中…" : "AI 润色经历"}
        </button>
      </div>
      <textarea
        id="career-resume-text"
        value={resumeText}
        onChange={(event) => onChange(event.target.value)}
        disabled={suification.kind === "loading"}
        maxLength={maxLength}
        rows={6}
        placeholder="简述项目、竞赛或实习经历，用于匹配命中原因说明。不上传文件。"
        className="w-full resize-y border-b border-ink/30 bg-transparent py-2 font-mono text-sm leading-6 outline-none placeholder:text-ink/60 focus:border-ink"
      />
      <p className="mt-1 text-right font-mono text-xs text-ink/60">
        {resumeText.length} / {maxLength}
      </p>
      <p className="mt-2 text-xs leading-5 text-ink/60">
        润色会把当前经历摘要发给外部 AI 服务，服务方可能按自己的规则处理这些内容。草稿最多保留 10 分钟，由你决定是否采用。请核对事实后再应用。
      </p>
      {suification.kind === "preview" ? (
        <div
          data-account-career-suification="preview"
          className="mt-4 border border-ink px-4 py-4"
        >
          <label htmlFor="career-resume-suification-preview" className="font-mono text-xs text-ink/60">
            润色预览
          </label>
          <textarea
            id="career-resume-suification-preview"
            aria-label="润色预览"
            readOnly
            value={suification.draft}
            rows={6}
            className="mt-2 w-full resize-y border-b border-ink/30 bg-transparent py-2 font-mono text-sm leading-6 outline-none"
          />
          <div className="mt-4 flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={onDismiss}
              className="inline-flex min-h-11 items-center justify-center border border-line px-4 py-2 font-mono text-xs transition-colors hover:border-ink"
            >
              撤销
            </button>
            <button
              type="button"
              onClick={applySuification}
              className="inline-flex min-h-11 items-center justify-center border border-ink bg-ink px-4 py-2 font-mono text-xs text-paper transition-colors hover:bg-paper hover:text-ink"
            >
              应用
            </button>
          </div>
        </div>
      ) : null}
      {suification.kind === "applied" ? (
        <button
          type="button"
          onClick={restoreSuificationOriginal}
          className="mt-4 inline-flex min-h-11 items-center justify-center border border-line px-4 py-2 font-mono text-xs transition-colors hover:border-ink"
        >
          恢复原文
        </button>
      ) : null}
      {suification.kind === "error" ? (
        <p
          data-account-career-suification="error"
          role="alert"
          className="mt-4 border border-accent px-4 py-3 text-sm leading-6 text-ink/75"
        >
          {suification.message}
        </p>
      ) : null}
    </div>
  );
}
