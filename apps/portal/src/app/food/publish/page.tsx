"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import {
  CAMPUSES,
  CAMPUS_KEYS,
  CampusKey,
  foodStore,
  parseMiniMd,
  STATIC_POSTS,
} from "@/lib/food/mock";
import PostBlocks from "@/components/food/post-blocks";
import Img from "@/components/ui/img";
import { fileToDataUrl } from "@/lib/image";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

function PublishForm() {
  const router = useRouter();
  const params = useSearchParams();
  const editId = params.get("edit");
  const { user, ready } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  const data = useSyncExternalStore(foodStore.subscribe, foodStore.get, foodStore.getServer);
  useReveal();

  const editPost = editId ? data.posts.find((p) => p.id === editId) : undefined;

  // 编辑模式：状态在首渲染时惰性预填（store 为同步模块单例）
  const [title, setTitle] = useState(() => editPost?.title ?? "");
  const [campus, setCampus] = useState<CampusKey>(() => editPost?.campus ?? "minglun");
  const [shopName, setShopName] = useState(() => editPost?.shop.name ?? "");
  const [lat, setLat] = useState(() => (editPost ? String(editPost.shop.lat) : ""));
  const [lng, setLng] = useState(() => (editPost ? String(editPost.shop.lng) : ""));
  const [tags, setTags] = useState(() => editPost?.tags.join(" ") ?? "");
  // 正文 img 块在编辑模式还原为附件 + ![N] 标记
  const [initAttach] = useState(() => {
    const attachments: string[] = [];
    const body = editPost
      ? editPost.blocks
          .map((b) => {
            if (b.type === "img" && b.src) {
              attachments.push(b.src);
              return `![${attachments.length}]`;
            }
            return b.type === "h2"
              ? `# ${b.text}`
              : b.type === "quote"
                ? `> ${b.text}`
                : b.type === "list"
                  ? (b.items ?? []).map((i) => `- ${i}`).join("\n")
                  : b.text;
          })
          .join("\n\n")
      : "";
    return { attachments, body };
  });
  const [body, setBody] = useState(initAttach.body);
  const [attachments, setAttachments] = useState<string[]>(initAttach.attachments);
  const [cover, setCover] = useState(() => editPost?.images?.[0] ?? "");
  const [error, setError] = useState("");

  // 守卫：未登录重定向
  useEffect(() => {
    if (ready && !user) {
      router.replace(`/account/login?next=${encodeURIComponent("/food/publish")}`);
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

  // 预设 POI（来自既有文章的店家位置）
  const pois = STATIC_POSTS.filter((p) => p.campus === campus).map((p) => p.shop);

  const pickPoi = (name: string) => {
    const poi = pois.find((p) => p.name === name);
    if (!poi) return;
    setShopName(poi.name);
    setLat(String(poi.lat));
    setLng(String(poi.lng));
  };

  const blocks = parseMiniMd(body);

  const uploadFile = async (
    e: React.ChangeEvent<HTMLInputElement>,
    apply: (url: string) => void
  ) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    try {
      apply(await fileToDataUrl(file));
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "图片读取失败");
    }
  };

  const insertImage = (url: string) => {
    const n = attachments.length + 1;
    setAttachments((a) => [...a, url]);
    setBody((b) => `${b.trimEnd()}\n\n![${n}]\n`);
  };

  const submit = () => {
    if (!title.trim()) return setError("请输入标题");
    if (!shopName.trim()) return setError("请填写店名");
    const la = Number(lat);
    const ln = Number(lng);
    if (!lat || !lng || Number.isNaN(la) || Number.isNaN(ln)) return setError("请填写合法坐标（或从预设点位选择）");
    if (blocks.length === 0) return setError("正文不能为空");
    setError("");

    const payload = {
      campus,
      title: title.trim(),
      excerpt: (blocks.find((b) => b.type === "p")?.text ?? title.trim()).slice(0, 60),
      // 附件序号解析为真实 src 后存储
      blocks: blocks.map((b) =>
        b.type === "img" ? { type: "img" as const, src: attachments[(b.ref ?? 1) - 1] } : b
      ),
      author: user.name,
      tags: tags.trim() ? tags.trim().split(/\s+/).slice(0, 4) : ["锐评"],
      shop: { name: shopName.trim(), lat: la, lng: ln },
      isMine: true as const,
      images: cover ? [cover] : undefined,
    };

    if (editPost) {
      foodStore.updatePost(editPost.id, payload);
      router.push(`/food/post/${editPost.id}`);
    } else {
      const id = foodStore.addPost(payload);
      router.push(`/food/post/${id}`);
    }
  };

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">F-03</span>
        <span className="mx-2">/</span>
        {editPost ? "EDIT" : "PUBLISH"}
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">
        {editPost ? "编辑锐评" : "发布锐评"}
      </h1>

      <div className="mt-8 gap-10 lg:flex">
        {/* 表单 */}
        <div data-enter className="min-w-0 flex-1 space-y-5">
          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">标题</label>
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="一句话说清这家店"
              className="w-full border-b border-ink/30 bg-transparent py-2 text-lg font-medium outline-none placeholder:text-ink/30 focus:border-ink"
            />
          </div>

          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">校区</label>
            <div className="flex gap-2">
              {CAMPUS_KEYS.map((k) => (
                <button
                  key={k}
                  type="button"
                  onClick={() => setCampus(k)}
                  className={cn(
                    "border px-4 py-1.5 font-mono text-xs transition-colors",
                    campus === k ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
                  )}
                >
                  {CAMPUSES[k].name}
                </button>
              ))}
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            <div>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">店名</label>
              <input
                value={shopName}
                onChange={(e) => setShopName(e.target.value)}
                className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none focus:border-ink"
              />
            </div>
            <div>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">纬度 LAT</label>
              <input
                value={lat}
                onChange={(e) => setLat(e.target.value)}
                placeholder="34.8186"
                className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
              />
            </div>
            <div>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">经度 LNG</label>
              <input
                value={lng}
                onChange={(e) => setLng(e.target.value)}
                placeholder="114.3544"
                className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
              />
            </div>
          </div>

          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              预设点位（选择自动填入店名与坐标）
            </label>
            <div className="flex flex-wrap gap-2">
              {pois.map((p) => (
                <button
                  key={p.name}
                  type="button"
                  onClick={() => pickPoi(p.name)}
                  className="border border-line px-2.5 py-1 font-mono text-[10px] text-ink/60 transition-colors hover:border-accent hover:text-accent"
                >
                  {p.name}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              标签（空格分隔，最多 4 个）
            </label>
            <input
              value={tags}
              onChange={(e) => setTags(e.target.value)}
              placeholder="面食 西门 夯"
              className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
            />
          </div>

          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              封面图（可选，≤2MB）
            </label>
            <div className="flex items-start gap-3">
              {cover ? (
                <Img src={cover} alt="封面预览" label="COVER" className="h-20 w-32" />
              ) : (
                <div className="bg-blueprint flex h-20 w-32 items-center justify-center border border-dashed border-ink/30 font-mono text-[10px] text-ink/40">
                  无封面
                </div>
              )}
              <div className="space-y-2">
                <label className="inline-block cursor-pointer border border-ink/30 px-4 py-1.5 font-mono text-xs transition-colors hover:border-ink">
                  上传封面
                  <input
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    className="hidden"
                    onChange={(e) => uploadFile(e, setCover)}
                  />
                </label>
                {cover && (
                  <button
                    type="button"
                    onClick={() => setCover("")}
                    className="block font-mono text-[10px] text-ink/50 hover:text-accent"
                  >
                    移除封面
                  </button>
                )}
              </div>
            </div>
          </div>

          <div>
            <div className="mb-1 flex items-center justify-between">
              <label className="block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                正文 · mini-markdown（# 小标题 / - 列表 / &gt; 引用 / ![N] 插图）
              </label>
              <label className="cursor-pointer font-mono text-[10px] text-accent hover:underline">
                + 插入图片
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  className="hidden"
                  onChange={(e) => uploadFile(e, insertImage)}
                />
              </label>
            </div>
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              rows={10}
              placeholder={"先说结论……\n\n# 点什么\n- 招牌 ¥14\n> 锐评：打分夯。"}
              className="w-full border border-ink/30 bg-transparent p-3 font-mono text-sm leading-6 outline-none placeholder:text-ink/30 focus:border-ink"
            />
            {attachments.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-2">
                {attachments.map((a, i) => (
                  <span key={i} className="border border-line px-1.5 py-0.5 font-mono text-[10px] text-ink/50">
                    图{i + 1} 已插入
                  </span>
                ))}
              </div>
            )}
          </div>

          {error && <p className="font-mono text-xs text-accent">{error}</p>}
          <button
            type="button"
            onClick={submit}
            className="border border-ink bg-ink px-8 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
          >
            {editPost ? "保存修改 →" : "发布 →"}
          </button>
        </div>

        {/* 预览 */}
        <div data-enter className="mt-10 lg:mt-0 lg:w-[26rem] lg:shrink-0">
          <div className="border border-ink/25 p-5 lg:sticky lg:top-20">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">PREVIEW / 预览</p>
            <h2 className="mt-3 font-display text-xl font-bold">{title || "（标题）"}</h2>
            <div className="mt-4">
              {blocks.length ? (
                <PostBlocks blocks={blocks} attachments={attachments} />
              ) : (
                <p className="font-mono text-xs text-ink/30">正文预览…</p>
              )}
            </div>
          </div>
        </div>
      </div>
    </main>
  );
}

export default function PublishPage() {
  return (
    <Suspense>
      <PublishForm />
    </Suspense>
  );
}
