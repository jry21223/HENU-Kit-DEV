"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";

import { useReveal } from "@/components/account/use-reveal";
import PostFormDishes, { MAX_DISHES, type DishDraft } from "@/components/food/post-form-dishes";
import PostFormImages, { MAX_IMAGES, type ImageDraft } from "@/components/food/post-form-images";
import {
  PortalApiError,
  PortalHttpError,
  PortalUnauthorizedError,
} from "@/lib/api/client";
import { authStore } from "@/lib/auth/store";
import { cn } from "@/lib/cn";
import { commandKey } from "@/lib/idempotency";
import { CAMPUSES, CAMPUS_KEYS } from "@/lib/food/campuses";
import { rememberCreatedFoodPost, rememberUpdatedFoodPost } from "@/lib/food/gateway";
import { FOOD_TIERS, type FoodTierKey } from "@/lib/food/ranking";
import {
  createFoodPost,
  updateFoodPost,
  type FoodPostEditData,
  foodPostDailyCapMessage,
  foodPostPlaceholderMessage,
  foodPostImageFromFile,
  foodPostImagePreview,
  isFoodPostDailyCapError,
  isFoodPostPlaceholderError,
  type FoodPostCampus,
  type FoodPostImageInput,
} from "@/lib/food/submit";



function textLength(value: string): number {
  return Array.from(value.trim()).length;
}

const EMPTY_DISH: DishDraft = { name: "", price: "", reason: "" };

interface FoodDraft {
  venue: string;
  campus: FoodPostCampus | null;
  tier: FoodTierKey | null;
  review: string;
  price: string;
  hours: string;
  dishes: DishDraft[];
  imageCount: number;
}

function draftKey(userID: string) {
  return `henukit:food-draft:${userID}`;
}

function storeFoodDraft(userID: string, draft: FoodDraft, images: ImageDraft[]): boolean {
  let saved = false;
  try {
    const key = draftKey(userID);
    window.sessionStorage.removeItem(`${key}:images`);
    window.sessionStorage.setItem(key, JSON.stringify(draft));
    saved = true;
    if (images.length) window.sessionStorage.setItem(`${key}:images`, JSON.stringify(images.map((image) => image.preview)));
  } catch {
    return saved;
  }
  return true;
}

function readFoodDraft(userID: string): (FoodDraft & { images: ImageDraft[] }) | null {
  const key = draftKey(userID);
  try {
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return null;
    const draft = JSON.parse(raw) as FoodDraft;
    let images: ImageDraft[] = [];
    try {
      const previews = JSON.parse(window.sessionStorage.getItem(`${key}:images`) ?? "[]") as string[];
      images = previews.flatMap((preview, index) => {
        const match = /^data:(image\/(?:jpeg|png|webp));base64,(.+)$/.exec(preview);
        if (!match) return [];
        const bytes = Uint8Array.from(window.atob(match[2]), (character) => character.charCodeAt(0));
        return [{ preview, file: new File([bytes], `photo-${index + 1}`, { type: match[1] }) }];
      });
    } catch {
      images = [];
    }
    return { ...draft, images };
  } catch {
    return null;
  }
}

function clearFoodDraft(userID: string) {
  try {
    const key = draftKey(userID);
    window.sessionStorage.removeItem(key);
    window.sessionStorage.removeItem(`${key}:images`);
  } catch {
    return;
  }
}

interface FieldErrors {
  venue?: string;
  campus?: string;
  tier?: string;
  review?: string;
}

