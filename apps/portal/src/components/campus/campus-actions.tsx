"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, useSyncExternalStore, type ChangeEvent, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import Img from "@/components/ui/img";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { formatPortalError, PortalHttpError, PortalUnauthorizedError } from "@/lib/api/client";
import { authStore } from "@/lib/auth/store";
import { cn } from "@/lib/cn";
import {
  CONTACT_CHANNELS,
  publishCampusItem,
  readCampusContactBook,
  saveCampusContactBook,
  type ContactBook,
  type ContactChannel,
} from "@/lib/campus/gateway";
import { foodPostImagePreview, type FoodPostImageInput } from "@/lib/food/submit";
import { IMAGE_MAX_SIZE } from "@/lib/image";

const MAX_CAMPUS_IMAGES = 6;
const CAMPUS_WRITES_UNAVAILABLE_CODE = "CAMPUS_WRITES_UNAVAILABLE";

const BOXED_INPUT_CLASS =
  "rounded-none border border-ink/25 bg-paper-raised px-3 focus:border-ink focus-visible:outline-2 focus-visible:outline-focus-ring";
const BOXED_TEXTAREA_CLASS =
  "min-h-24 rounded-none border border-ink/25 bg-paper-raised px-3 py-2.5 focus:border-ink focus-visible:outline-2 focus-visible:outline-focus-ring";
const FIELD_LABEL_CLASS = "block font-mono text-xs text-ink/70";
const PRICE_INPUT_CLASS =
  "pl-7 tabular-nums [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none";

const CONTACT_PLACEHOLDERS: Record<ContactChannel, string> = {
  qq: "例如：123456789",
  wechat: "例如：微信号或绑定手机号",
  phone: "11 位手机号",
};

const CAMPUS_TYPE_OPTIONS: ReadonlyArray<{ value: "help" | "sell"; label: string }> = [
  { value: "help", label: "互助" },
  { value: "sell", label: "闲置" },
];

export const loginForCampus = (path: string) => `/account/login?next=${encodeURIComponent(path)}`;
export const useCampusViewer = () =>
  useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);

/** 服务未开启（503 CAMPUS_WRITES_UNAVAILABLE）与临时网络错误区分处理。 */
export function isCampusWritesUnavailable(error: unknown): boolean {
  return (
    error instanceof PortalHttpError &&
    error.status === 503 &&
    error.errorCode === CAMPUS_WRITES_UNAVAILABLE_CODE
  );
}

export function formatCampusUnavailableMessage(error: unknown): string {
  const formatted = formatPortalError(error);
  if (formatted && formatted !== "服务暂时不可用，请稍后再试。") return formatted;
  return "互助发布与联系方式簿暂未开启，请稍后再来查看。";
}

