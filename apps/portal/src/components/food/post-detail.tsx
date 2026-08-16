"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  fetchFoodPost,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { FoodComment, FoodPost } from "@/lib/api/types";
import { buildFoodVenueDetail } from "@/lib/food/detail";
import { CAMPUSES } from "@/lib/food/campuses";
import { foodStore } from "@/lib/food/mock";
import PostBlocks from "@/components/food/post-blocks";
import Img from "@/components/ui/img";
import {
  EmptyBlock,
  ErrorBanner,
  LoadingBlock,
} from "@/components/data-state";

const ShopMap = dynamic(() => import("@/components/food/map"), {
  ssr: false,
  loading: () => (
    <div className="bg-blueprint flex h-[280px] items-center justify-center border-t border-line">
      <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
        MAP LOADING…
      </p>
    </div>
  ),
});

type LoadState = "loading" | "ready" | "error" | "missing";

export default function PostDetail({ id }: { id: string }) {
  const [post, setPost] = useState<FoodPost | null>(null);
  const [comments, setComments] = useState<FoodComment[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const response = await fetchFoodPost(id);
      if (response.post.hidden) {
        setPost(null);
        setComments([]);
        setLoadState("missing");
        return;
      }
      setPost(response.post);
      setComments(response.comments);
      setLoadState("ready");
    } catch (cause) {
      if (mockAllowed) {
        const local = foodStore.get();
        const fallback = local.posts.find(
          (candidate) =>
            candidate.id === id &&
            (!candidate.hidden || candidate.isMine === true)
        );
        if (fallback) {
          setPost(fallback);
          setComments(local.comments.filter((comment) => comment.postId === id));
          setLoadState("ready");
          return;
        }
      }

      const message = formatPortalError(cause);
      if (
        typeof cause === "object" &&
        cause !== null &&
        "status" in cause &&
        cause.status === 404
      ) {
        setLoadState("missing");
        return;
      }
      setPost(null);
      setComments([]);
      setError(message || "美食详情暂时加载不出来，请稍后刷新试试。");
      setLoadState("error");
    }
  }, [id]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const detail = useMemo(
    () => (post ? buildFoodVenueDetail(post) : null),
    [post]
  );

  if (loadState === "loading") {
    return (
      <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8">
        <LoadingBlock label="加载商家档案" />
      </main>
    );
  }

  if (loadState === "error") {
    return (
      <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8">
        <ErrorBanner
          message={error ?? "美食详情暂时加载不出来，请稍后刷新试试。"}
          onRetry={() => void load()}
        />
      </main>
    );
  }

  if (loadState === "missing" || !post || !detail) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
          404 / NOT FOUND
        </p>
        <p className="mt-4 font-display text-2xl font-bold">
          商家档案不存在或已隐藏
        </p>
        <Link
          href="/food"
          className="mt-6 inline-block font-mono text-sm text-accent hover:underline"
        >
          ← 返回五档榜
        </Link>
      </main>
    );
  }

  const campus = CAMPUSES[post.campus];
  const tierLabel = detail.tier?.label ?? "未定档";

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8 md:py-14">
      <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_22rem] lg:gap-14">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2 font-mono text-[10px] tracking-[0.22em] text-ink/50">
            <span className="border border-accent px-2 py-1 text-accent">
              {tierLabel}
            </span>
            <span>{campus.name}</span>
            <span aria-hidden>/</span>
            <span>{post.time}</span>
          </div>

          <h1 className="mt-5 max-w-[18ch] font-display text-4xl font-bold leading-[0.95] tracking-[-0.04em] md:text-6xl">
            {post.shop.name || post.title}
          </h1>
          {post.shop.name !== post.title && (
            <p className="mt-4 max-w-[60ch] text-lg leading-8 text-ink/75">
              {post.title}
            </p>
          )}
          <p className="mt-4 max-w-[65ch] leading-7 text-ink/65">
            {post.excerpt}
          </p>

          <div className="mt-7 grid border-y border-ink md:grid-cols-3">
            {[
              {
                index: "01",
                label: "五档定位",
                value: detail.tier
                  ? `${detail.tier.label} · ${detail.tier.blurb}`
                  : "未定档 · 不进入五档榜",
              },
              {
                index: "02",
                label: "价格参考",
                value: detail.priceReference ?? "未填写",
              },
              {
                index: "03",
                label: "营业参考",
                value:
                  detail.hoursReference ?? "未填写 · 出发前请查地图",
              },
            ].map((item) => (
              <div
                key={item.index}
                className="border-b border-ink px-4 py-5 last:border-b-0 md:border-r md:border-b-0 md:last:border-r-0"
              >
                <p className="font-mono text-[10px] tracking-[0.22em] text-accent">
                  {item.index} / {item.label}
                </p>
                <p className="mt-3 text-sm leading-6">{item.value}</p>
              </div>
            ))}
          </div>

          {detail.gallery[0] && (
            <div className="mt-8">
              <Img
                src={detail.gallery[0]}
                alt={`${post.shop.name}参考图`}
                label="VENUE / REFERENCE"
                className="h-64 w-full md:h-[28rem]"
              />
            </div>
          )}

          <section className="mt-12">
            <p className="font-mono text-[10px] tracking-[0.28em] text-accent">
              WHY WE PICKED IT
            </p>
            <h2 className="mt-2 font-display text-3xl font-bold">
              为什么是这一档
            </h2>
            {detail.reasons.length ? (
              <ol className="mt-6 border-t border-line">
                {detail.reasons.map((reason, index) => (
                  <li
                    key={`${reason}-${index}`}
                    className="grid gap-3 border-b border-line py-4 sm:grid-cols-[3rem_1fr]"
                  >
                    <span className="font-display text-2xl font-bold text-accent">
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    <span className="leading-7 text-ink/75">{reason}</span>
                  </li>
                ))}
              </ol>
            ) : (
              <EmptyBlock label="投稿未附推荐理由" />
            )}
          </section>

          <section className="mt-12">
            <p className="font-mono text-[10px] tracking-[0.28em] text-accent">
              WHAT TO ORDER
            </p>
            <h2 className="mt-2 font-display text-3xl font-bold">推荐菜品</h2>
            {detail.dishes.length ? (
              <div className="mt-6 grid gap-px bg-line border border-line sm:grid-cols-2">
                {detail.dishes.map((dish, index) => (
                  <article key={`${dish.name}-${index}`} className="bg-paper p-5">
                    <p className="font-mono text-[10px] tracking-[0.2em] text-ink/40">
                      DISH {String(index + 1).padStart(2, "0")}
                    </p>
                    <h3 className="mt-2 font-display text-xl font-bold">
                      {dish.name}
                    </h3>
                    {dish.note && (
                      <p className="mt-2 text-sm leading-6 text-ink/65">
                        {dish.note}
                      </p>
                    )}
                  </article>
                ))}
              </div>
            ) : (
              <div className="mt-6">
                <EmptyBlock label="推荐菜品待补充" />
              </div>
            )}
          </section>

          <section className="mt-12">
            <p className="font-mono text-[10px] tracking-[0.28em] text-accent">
              IMAGE NOTES / THE VIBE
            </p>
            <h2 className="mt-2 font-display text-3xl font-bold">图片与环境</h2>
            {detail.gallery.length ? (
              <div className="mt-6 grid gap-3 sm:grid-cols-2">
                {detail.gallery.map((src, index) => (
                  <figure key={`${src}-${index}`}>
                    <Img
                      src={src}
                      alt={`${post.shop.name}稿件参考图 ${index + 1}`}
                      label={`PHOTO ${String(index + 1).padStart(2, "0")}`}
                      className="h-52 w-full"
                    />
                    <figcaption className="mt-2 font-mono text-[10px] text-ink/40">
                      稿件参考图 {String(index + 1).padStart(2, "0")}
                    </figcaption>
                  </figure>
                ))}
              </div>
            ) : (
              <div className="mt-6">
                <EmptyBlock label="图片与环境待补充" />
              </div>
            )}
          </section>

          <section className="mt-12 border-t border-ink pt-8">
            <p className="font-mono text-[10px] tracking-[0.28em] text-accent">
              FIELD NOTES
            </p>
            <h2 className="mt-2 font-display text-3xl font-bold">学生锐评</h2>
            <article className="mt-6 max-w-[70ch]">
              <PostBlocks blocks={post.blocks} />
            </article>
            <p className="mt-5 font-mono text-[10px] leading-5 text-ink/45">
              投稿人 / {post.author} · ▲ {post.likes} · ★ {post.stars}
            </p>
          </section>

          <section className="mt-12">
            <p className="font-mono text-[10px] tracking-[0.28em] text-accent">
              COMMENTS / {comments.length}
            </p>
            <h2 className="mt-2 font-display text-3xl font-bold">学生补充</h2>
            {comments.length ? (
              <ul className="mt-6 space-y-5 border-t border-line">
                {comments.map((comment) => (
                  <li
                    key={comment.id}
                    className="grid gap-2 border-b border-line py-5 sm:grid-cols-[10rem_1fr]"
                  >
                    <p className="font-mono text-[10px] leading-5 text-ink/45">
                      {comment.author}
                      <br />
                      {comment.time}
                    </p>
                    <p className="leading-7 text-ink/75">{comment.text}</p>
                  </li>
                ))}
              </ul>
            ) : (
              <div className="mt-6">
                <EmptyBlock label="暂无学生补充" />
              </div>
            )}
          </section>
        </div>

        <aside className="lg:sticky lg:top-24 lg:h-fit">
          <section className="border border-ink">
            <div className="p-5">
              <p className="font-mono text-[10px] tracking-[0.25em] text-accent">
                VENUE DOSSIER
              </p>
              <dl className="mt-5 space-y-5">
                <div>
                  <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">
                    地点
                  </dt>
                  <dd className="mt-1 text-sm leading-6">
                    {detail.location}
                    <span className="mt-1 block font-mono text-[10px] text-ink/45">
                      {detail.coordinates}
                    </span>
                  </dd>
                </div>
                <div>
                  <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">
                    资料状态
                  </dt>
                  <dd className="mt-1 text-sm leading-6">
                    社区稿件 · 投稿人提供
                  </dd>
                </div>
                <div>
                  <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">
                    数据来源
                  </dt>
                  <dd className="mt-1 text-sm leading-6">
                    {detail.source.author} · 社区稿件
                    <span className="mt-1 block font-mono text-[10px] text-ink/45">
                      发布于 {detail.source.publishedAt}
                    </span>
                  </dd>
                </div>
              </dl>
              <a
                href={detail.mapUrl}
                target="_blank"
                rel="noreferrer"
                className="mt-6 block border border-ink px-4 py-3 text-center font-mono text-xs tracking-[0.12em] transition-colors hover:bg-ink hover:text-paper"
              >
                在高德地图打开 ↗
              </a>
            </div>
            <ShopMap shop={post.shop} />
          </section>

          <section className="mt-5 border border-accent bg-accent/5 p-5">
            <p className="font-mono text-[10px] tracking-[0.25em] text-accent">
              STUDENT FOOD DESK
            </p>
            <h2 className="mt-3 font-display text-2xl font-bold">
              你也有一家想抬上榜？
            </h2>
            <p className="mt-3 text-sm leading-6 text-ink/65">
              提交后立即公开到五档榜，同校区同学马上能看到。
            </p>
            <Link
              href="/food/publish"
              className="mt-5 block bg-ink px-4 py-3 text-center font-mono text-xs tracking-[0.12em] text-paper transition-colors hover:bg-accent"
            >
              投稿一家好店 →
            </Link>
          </section>
        </aside>
      </div>

      <p className="mt-12 border-t border-line pt-5 font-mono text-[10px] leading-5 text-ink/45">
        图片可能为菜品或环境参考图；营业时间、价格与门店状态可能变化，请以地图和现场信息为准。
      </p>
    </main>
  );
}