export default function FoodPostForm({
  initial,
  onReload,
}: {
  initial?: FoodPostEditData;
  onReload?: () => void;
}) {
  const router = useRouter();
  const { user, ready } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer,
  );
  useReveal();

  const [venue, setVenue] = useState(initial?.venue_name ?? "");
  const [campus, setCampus] = useState<FoodPostCampus | null>(
    initial?.campus ?? null,
  );
  const [tier, setTier] = useState<FoodTierKey | null>(initial?.tier ?? null);
  const [review, setReview] = useState(initial?.review_text ?? "");
  const [price, setPrice] = useState(initial?.price_reference ?? "");
  const [hours, setHours] = useState(initial?.hours_reference ?? "");
  const [dishes, setDishes] = useState<DishDraft[]>(
    initial
      ? initial.dishes.map((dish) => ({
          name: dish.name,
          price: dish.price ?? "",
          reason: dish.reason ?? "",
        }))
      : [{ ...EMPTY_DISH }],
  );
  const [images, setImages] = useState<ImageDraft[]>(
    initial?.images.map((image) => ({
      position: image.position,
      preview: image.url,
    })) ?? [],
  );
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const busyRef = useRef(false);
  const imageReadingRef = useRef(false);
  const [imageReading, setImageReading] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [draftNotice, setDraftNotice] = useState("");
  const returnPath = initial
    ? `/food/post/${initial.id}/edit`
    : "/food/publish";
  const submitKeyRef = useRef<string | null>(null);
  const userID = user?.uid;

  useEffect(() => {
    if (!userID || initial) return;
    const timer = window.setTimeout(() => {
      const draft = readFoodDraft(userID);
      if (!draft) return;
      setVenue(draft.venue);
      setCampus(draft.campus);
      setTier(draft.tier);
      setReview(draft.review);
      setPrice(draft.price);
      setHours(draft.hours);
      setDishes(draft.dishes);
      setImages(draft.images);
      setDraftNotice(draft.images.length < draft.imageCount ? "已恢复上次填写的文字，请重新添加图片。" : "已恢复上次填写的草稿。");
    }, 0);
    return () => window.clearTimeout(timer);
  }, [initial, userID]);

  // 守卫：未登录重定向
  useEffect(() => {
    if (ready && !user) {
      router.replace(`/account/login?next=${encodeURIComponent(returnPath)}`);
    }
  }, [ready, user, router, returnPath]);

  if (!ready || !user) {
    return (
      <main className="flex min-h-[60vh] items-center justify-center">
        <p className="font-mono text-xs text-ink/60">
          正在核对登录状态
          <span aria-hidden className="animate-pulse text-accent-text">
            …
          </span>
        </p>
      </main>
    );
  }

  // 表单被编辑：清除错误并让下次提交换新幂等 key（提交中不改）
  const noteEdit = () => {
    if (!pending) submitKeyRef.current = null;
    setError("");
  };

  const clearFieldError = (key: keyof FieldErrors) => {
    setFieldErrors((errors) => ({ ...errors, [key]: undefined }));
  };

  const addImage = async (event: ChangeEvent<HTMLInputElement>) => {
    if (busyRef.current || imageReadingRef.current) return;
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (images.length >= MAX_IMAGES) {
      setError(`最多上传 ${MAX_IMAGES} 张图片`);
      return;
    }
    imageReadingRef.current = true;
    setImageReading(true);
    try {
      const preview = await foodPostImagePreview(file);
      setImages((current) => [...current, { file, preview }]);
      noteEdit();
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "图片读取失败");
    } finally {
      imageReadingRef.current = false;
      setImageReading(false);
    }
  };

  const removeImage = (index: number) => {
    setImages((current) => current.filter((_, i) => i !== index));
    noteEdit();
  };

  const updateDish = (index: number, patch: Partial<DishDraft>) => {
    setDishes((current) =>
      current.map((dish, i) => (i === index ? { ...dish, ...patch } : dish)),
    );
    noteEdit();
  };

  const addDish = () => {
    if (dishes.length >= MAX_DISHES) return;
    setDishes((current) => [...current, { ...EMPTY_DISH }]);
    noteEdit();
  };

  const removeDish = (index: number) => {
    setDishes((current) => current.filter((_, i) => i !== index));
    noteEdit();
  };

  const submit = async () => {
    if (
      busyRef.current ||
      imageReadingRef.current ||
      conflict ||
      sessionExpired
    )
      return;
    const cleanedVenue = venue.trim();
    const cleanedReview = review.trim();
    if (!cleanedVenue || !campus || !tier || !cleanedReview) {
      setFieldErrors({
        venue: cleanedVenue ? undefined : "请填写店铺名",
        campus: campus ? undefined : "请选择校区",
        tier: tier ? undefined : "请选择五档定位",
        review: cleanedReview ? undefined : "请填写锐评正文",
      });
      setError("必填项还没填完，请先补齐再提交。");
      return;
    }

    if (dishes.some((dish) => !dish.name.trim() && (dish.price.trim() || dish.reason.trim()))) {
      setError("请为已填写价格或理由的菜品补充菜名。");
      return;
    }

    const venueError =
      textLength(cleanedVenue) > 160 ? "店铺名不能超过 160 字" : undefined;
    const reviewLength = textLength(cleanedReview);
    const reviewError =
      reviewLength < 2
        ? "锐评正文至少填写 2 个字"
        : reviewLength > 2000
          ? "锐评正文不能超过 2000 字"
          : undefined;
    const detailError =
      textLength(price) > 200
        ? "价格参考不能超过 200 字"
        : textLength(hours) > 200
          ? "营业参考不能超过 200 字"
          : dishes.find((dish) => textLength(dish.name) > 80)
            ? "菜名不能超过 80 字"
            : dishes.find((dish) => textLength(dish.price) > 40)
              ? "菜品价格不能超过 40 字"
              : dishes.find((dish) => textLength(dish.reason) > 200)
                ? "菜品理由不能超过 200 字"
                : undefined;
    if (venueError || reviewError || detailError) {
      setFieldErrors({ venue: venueError, review: reviewError });
      setError(
        venueError ?? reviewError ?? detailError ?? "请检查投稿内容后重试。",
      );
      return;
    }

    busyRef.current = true;
    setPending(true);
    setError("");
    const idempotencyKey =
      submitKeyRef.current ?? commandKey("portal-food-post");
    submitKeyRef.current = idempotencyKey;

    try {
      const imageInputs: FoodPostImageInput[] = [];
      for (const { file } of images) {
        if (!file) continue;
        // 提交前再校验一遍单张大小（≤2MiB）与类型
        imageInputs.push(await foodPostImageFromFile(file));
      }
      const input = {
        venue_name: cleanedVenue,
        campus,
        tier,
        review_text: cleanedReview,
        price_reference: price.trim(),
        hours_reference: hours.trim(),
        dishes: dishes
          .map((dish) => ({
            name: dish.name.trim(),
            price: dish.price.trim(),
            reason: dish.reason.trim(),
          }))
          .filter((dish) => dish.name !== ""),
        images: imageInputs,
      };
      const response = initial
        ? await updateFoodPost(
            initial.id,
            {
              ...input,
              expected_version: initial.version,
              retained_image_positions: images.flatMap((image) =>
                image.position === undefined ? [] : [image.position],
              ),
            },
            idempotencyKey,
          )
        : await createFoodPost(input, idempotencyKey);
      submitKeyRef.current = null;
      if (initial) rememberUpdatedFoodPost(response.post);
      else {
        if (userID) clearFoodDraft(userID);
        rememberCreatedFoodPost(response.post);
      }
      router.push(`/food/post/${response.post.id}`);
    } catch (err) {
      if (initial && err instanceof PortalUnauthorizedError) {
        setSessionExpired(true);
        setError("登录已过期，修改仍保留在本页。请重新登录后再编辑。");
        return;
      }
      if (
        initial &&
        err instanceof PortalHttpError &&
        (err.status === 404 || err.status === 409)
      ) {
        setConflict(true);
        setError(
          err.errorCode === "VERSION_CONFLICT"
            ? "投稿已被更新，当前修改尚未保存。请先重新加载最新内容，再确认修改。"
            : err.errorCode === "IDEMPOTENCY_KEY_CONFLICT"
              ? "保存请求已发生变化，当前修改尚未保存。请重新加载后确认。"
              : "这条投稿目前无法编辑，当前修改尚未保存。可以重新加载查看状态。",
        );
        return;
      }
      if (err instanceof PortalUnauthorizedError) {
        if (!userID || !storeFoodDraft(userID, { venue, campus, tier, review, price, hours, dishes, imageCount: images.length }, images)) {
          setSessionExpired(true);
          setError("登录已过期，草稿无法暂存。请先复制已填写的内容，再重新登录。");
          return;
        }
        // 填表期间会话过期：回登录页，登录后回到表单
        router.replace(`/account/login?next=${encodeURIComponent(returnPath)}`);
        return;
      }
      if (isFoodPostDailyCapError(err)) {
        setError(foodPostDailyCapMessage());
        return;
      }
      if (isFoodPostPlaceholderError(err)) {
        setError(foodPostPlaceholderMessage());
        return;
      }
      if (err instanceof PortalApiError) {
        setError("提交失败，请稍后重试。");
        return;
      }
      setError(err instanceof Error ? err.message : "提交失败，请稍后重试。");
    } finally {
      busyRef.current = false;
      setPending(false);
    }
  };

  return (
    <main className="mx-auto max-w-site px-5 py-10 md:px-8 md:py-14">
      <div className="grid gap-12 lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-16">
        <div>
          <p
            data-enter
            className="font-mono text-xs tracking-[0.3em] text-ink/60"
          >
            <span className="text-accent-text">F-03</span>
            <span className="mx-2">/</span>
            STUDENT FOOD DESK
          </p>
          <h1
            data-enter
            className="mt-5 max-w-[15ch] font-display text-5xl font-bold leading-[0.92] tracking-[-0.05em] md:text-7xl"
          >
            {initial ? "编辑我的投稿" : "你吃到的好店，投到这里。"}
          </h1>
          <p
            data-enter
            className="mt-6 max-w-[64ch] text-lg leading-8 text-ink/65"
          >
            {initial
              ? "修改店铺、文字和照片，保存后更新公开详情与五档榜。"
              : "写清店铺、校区与推荐理由，提交后立即公开到五档榜，同校区同学马上能看到。"}
          </p>

          <div
            data-enter
            className="mt-10 grid gap-px border border-line bg-line md:grid-cols-3"
          >
            {[
              ["01", "写清楚是哪家", "店名、校区和具体位置最重要。"],
              ["02", "讲明白为什么", "价格、分量、排队和推荐菜都可以。"],
              ["03", "提交即公开", "没有审核环节，帖子会立即进入五档榜。"],
            ].map(([index, title, copy]) => (
              <section key={index} className="bg-paper p-5">
                <p className="font-display text-4xl font-bold text-accent-text">
                  {index}
                </p>
                <h2 className="mt-4 font-display text-lg font-bold">{title}</h2>
                <p className="mt-2 text-sm leading-6 text-ink/60">{copy}</p>
              </section>
            ))}
          </div>

          <form
            data-enter
            className="mt-12 border-t border-ink"
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            {draftNotice && <p role="status" className="mb-4 text-sm text-ink/70">{draftNotice}</p>}
            <fieldset disabled={pending || imageReading} className="min-w-0">
              <div className="border-b border-line py-6 md:py-8">
                <label
                  htmlFor="food-venue"
                  className="mb-1 block font-mono text-xs text-ink/60"
                >
                  店铺名
                  <span aria-hidden className="text-accent-text">
                    *
                  </span>
                </label>
                <input
                  id="food-venue"
                  maxLength={160}
                  value={venue}
                  onChange={(event) => {
                    setVenue(event.target.value);
                    clearFieldError("venue");
                    noteEdit();
                  }}
                  placeholder="如：仁和食堂三楼 8 号窗口"
                  className="min-h-11 w-full border-b border-ink/30 bg-transparent py-2 text-lg font-medium outline-none placeholder:text-ink/60 focus:border-ink"
                />
                {fieldErrors.venue && (
                  <p className="mt-2 font-mono text-xs text-accent-text">
                    {fieldErrors.venue}
                  </p>
                )}
              </div>

              <fieldset className="border-b border-line py-6 md:py-8">
                <legend className="mb-3 font-mono text-xs text-ink/60">
                  校区
                  <span aria-hidden className="text-accent-text">
                    *
                  </span>
                </legend>
                <div className="flex flex-wrap gap-2">
                  {CAMPUS_KEYS.map((key) => (
                    <button
                      key={key}
                      type="button"
                      aria-pressed={campus === key}
                      onClick={() => {
                        setCampus(key);
                        clearFieldError("campus");
                        noteEdit();
                      }}
                      className={cn(
                        "min-h-11 border px-4 py-2 font-mono text-xs transition-colors",
                        campus === key
                          ? "border-ink bg-ink text-paper"
                          : "border-line text-ink/60 hover:border-ink/40",
                      )}
                    >
                      {CAMPUSES[key].name}
                    </button>
                  ))}
                </div>
                {fieldErrors.campus && (
                  <p className="mt-2 font-mono text-xs text-accent-text">
                    {fieldErrors.campus}
                  </p>
                )}
              </fieldset>

              <fieldset className="border-b border-line py-6 md:py-8">
                <legend className="mb-3 font-mono text-xs text-ink/60">
                  五档定位
                  <span aria-hidden className="text-accent-text">
                    *
                  </span>
                </legend>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
                  {FOOD_TIERS.map((option) => (
                    <button
                      key={option.key}
                      type="button"
                      aria-pressed={tier === option.key}
                      onClick={() => {
                        setTier(option.key);
                        clearFieldError("tier");
                        noteEdit();
                      }}
                      className={cn(
                        "border p-3 text-left transition-colors",
                        tier === option.key
                          ? "border-ink bg-ink text-paper"
                          : "border-line hover:border-ink/40",
                      )}
                    >
                      <span className="block font-display text-xl font-bold">
                        {option.label}
                      </span>
                      <span
                        className={cn(
                          "mt-1 block font-mono text-xs",
                          tier === option.key ? "text-paper/60" : "text-ink/60",
                        )}
                      >
                        {option.blurb}
                      </span>
                    </button>
                  ))}
                </div>
                {fieldErrors.tier && (
                  <p className="mt-2 font-mono text-xs text-accent-text">
                    {fieldErrors.tier}
                  </p>
                )}
              </fieldset>

              <div className="border-b border-line py-6 md:py-8">
                <label
                  htmlFor="food-review"
                  className="mb-1 block font-mono text-xs text-ink/60"
                >
                  锐评正文
                  <span aria-hidden className="text-accent-text">
                    *
                  </span>
                </label>
                <textarea
                  id="food-review"
                  maxLength={2000}
                  value={review}
                  onChange={(event) => {
                    setReview(event.target.value);
                    clearFieldError("review");
                    noteEdit();
                  }}
                  rows={5}
                  placeholder="味道、分量、性价比、排队情况和适合场景…"
                  className="w-full border border-ink/30 bg-transparent p-3 text-sm leading-6 outline-none placeholder:text-ink/60 focus:border-ink"
                />
                {fieldErrors.review ? (
                  <p className="mt-2 font-mono text-xs text-accent-text">
                    {fieldErrors.review}
                  </p>
                ) : (
                  <p className="mt-2 font-mono text-xs text-ink/60">
                    2–2000 字
                  </p>
                )}
              </div>

              <div className="grid gap-6 border-b border-line py-6 md:grid-cols-2 md:py-8">
                <div>
                  <label
                    htmlFor="food-price"
                    className="mb-1 block font-mono text-xs text-ink/60"
                  >
                    价格参考（可选）
                  </label>
                  <input
                    id="food-price"
                    maxLength={200}
                    value={price}
                    onChange={(event) => {
                      setPrice(event.target.value);
                      noteEdit();
                    }}
                    placeholder="如：人均 ¥18"
                    className="min-h-11 w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/60 focus:border-ink"
                  />
                </div>
                <div>
                  <label
                    htmlFor="food-hours"
                    className="mb-1 block font-mono text-xs text-ink/60"
                  >
                    营业参考（可选）
                  </label>
                  <input
                    id="food-hours"
                    maxLength={200}
                    value={hours}
                    onChange={(event) => {
                      setHours(event.target.value);
                      noteEdit();
                    }}
                    placeholder="如：11:00–21:00"
                    className="min-h-11 w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/60 focus:border-ink"
                  />
                </div>
              </div>

              <PostFormDishes dishes={dishes} addDish={addDish} updateDish={updateDish} removeDish={removeDish} />

              <PostFormImages images={images} addImage={addImage} removeImage={removeImage} />

              <div className="pt-6">
                {error && (
                  <p
                    role="alert"
                    className="border border-accent/50 bg-accent/5 px-3 py-2 font-mono text-xs leading-5 text-accent-text"
                  >
                    {error}
                  </p>
                )}
                <button
                  type="submit"
                  disabled={
                    pending || imageReading || conflict || sessionExpired
                  }
                  className="mt-4 border border-ink bg-ink px-8 py-3 font-mono text-sm text-paper transition-colors hover:bg-accent hover:text-ink disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {pending
                    ? "提交中…"
                    : imageReading
                      ? "图片读取中…"
                      : initial
                        ? "保存修改 →"
                        : "提交投稿 →"}
                </button>
              </div>
            </fieldset>
            {conflict && (
              <button
                type="button"
                onClick={onReload}
                className="mt-4 min-h-11 border border-ink px-4 py-2 text-sm"
              >
                放弃未保存修改并重新加载
              </button>
            )}
            {sessionExpired && (
              <Link
                href={`/account/login?next=${encodeURIComponent(returnPath)}`}
                className="mt-4 inline-flex min-h-11 items-center border border-ink px-4 py-2 text-sm"
              >
                重新登录
              </Link>
            )}
          </form>
        </div>

        <aside className="lg:sticky lg:top-24 lg:h-fit">
          <section className="border border-ink p-6">
            <p className="font-mono text-xs tracking-[0.25em] text-accent-text">
              MY POSTS
            </p>
            <h2 className="mt-3 font-display text-2xl font-bold">我的投稿</h2>
            <p className="mt-3 text-sm leading-6 text-ink/65">
              投稿直接发布到站内五档榜，提交后立即公开；想回顾自己发过的帖子，到“我的投稿”查看。
            </p>
            <Link
              href="/account/posts"
              className="mt-6 flex min-h-11 items-center justify-center bg-ink px-5 py-3 text-center font-mono text-xs text-paper transition-colors hover:bg-accent hover:text-ink"
            >
              查看我的投稿 →
            </Link>
            <Link
              href="/food"
              className="mt-3 flex min-h-11 items-center justify-center border border-ink px-5 py-3 text-center font-mono text-xs transition-colors hover:bg-ink hover:text-paper"
            >
              先看看五档榜
            </Link>
          </section>

          <section className="mt-5 border border-accent/50 bg-accent/5 p-5">
            <p className="font-mono text-xs text-accent-text">
              <span className="tracking-[0.22em]">PRIVACY</span> / 隐私提醒
            </p>
            <p className="mt-3 text-sm leading-6 text-ink/65">
              不要填写店主私人手机号、微信或学生个人信息；照片请使用自己的实拍或有授权的图片。
            </p>
          </section>
        </aside>
      </div>
    </main>
  );
}