export function CampusContactDialog({
  path,
  onSaved,
}: {
  path: string;
  onSaved?: (book: ContactBook) => void;
}) {
  const { user, ready } = useCampusViewer();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">联系方式簿</Button>
      </DialogTrigger>
      <DialogContent className="max-h-[88dvh] w-[min(92vw,34rem)] overscroll-contain p-5 sm:p-6">
        <DialogHeader className="border-b border-line pb-4">
          <DialogTitle>联系方式簿</DialogTitle>
          <DialogDescription>
            联系方式不会出现在公开单子里。双方同意交换后，所选渠道和备注才会分享给对方。
          </DialogDescription>
        </DialogHeader>
        {ready && user && open ? (
          <ContactForm key={user.uid} path={path} onSaved={onSaved} onClose={() => setOpen(false)} />
        ) : ready ? (
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-ink/70">登录后即可填写并管理你的联系方式。</p>
            <Button asChild>
              <Link href={loginForCampus(path)}>登录后设置</Link>
            </Button>
          </div>
        ) : (
          <p role="status" className="mt-5 text-sm text-ink/70">
            正在核对登录状态…
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}

function ContactForm({
  path,
  onSaved,
  onClose,
}: {
  path: string;
  onSaved?: (book: ContactBook) => void;
  onClose: () => void;
}) {
  const [contact, setContact] = useState<ContactBook>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [failure, setFailure] = useState("");
  const [expired, setExpired] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    void readCampusContactBook(controller.signal)
      .then(({ contact: result }) => {
        if (controller.signal.aborted) return;
        setUnavailable(false);
        setContact(result);
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        const serviceUnavailable = isCampusWritesUnavailable(error);
        setUnavailable(serviceUnavailable);
        setExpired(error instanceof PortalUnauthorizedError);
        setFailure(serviceUnavailable ? formatCampusUnavailableMessage(error) : formatPortalError(error));
      });
    return () => controller.abort();
  }, [attempt]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!contact || busy) return;
    setBusy(true);
    setFailure("");
    setMessage("");
    try {
      const response = await saveCampusContactBook(contact);
      setContact(response.contact);
      setUnavailable(false);
      onSaved?.(response.contact);
      setMessage("联系方式已保存。");
    } catch (error) {
      const serviceUnavailable = isCampusWritesUnavailable(error);
      setUnavailable(serviceUnavailable);
      setExpired(error instanceof PortalUnauthorizedError);
      setFailure(serviceUnavailable ? formatCampusUnavailableMessage(error) : formatPortalError(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mt-5 space-y-4">
      {expired ? (
        <div className="space-y-3">
          {failure && (
            <p role="alert" className="text-sm text-accent-text">
              {failure}
            </p>
          )}
          <Button asChild>
            <Link href={loginForCampus(path)}>重新登录</Link>
          </Button>
        </div>
      ) : unavailable && !contact ? (
        <div
          role="status"
          data-campus-writes-state="unavailable"
          className="space-y-2 border border-ink/20 bg-paper-raised p-4"
        >
          <p className="font-mono text-xs text-ink/70">暂未开启</p>
          <p className="text-sm leading-6 text-ink">{failure}</p>
          <p className="text-xs leading-5 text-ink/65">
            当前仍可浏览公开单子，待功能开启后再回来设置联系方式。
          </p>
        </div>
      ) : contact ? (
        <form onSubmit={(event) => void save(event)}>
          <fieldset disabled={busy} className="min-w-0 space-y-4">
            {(Object.keys(CONTACT_CHANNELS) as ContactChannel[]).map((channel) => {
              const fieldId = `campus-contact-${channel}`;
              return (
                <div key={channel} className="space-y-1.5">
                  <label htmlFor={fieldId} className={FIELD_LABEL_CLASS}>
                    {CONTACT_CHANNELS[channel]}
                  </label>
                  <Input
                    id={fieldId}
                    inputMode={channel === "phone" ? "tel" : channel === "qq" ? "numeric" : undefined}
                    maxLength={channel === "phone" ? 32 : 64}
                    placeholder={CONTACT_PLACEHOLDERS[channel]}
                    className={BOXED_INPUT_CLASS}
                    value={contact[channel]}
                    onChange={(event) => setContact({ ...contact, [channel]: event.target.value })}
                  />
                </div>
              );
            })}
            <div className="space-y-1.5">
              <label htmlFor="campus-contact-note" className={FIELD_LABEL_CLASS}>
                备注（选填）
              </label>
              <Textarea
                id="campus-contact-note"
                maxLength={200}
                placeholder="例如：晚课后方便联系"
                className={BOXED_TEXTAREA_CLASS}
                value={contact.note}
                onChange={(event) => setContact({ ...contact, note: event.target.value })}
              />
            </div>

            {failure && (
              <p role="alert" className="text-sm text-accent-text">
                {failure}
              </p>
            )}
            {message && (
              <p role="status" className="text-sm text-ink">
                {message}
              </p>
            )}

            <div className="flex flex-wrap items-center justify-end gap-3 border-t border-line pt-4">
              <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
                关闭
              </Button>
              <Button type="submit">{busy ? "正在保存…" : "保存联系方式"}</Button>
            </div>
          </fieldset>
        </form>
      ) : failure ? (
        <div className="space-y-3">
          <p role="alert" className="text-sm text-accent-text">
            {failure}
          </p>
          <Button
            variant="outline"
            onClick={() => {
              setFailure("");
              setAttempt((value) => value + 1);
            }}
          >
            重新加载
          </Button>
        </div>
      ) : (
        <p role="status" className="text-sm text-ink/70">
          正在加载联系方式…
        </p>
      )}
    </div>
  );
}

export function updateCampusPublishDialog(
  open: boolean,
  pending: boolean,
  resetDraft: () => void,
  setOpen: (open: boolean) => void,
) {
  if (pending) return;
  if (!open) resetDraft();
  setOpen(open);
}

export function CampusPublishDialog({
  categories,
}: {
  categories: { key: string; name: string }[];
}) {
  const { user, ready } = useCampusViewer();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [expired, setExpired] = useState(false);
  const [itemType, setItemType] = useState<"help" | "sell">("help");
  const [category, setCategory] = useState(categories[0]?.key ?? "");
  const [photos, setPhotos] = useState<{ preview: string; image: FoodPostImageInput }[]>([]);
  const [readingPhotos, setReadingPhotos] = useState(false);
  const [photoError, setPhotoError] = useState("");

  const activeCategory = categories.some((entry) => entry.key === category)
    ? category
    : (categories[0]?.key ?? "");

  function resetDraft() {
    setItemType("help");
    setCategory(categories[0]?.key ?? "");
    setPhotos([]);
    setPhotoError("");
    setFailure("");
    setExpired(false);
  }

  function changePublishDialogOpen(next: boolean) {
    updateCampusPublishDialog(next, busy || readingPhotos, resetDraft, setOpen);
  }

  async function selectPhotos(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.currentTarget.files ?? []);
    event.currentTarget.value = "";
    if (!files.length || readingPhotos) return;
    setPhotoError("");
    if (photos.length + files.length > MAX_CAMPUS_IMAGES) {
      setPhotoError(`最多上传 ${MAX_CAMPUS_IMAGES} 张图片，请减少选择数量。`);
      return;
    }
    setReadingPhotos(true);
    try {
      const selected: { preview: string; image: FoodPostImageInput }[] = [];
      for (const file of files) {
        const contentType = file.type;
        if (contentType !== "image/jpeg" && contentType !== "image/png" && contentType !== "image/webp") {
          throw new Error("仅支持 JPG、PNG、WebP 格式图片，请重新选择。");
        }
        if (file.size > IMAGE_MAX_SIZE) {
          throw new Error("单张图片不能超过 2 MB，请压缩后重新选择。");
        }
        const preview = await foodPostImagePreview(file);
        selected.push({
          preview,
          image: { content_type: contentType, data: preview.slice(preview.indexOf(",") + 1) },
        });
      }
      setPhotos((current) => [...current, ...selected]);
    } catch (error) {
      setPhotoError(error instanceof Error ? error.message : "图片无法读取，请重新选择 JPG、PNG 或 WebP 图片。");
    } finally {
      setReadingPhotos(false);
    }
  }

  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || readingPhotos) return;
    const fields = new FormData(event.currentTarget);
    const value = (name: string) => String(fields.get(name) ?? "").trim();
    if (!activeCategory) {
      setFailure("请选择一个分类后再发布。");
      return;
    }
    setBusy(true);
    setFailure("");
    try {
      const { item } = await publishCampusItem({
        type: itemType,
        category: activeCategory,
        title: value("title"),
        desc: value("desc"),
        price: Number(value("price")),
        place: value("place"),
        deadline: value("deadline") || undefined,
        images: photos.length ? photos.map(({ image }) => image) : undefined,
      });
      resetDraft();
      setOpen(false);
      router.push(`/campus/item/${encodeURIComponent(item.id)}`);
    } catch (error) {
      setExpired(error instanceof PortalUnauthorizedError);
      setFailure(
        isCampusWritesUnavailable(error)
          ? formatCampusUnavailableMessage(error)
          : formatPortalError(error)
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={changePublishDialogOpen}
    >
      <DialogTrigger asChild>
        <Button>发布新单子</Button>
      </DialogTrigger>
      <DialogContent className="max-h-[88dvh] w-[min(92vw,38rem)] overscroll-contain p-5 sm:p-6">
        <DialogHeader className="border-b border-line pb-4">
          <DialogTitle>发布新单子</DialogTitle>
          <DialogDescription>
            写清楚需求、地点与时间，请勿在标题、地点或说明中填写私人联系方式。
          </DialogDescription>
        </DialogHeader>
        {ready && user ? (
          <form className="mt-5" onSubmit={(event) => void publish(event)}>
            <fieldset disabled={busy || expired || readingPhotos} className="min-w-0 space-y-4">
              {/* 类型：2 格直角分段切换按钮 */}
              <div className="space-y-1.5">
                <p id="campus-publish-type-label" className={FIELD_LABEL_CLASS}>
                  类型
                </p>
                <div
                  role="group"
                  aria-labelledby="campus-publish-type-label"
                  className="grid grid-cols-2 gap-2"
                >
                  {CAMPUS_TYPE_OPTIONS.map((option) => (
                    <button
                      key={option.value}
                      type="button"
                      aria-pressed={itemType === option.value}
                      onClick={() => setItemType(option.value)}
                      className={cn(
                        "min-h-11 border px-3 font-mono text-xs transition-colors focus-visible:outline-2 focus-visible:outline-focus-ring",
                        itemType === option.value
                          ? "border-ink bg-ink text-paper"
                          : "border-ink/25 bg-paper-raised text-ink/75 hover:border-ink"
                      )}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
              </div>

              {/* 分类：直角可点选标签按钮网格 */}
              <div className="space-y-1.5">
                <p id="campus-publish-category-label" className={FIELD_LABEL_CLASS}>
                  分类
                </p>
                <div
                  role="group"
                  aria-labelledby="campus-publish-category-label"
                  className="flex flex-wrap gap-2"
                >
                  {categories.map(({ key, name }) => (
                    <button
                      key={key}
                      type="button"
                      aria-pressed={activeCategory === key}
                      onClick={() => setCategory(key)}
                      className={cn(
                        "min-h-11 border px-3 font-mono text-xs transition-colors focus-visible:outline-2 focus-visible:outline-focus-ring",
                        activeCategory === key
                          ? "border-ink bg-ink text-paper"
                          : "border-ink/25 bg-paper-raised text-ink/75 hover:border-ink"
                      )}
                    >
                      {name}
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-1.5">
                <label htmlFor="campus-publish-title" className={FIELD_LABEL_CLASS}>
                  标题
                </label>
                <Input
                  id="campus-publish-title"
                  name="title"
                  maxLength={120}
                  placeholder="例如：明伦北苑代取顺丰快递"
                  className={BOXED_INPUT_CLASS}
                  required
                />
              </div>

              <div className="space-y-1.5">
                <label htmlFor="campus-publish-desc" className={FIELD_LABEL_CLASS}>
                  说明
                </label>
                <Textarea
                  id="campus-publish-desc"
                  name="desc"
                  maxLength={1000}
                  placeholder="写明成色、要求或交接时间，请勿留微信号/手机号"
                  className={BOXED_TEXTAREA_CLASS}
                  required
                />
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <label htmlFor="campus-publish-place" className={FIELD_LABEL_CLASS}>
                    地点
                  </label>
                  <Input
                    id="campus-publish-place"
                    name="place"
                    maxLength={120}
                    placeholder="例如：明伦校区北苑楼下"
                    className={BOXED_INPUT_CLASS}
                    required
                  />
                </div>

                <div className="space-y-1.5">
                  <label htmlFor="campus-publish-price" className={FIELD_LABEL_CLASS}>
                    {itemType === "help" ? "赏金（元）" : "金额（元）"}
                  </label>
                  <div className="relative flex items-center">
                    <span
                      aria-hidden="true"
                      className="pointer-events-none absolute left-3 font-mono text-sm text-ink/70"
                    >
                      ¥
                    </span>
                    <Input
                      id="campus-publish-price"
                      name="price"
                      type="number"
                      inputMode="decimal"
                      min="0"
                      max="100000"
                      step="0.01"
                      defaultValue="0"
                      placeholder="0"
                      className={cn(BOXED_INPUT_CLASS, PRICE_INPUT_CLASS)}
                      required
                    />
                  </div>
                </div>
              </div>

              <div className="space-y-1.5">
                <label htmlFor="campus-publish-deadline" className={FIELD_LABEL_CLASS}>
                  时限（选填）
                </label>
                <Input
                  id="campus-publish-deadline"
                  name="deadline"
                  maxLength={120}
                  placeholder="例如：今晚 21:00 前"
                  className={BOXED_INPUT_CLASS}
                />
              </div>

              {/* 图片上传区：对齐 /food/publish，隐藏原生 file input */}
              <div className="space-y-2 border-t border-line pt-4">
                <p className={FIELD_LABEL_CLASS}>
                  图片（{photos.length}/{MAX_CAMPUS_IMAGES}，单张 ≤2MB，选填）
                </p>
                <p id="campus-publication-images-hint" className="text-xs leading-5 text-ink/65">
                  支持 JPG、PNG、WebP；请勿上传含私人联系方式的图片。
                </p>

                <div className="grid grid-cols-3 gap-2.5 pt-1 sm:gap-3">
                  {photos.map(({ preview }, index) => (
                    <div key={`${preview.slice(0, 48)}-${index}`} className="flex min-w-0 flex-col">
                      <Img
                        src={preview}
                        alt={`待上传图片 ${index + 1}`}
                        label={`FIG.${index + 1}`}
                        className="h-20 w-full"
                      />
                      <button
                        type="button"
                        onClick={() => {
                          setPhotos((current) => current.filter((_, position) => position !== index));
                          setPhotoError("");
                        }}
                        className="flex min-h-11 w-full items-center justify-center font-mono text-xs text-ink/65 transition-colors hover:text-accent-text focus-visible:outline-2 focus-visible:outline-focus-ring"
                        aria-label={`删除图片 ${index + 1}`}
                      >
                        删除
                      </button>
                    </div>
                  ))}
                  {photos.length < MAX_CAMPUS_IMAGES && (
                    <label
                      htmlFor="campus-publication-images"
                      className={cn(
                        "flex h-20 w-full cursor-pointer items-center justify-center border border-dashed border-ink/30 bg-paper-raised font-mono text-xs text-ink/70 transition-colors hover:border-ink hover:text-ink focus-within:outline-2 focus-within:outline-focus-ring",
                        (busy || readingPhotos) && "pointer-events-none opacity-50"
                      )}
                    >
                      <span>{readingPhotos ? "读取中…" : "+ 上传"}</span>
                      <input
                        id="campus-publication-images"
                        type="file"
                        multiple
                        accept="image/jpeg,image/png,image/webp"
                        aria-describedby="campus-publication-images-hint"
                        disabled={busy || readingPhotos || photos.length >= MAX_CAMPUS_IMAGES}
                        className="sr-only"
                        onChange={(event) => void selectPhotos(event)}
                      />
                    </label>
                  )}
                </div>

                {readingPhotos && (
                  <p role="status" className="text-xs text-ink/70">
                    正在读取图片…
                  </p>
                )}
                {photoError && (
                  <p role="alert" className="text-sm text-accent-text">
                    {photoError}
                  </p>
                )}
              </div>

              {failure && (
                <p role="alert" className="text-sm text-accent-text">
                  {failure}
                </p>
              )}

              <div className="flex flex-wrap items-center justify-end gap-3 border-t border-line pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => changePublishDialogOpen(false)}
                  disabled={busy || readingPhotos}
                >
                  取消
                </Button>
                <Button type="submit">{busy ? "正在发布…" : "确认发布"}</Button>
              </div>
            </fieldset>
            {expired && (
              <div className="mt-3">
                <Button asChild>
                  <Link href={loginForCampus("/campus")}>重新登录</Link>
                </Button>
              </div>
            )}
          </form>
        ) : ready ? (
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-ink/70">登录后即可发布互助或闲置单子。</p>
            <Button asChild>
              <Link href={loginForCampus("/campus")}>登录后发布</Link>
            </Button>
          </div>
        ) : (
          <p role="status" className="mt-5 text-sm text-ink/70">
            正在核对登录状态…
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}
