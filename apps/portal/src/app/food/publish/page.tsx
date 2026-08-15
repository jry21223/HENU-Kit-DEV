"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";

import { useReveal } from "@/components/account/use-reveal";
import Img from "@/components/ui/img";
import {
  PortalApiError,
  PortalUnauthorizedError,
} from "@/lib/api/client";
import { authStore } from "@/lib/auth/store";
import { cn } from "@/lib/cn";
import { CAMPUSES, CAMPUS_KEYS } from "@/lib/food/mock";
import { FOOD_TIERS, type FoodTierKey } from "@/lib/food/ranking";
import {
  createFoodPost,
  foodPostDailyCapMessage,
  foodPostImageFromFile,
  isFoodPostDailyCapError,
  type FoodPostCampus,
  type FoodPostImageInput,
} from "@/lib/food/submit";
import { fileToDataUrl } from "@/lib/image";

const MAX_DISHES = 6;
const MAX_IMAGES = 6;

interface DishDraft {
  name: string;
  price: string;
  reason: string;
}

interface ImageDraft {
  file: File;
  preview: string;
}

function commandKey(prefix: string): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `${prefix}:${crypto.randomUUID()}`;
  }
  return `${prefix}:${Date.now().toString(36)}:${Math.random().toString(36).slice(2)}`;
}

const EMPTY_DISH: DishDraft = { name: "", price: "", reason: "" };

interface FieldErrors {
  venue?: string;
  campus?: string;
  tier?: string;
  review?: string;
}

