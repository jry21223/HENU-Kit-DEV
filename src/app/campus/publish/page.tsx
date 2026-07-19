"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { CATEGORIES, campusStore, ItemType } from "@/lib/campus/mock";
import Img from "@/components/ui/img";
import { fileToDataUrl } from "@/lib/image";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

function PublishForm() {
  const router = useRouter();
  const params = useSearchParams();
  const editId = params.get("edit");
  const { user, ready } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  const data = useSyncExternalStore(campusStore.subscribe, campusStore.get, campusStore.getServer);
  useReveal();

  const editItem = editId ? data.items.find((i) => i.id === editId) : undefined;

  // 编辑模式：首渲染惰性预填
  const [type, setType] = useState<ItemType>(() => editItem?.type ?? "help");
  const [category, setCategory] = useState(() => editItem?.category ?? "errand");
  const [title, setTitle] = useState(() => editItem?.title ?? "");
  const [desc, setDesc] = useState(() => editItem?.desc ?? "");
  const [price, setPrice] = useState(() => (editItem ? String(editItem.price) : ""));
  const [place, setPlace] = useState(() => editItem?.place ?? "");
  const [deadline, setDeadline] = useState(() => editItem?.deadline ?? "");
  const [images, setImages] = useState<string[]>(() => editItem?.images ?? []);
  const [error, setError] = useState("");

  // 守卫：未登录重定向
  useEffect(() => {
    if (ready && !user) {
      router.replace(`/account/login?next=${encodeURIComponent("/campus/publish")}`);
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

  const uploadImage = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    if (images.length >= 3) return setError("最多上传 3 张图片");
    try {
      const url = await fileToDataUrl(file);
      setImages((imgs) => [...imgs, url]);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "图片读取失败");
    }
  };

  const submit = () => {
    if (!title.trim()) return setError("请输入标题");
    if (!desc.trim()) return setError("请输入描述");
    const p = Number(price);
    if (!price || Number.isNaN(p) || p <= 0) return setError("请填写合法金额");
    if (!place.trim()) return setError("请填写位置");
    setError("");

    const payload = {
      type,
      category,
      title: title.trim(),
      desc: desc.trim(),
      price: Math.round(p),
      place: place.trim(),
      deadline: type === "help" ? deadline.trim() || undefined : undefined,
      seller: user.name,
      images: images.length ? images : undefined,
    };

    if (editItem) {
      campusStore.updateItem(editItem.id, payload);
      router.push(`/campus/item/${editItem.id}`);
    } else {
      const id = campusStore.publish(payload);
      router.push(`/campus/item/${id}`);
    }
  };

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <div className="max-w-3xl">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">M-03</span>
        <span className="mx-2">/</span>
        {editItem ? "EDIT" : "PUBLISH"}
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">
        {editItem ? "编辑单子" : "发布单子"}
      </h1>
      <p data-enter className="mt-3 border border-dashed border-ink/30 px-3 py-2 font-mono text-[10px] tracking-wider text-ink/50">
        发布后赏金由平台托管，确认完成后才结算给对方。
      </p>

      <div className="mt-8 space-y-6">
        {/* 类型 */}
        <div data-enter className="grid grid-cols-2 gap-3">
          {(["help", "sell"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => {
                setType(t);
                setCategory(t === "help" ? "errand" : "flea");
              }}
              className={cn(
                "border p-5 text-left transition-colors",
                type === t
                  ? t === "help"
                    ? "border-accent"
                    : "border-ink"
                  : "border-line hover:border-ink/40"
              )}
            >
              <p className={cn("font-display text-xl font-bold", type === t && t === "help" && "text-accent")}>
                {t === "help" ? "发求助单" : "出闲置"}
              </p>
              <p className="mt-1 font-mono text-[10px] text-ink/50">
                {t === "help" ? "悬赏赏金，找人帮忙" : "一口价转让闲置物品"}
              </p>
            </button>
          ))}
        </div>

        {/* 分类 */}
        <div data-enter>
          <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">分类</label>
          <div className="flex flex-wrap gap-2">
            {CATEGORIES.filter((c) => (type === "sell" ? c.key === "flea" : c.key !== "flea")).map((c) => (
              <button
                key={c.key}
                type="button"
                onClick={() => setCategory(c.key)}
                className={cn(
                  "border px-3 py-1.5 font-mono text-xs transition-colors",
                  category === c.key ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
                )}
              >
                {c.name}
              </button>
            ))}
          </div>
        </div>

        <div data-enter>
          <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">标题</label>
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={type === "help" ? "如：代取中通快递 3 件到 6 号楼" : "如：九成新机械键盘"}
            className="w-full border-b border-ink/30 bg-transparent py-2 text-lg font-medium outline-none placeholder:text-ink/30 focus:border-ink"
          />
        </div>

        <div data-enter>
          <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">描述</label>
          <textarea
            value={desc}
            onChange={(e) => setDesc(e.target.value)}
            rows={4}
            placeholder="具体要求 / 物品成色、交接方式…"
            className="w-full border border-ink/30 bg-transparent p-3 text-sm leading-6 outline-none placeholder:text-ink/30 focus:border-ink"
          />
        </div>

        <div data-enter>
          <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
            图片（{images.length}/3，≤2MB，可选）
          </label>
          <div className="flex flex-wrap items-start gap-3">
            {images.map((src, i) => (
              <div key={i} className="relative">
                <Img src={src} alt={`图 ${i + 1}`} label={`FIG.${i + 1}`} className="h-20 w-28" />
                <button
                  type="button"
                  onClick={() => setImages((imgs) => imgs.filter((_, j) => j !== i))}
                  className="absolute -right-2 -top-2 flex h-5 w-5 items-center justify-center border border-ink bg-paper font-mono text-[10px] hover:border-accent hover:text-accent"
                  aria-label={`删除图 ${i + 1}`}
                >
                  ×
                </button>
              </div>
            ))}
            {images.length < 3 && (
              <label className="flex h-20 w-28 cursor-pointer items-center justify-center border border-dashed border-ink/30 font-mono text-[10px] text-ink/40 transition-colors hover:border-ink hover:text-ink">
                + 上传
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  className="hidden"
                  onChange={uploadImage}
                />
              </label>
            )}
          </div>
        </div>

        <div data-enter className="grid gap-4 md:grid-cols-3">
          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              {type === "help" ? "赏金（元）" : "价格（元）"}
            </label>
            <input
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              inputMode="numeric"
              placeholder="3"
              className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
            />
          </div>
          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">位置</label>
            <input
              value={place}
              onChange={(e) => setPlace(e.target.value)}
              placeholder="明伦校区 · 西门"
              className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
            />
          </div>
          {type === "help" && (
            <div>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">时限（可选）</label>
              <input
                value={deadline}
                onChange={(e) => setDeadline(e.target.value)}
                placeholder="今天 18:00 前"
                className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
              />
            </div>
          )}
        </div>

        {error && <p className="font-mono text-xs text-accent">{error}</p>}
        <button
          type="button"
          onClick={submit}
          className="border border-ink bg-ink px-8 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
        >
          {editItem ? "保存修改 →" : "发布 →"}
        </button>
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