export default function FoodPublishPage() {
  const router = useRouter();
  const { user, ready } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer
  );
  useReveal();

  const [venue, setVenue] = useState("");
  const [campus, setCampus] = useState<FoodPostCampus | null>(null);
  const [tier, setTier] = useState<FoodTierKey | null>(null);
  const [review, setReview] = useState("");
  const [price, setPrice] = useState("");
  const [hours, setHours] = useState("");
  const [dishes, setDishes] = useState<DishDraft[]>([{ ...EMPTY_DISH }]);
  const [images, setImages] = useState<ImageDraft[]>([]);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const submitKeyRef = useRef<string | null>(null);

  // 守卫：未登录重定向（与 campus/publish 同模式）
  useEffect(() => {
    if (ready && !user) {
      router.replace(
        `/account/login?next=${encodeURIComponent("/food/publish")}`
      );
    }
  }, [ready, user, router]);

  if (!ready || !user) {
    return (
      <main className="flex min-h-[60vh] items-center justify-center">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
          AUTH CHECK<span className="animate-pulse text-accent">…</span>
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
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (images.length >= MAX_IMAGES) {
      setError(`最多上传 ${MAX_IMAGES} 张图片`);
      return;
    }
    try {
      // fileToDataUrl 校验类型与单张 ≤2MiB
      const preview = await fileToDataUrl(file);
      setImages((current) => [...current, { file, preview }]);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "图片读取失败");
    }
  };

  const removeImage = (index: number) => {
    setImages((current) => current.filter((_, i) => i !== index));
    noteEdit();
  };

  const updateDish = (index: number, patch: Partial<DishDraft>) => {
    setDishes((current) =>
      current.map((dish, i) => (i === index ? { ...dish, ...patch } : dish))
    );
    noteEdit();
  };

  const addDish = () => {
    if (dishes.length >= MAX_DISHES) return;
    setDishes((current) => [...current, { ...EMPTY_DISH }]);
  };

  const removeDish = (index: number) => {
    setDishes((current) => current.filter((_, i) => i !== index));
    noteEdit();
  };

  const submit = async () => {
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

    setPending(true);
    setError("");
    const idempotencyKey =
      submitKeyRef.current ?? commandKey("portal-food-post");
    submitKeyRef.current = idempotencyKey;

    try {
      const imageInputs: FoodPostImageInput[] = [];
      for (const { file } of images) {
        // 提交前再校验一遍单张大小（≤2MiB）与类型
        imageInputs.push(await foodPostImageFromFile(file));
      }
      const response = await createFoodPost(
        {
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
        },
        idempotencyKey
      );
      submitKeyRef.current = null;
      router.push(`/food/post/${response.post.id}`);
    } catch (err) {
      if (err instanceof PortalUnauthorizedError) {
        // 填表期间会话过期：回登录页，登录后回到表单
        router.replace(
          `/account/login?next=${encodeURIComponent("/food/publish")}`
        );
        return;
      }
      if (isFoodPostDailyCapError(err)) {
        setError(foodPostDailyCapMessage());
        return;
      }
      if (err instanceof PortalApiError) {
        setError("提交失败，请稍后重试。");
        return;
      }
      setError(err instanceof Error ? err.message : "提交失败，请稍后重试。");
    } finally {
      setPending(false);
    }
  };

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8 md:py-14">
      <div className="grid gap-12 lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-16">
        <div>
          <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/55">
            <span className="text-accent">F-03</span>
            <span className="mx-2">/</span>
            STUDENT FOOD DESK
          </p>
          <h1
            data-enter
            className="mt-5 max-w-[15ch] font-display text-5xl font-bold leading-[0.92] tracking-[-0.05em] md:text-7xl"
          >
            你吃到的好店，投到这里。
          </h1>
          <p data-enter className="mt-6 max-w-[64ch] text-lg leading-8 text-ink/65">
            写清店铺、校区与推荐理由，提交后立即公开到五档榜，同校区同学马上能看到。
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
                <p className="font-display text-4xl font-bold text-accent">
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
            <div className="border-b border-line py-6 md:py-8">
              <label
                htmlFor="food-venue"
                className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50"
              >
                店铺名<span aria-hidden className="text-accent">*</span>
              </label>
              <input
                id="food-venue"
                value={venue}
                onChange={(event) => {
                  setVenue(event.target.value);
                  clearFieldError("venue");
                  noteEdit();
                }}
                placeholder="如：仁和食堂三楼 8 号窗口"
                className="w-full border-b border-ink/30 bg-transparent py-2 text-lg font-medium outline-none placeholder:text-ink/30 focus:border-ink"
              />
              {fieldErrors.venue && (
                <p className="mt-2 font-mono text-[10px] text-accent">
                  {fieldErrors.venue}
                </p>
              )}
            </div>

            <fieldset className="border-b border-line py-6 md:py-8">
              <legend className="mb-3 font-mono text-[10px] tracking-[0.25em] text-ink/50">
                校区<span aria-hidden className="text-accent">*</span>
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
                      "border px-4 py-2 font-mono text-xs transition-colors",
                      campus === key
                        ? "border-ink bg-ink text-paper"
                        : "border-line text-ink/60 hover:border-ink/40"
                    )}
                  >
                    {CAMPUSES[key].name}
                  </button>
                ))}
              </div>
              {fieldErrors.campus && (
                <p className="mt-2 font-mono text-[10px] text-accent">
                  {fieldErrors.campus}
                </p>
              )}
            </fieldset>

            <fieldset className="border-b border-line py-6 md:py-8">
              <legend className="mb-3 font-mono text-[10px] tracking-[0.25em] text-ink/50">
                五档定位<span aria-hidden className="text-accent">*</span>
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
                        : "border-line hover:border-ink/40"
                    )}
                  >
                    <span className="block font-display text-xl font-bold">
                      {option.label}
                    </span>
                    <span
                      className={cn(
                        "mt-1 block font-mono text-[10px]",
                        tier === option.key ? "text-paper/60" : "text-ink/50"
                      )}
                    >
                      {option.blurb}
                    </span>
                  </button>
                ))}
              </div>
              {fieldErrors.tier && (
                <p className="mt-2 font-mono text-[10px] text-accent">
                  {fieldErrors.tier}
                </p>
              )}
            </fieldset>

            <div className="border-b border-line py-6 md:py-8">
              <label
                htmlFor="food-review"
                className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50"
              >
                锐评正文<span aria-hidden className="text-accent">*</span>
              </label>
              <textarea
                id="food-review"
                value={review}
                onChange={(event) => {
                  setReview(event.target.value);
                  clearFieldError("review");
                  noteEdit();
                }}
                rows={5}
                placeholder="味道、分量、性价比、排队情况和适合场景…"
                className="w-full border border-ink/30 bg-transparent p-3 text-sm leading-6 outline-none placeholder:text-ink/30 focus:border-ink"
              />
              {fieldErrors.review ? (
                <p className="mt-2 font-mono text-[10px] text-accent">
                  {fieldErrors.review}
                </p>
              ) : (
                <p className="mt-2 font-mono text-[10px] text-ink/40">
                  2–2000 字
                </p>
              )}
            </div>

            <div className="grid gap-6 border-b border-line py-6 md:grid-cols-2 md:py-8">
              <div>
                <label
                  htmlFor="food-price"
                  className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50"
                >
                  价格参考（可选）
                </label>
                <input
                  id="food-price"
                  value={price}
                  onChange={(event) => {
                    setPrice(event.target.value);
                    noteEdit();
                  }}
                  placeholder="如：人均 ¥18"
                  className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                />
              </div>
              <div>
                <label
                  htmlFor="food-hours"
                  className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50"
                >
                  营业参考（可选）
                </label>
                <input
                  id="food-hours"
                  value={hours}
                  onChange={(event) => {
                    setHours(event.target.value);
                    noteEdit();
                  }}
                  placeholder="如：11:00–21:00"
                  className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                />
              </div>
            </div>

            <section className="border-b border-line py-6 md:py-8">
              <div className="flex flex-wrap items-end justify-between gap-4">
                <div>
                  <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
                    推荐菜品（可选）
                  </p>
                  <p className="mt-1 font-mono text-[10px] text-ink/40">
                    至多 {MAX_DISHES} 道；菜名必填，价格与理由可留空。
                  </p>
                </div>
                {dishes.length < MAX_DISHES && (
                  <button
                    type="button"
                    onClick={addDish}
                    className="border border-line px-3 py-1.5 font-mono text-xs transition-colors hover:border-ink"
                  >
                    + 加一道
                  </button>
                )}
              </div>
              <div className="mt-4 space-y-3">
                {dishes.map((dish, index) => (
                  <div
                    key={index}
                    className="grid gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,2fr)_auto]"
                  >
                    <input
                      aria-label={`菜品 ${index + 1} 名称`}
                      value={dish.name}
                      onChange={(event) =>
                        updateDish(index, { name: event.target.value })
                      }
                      placeholder="菜名（必填）"
                      className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                    />
                    <input
                      aria-label={`菜品 ${index + 1} 价格`}
                      value={dish.price}
                      onChange={(event) =>
                        updateDish(index, { price: event.target.value })
                      }
                      placeholder="参考价格"
                      className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                    />
                    <input
                      aria-label={`菜品 ${index + 1} 理由`}
                      value={dish.reason}
                      onChange={(event) =>
                        updateDish(index, { reason: event.target.value })
                      }
                      placeholder="推荐理由"
                      className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                    />
                    <button
                      type="button"
                      onClick={() => removeDish(index)}
                      className="self-center px-2 py-1 font-mono text-xs text-ink/40 transition-colors hover:text-accent"
                      aria-label={`删除菜品 ${index + 1}`}
                    >
                      ×
                    </button>
                  </div>
                ))}
              </div>
            </section>

            <section className="border-b border-line py-6 md:py-8">
              <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
                图片（{images.length}/{MAX_IMAGES}，单张 ≤2MB，可选）
              </p>
              <div className="mt-4 flex flex-wrap items-start gap-3">
                {images.map((image, index) => (
                  <div key={index} className="relative">
                    <Img
                      src={image.preview}
                      alt={`图 ${index + 1}`}
                      label={`FIG.${index + 1}`}
                      className="h-20 w-28"
                    />
                    <button
                      type="button"
                      onClick={() => removeImage(index)}
                      className="absolute -right-2 -top-2 flex h-5 w-5 items-center justify-center border border-ink bg-paper font-mono text-[10px] hover:border-accent hover:text-accent"
                      aria-label={`删除图 ${index + 1}`}
                    >
                      ×
                    </button>
                  </div>
                ))}
                {images.length < MAX_IMAGES && (
                  <label className="flex h-20 w-28 cursor-pointer items-center justify-center border border-dashed border-ink/30 font-mono text-[10px] text-ink/40 transition-colors hover:border-ink hover:text-ink">
                    + 上传
                    <input
                      type="file"
                      accept="image/jpeg,image/png,image/webp"
                      className="hidden"
                      onChange={(event) => void addImage(event)}
                    />
                  </label>
                )}
              </div>
            </section>

            <div className="pt-6">
              {error && (
                <p
                  role="alert"
                  className="border border-accent/50 bg-accent/5 px-3 py-2 font-mono text-xs leading-5 text-accent"
                >
                  {error}
                </p>
              )}
              <button
                type="submit"
                disabled={pending}
                className="mt-4 border border-ink bg-ink px-8 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
              >
                {pending ? "提交中…" : "提交投稿 →"}
              </button>
            </div>
          </form>
        </div>

        <aside className="lg:sticky lg:top-24 lg:h-fit">
          <section className="border border-ink p-6">
            <p className="font-mono text-[10px] tracking-[0.25em] text-accent">
              MY POSTS
            </p>
            <h2 className="mt-3 font-display text-2xl font-bold">我的投稿</h2>
            <p className="mt-3 text-sm leading-6 text-ink/65">
              投稿直接发布到站内五档榜，提交后立即公开；想回顾自己发过的帖子，到“我的投稿”查看。
            </p>
            <Link
              href="/account/posts"
              className="mt-6 block bg-ink px-5 py-3 text-center font-mono text-xs tracking-[0.12em] text-paper transition-colors hover:bg-accent"
            >
              查看我的投稿 →
            </Link>
            <Link
              href="/food"
              className="mt-3 block border border-ink px-5 py-3 text-center font-mono text-xs tracking-[0.12em] transition-colors hover:bg-ink hover:text-paper"
            >
              先看看五档榜
            </Link>
          </section>

          <section className="mt-5 border border-accent/50 bg-accent/5 p-5">
            <p className="font-mono text-[10px] tracking-[0.22em] text-accent">
              PRIVACY / 隐私提醒
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
