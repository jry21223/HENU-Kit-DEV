"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { gsap, useGSAP, FINE_MOTION, ScrollTrigger } from "@/lib/gsap";
import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";
import TiltCard from "@/components/ui/tilt-card";
import AmbientSvg from "@/components/ui/ambient-svg";
import {
  fetchLibraryCourses,
  fetchLibraryMaterials,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { CourseSummary, Material as ApiMaterial } from "@/lib/api/types";
import { STATIC_MATERIALS } from "@/lib/library/mock";
import {
  getLibraryGatewayError,
  getMaterials,
  initGateway,
} from "@/lib/library/gateway";
import { ErrorBanner } from "@/components/data-state";

const FEATURES = ["往年试卷持续更新", "学长笔记免费下载", "按课程与专业分类检索"];

type LibraryCard = { id: string; title: string; meta: string; size: string };

/** 档案卡按真实资料类型统计；数量为 0 的类型不占位展示。 */
const CARD_DEFS = [
  { type: "note", title: "学长笔记", meta: "PDF / 扫描版 / 按课程归档", unit: "份" },
  { type: "exam", title: "往年试卷", meta: "近五年真题 / 含答案", unit: "套" },
  { type: "mock", title: "模拟卷", meta: "教研组命制 / 考前自测", unit: "套" },
  { type: "lab", title: "实验报告", meta: "模板 + 优秀范例", unit: "份" },
  { type: "path", title: "学习路径", meta: "按专业整理 / 从入门到期末", unit: "条" },
] as const;

function buildCards(materials: ApiMaterial[], courses: CourseSummary[]): LibraryCard[] {
  const cards: LibraryCard[] = [];
  for (const def of CARD_DEFS) {
    const count = materials.filter((m) => m.type === def.type).length;
    if (count > 0) {
      cards.push({
        id: def.type.toUpperCase().slice(0, 2),
        title: def.title,
        meta: def.meta,
        size: `${count} ${def.unit}`,
      });
    }
  }
  if (courses.length > 0) {
    cards.push({
      id: "CU",
      title: "课程归档",
      meta: "按专业整理 / 从入门到期末",
      size: `${courses.length} 门`,
    });
  }
  return cards;
}

export default function SectionLibrary() {
  const sectionRef = useRef<HTMLElement>(null);
  const trackRef = useRef<HTMLDivElement>(null);
  const [cards, setCards] = useState<LibraryCard[] | null>(null);
  const [totalCount, setTotalCount] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      // 直接读取真实资料与课程列表；课程不可用不阻塞资料统计。
      const [materialsResp, coursesResp] = await Promise.all([
        fetchLibraryMaterials(),
        fetchLibraryCourses().catch(() => null),
      ]);
      setCards(buildCards(materialsResp.materials, coursesResp?.courses ?? []));
      setTotalCount(materialsResp.materials.length);
      return;
    } catch (e) {
      // 生产环境禁止静默回退 mock；只有允许 mock 的开发环境才走缓存/静态数据。
      try {
        await initGateway();
        const cached = getMaterials();
        if (cached.length > 0) {
          setCards(buildCards(cached, []));
          setTotalCount(cached.length);
          return;
        }
        if (mockAllowed) {
          setCards(buildCards(STATIC_MATERIALS, []));
          setTotalCount(STATIC_MATERIALS.length);
          return;
        }
        throw new Error(
          getLibraryGatewayError() || formatPortalError(e) || "资料库暂时加载不出来，请稍后刷新试试"
        );
      } catch (e2) {
        setCards([]);
        setError(e2 instanceof Error ? e2.message : "资料库暂时加载不出来，请稍后刷新试试");
      }
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const ready = cards !== null && !error;

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        const track = trackRef.current!;
        // 进入视觉中心后自动播放：卡片依次入场 → 缓慢往返巡检（非线性、循环）
        const entrance = gsap.from(gsap.utils.toArray("[data-lib-card]"), {
          x: 140,
          opacity: 0,
          duration: 0.8,
          ease: "power3.out",
          stagger: 0.1,
          paused: true,
        });
        const sweep = gsap.to(track, {
          x: () =>
            -Math.max(
              0,
              track.scrollWidth - (track.parentElement?.clientWidth ?? 0)
            ),
          duration: 9,
          ease: "sine.inOut",
          yoyo: true,
          repeat: -1,
          repeatDelay: 1.4,
          paused: true,
        });
        const play = () => {
          entrance.restart();
          sweep.restart();
        };
        ScrollTrigger.create({
          trigger: sectionRef.current,
          start: "top 60%",
          onEnter: play,
          onEnterBack: play,
        });
      });
      return () => mm.revert();
    },
    { scope: sectionRef, dependencies: [ready] }
  );

  return (
    <section
      ref={sectionRef}
      className="snap-screen relative border-t border-line bg-paper"
    >
      <div className="mx-auto flex min-h-svh max-w-7xl flex-col justify-center px-5 py-24 md:px-10">
        <div className="grid gap-8 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] md:items-end">
          <div>
            <SectionHeading index="01" en="LIBRARY" title="资料库" />
            <p className="mt-5 max-w-md text-sm leading-7 text-ink/70">
              别再满群求资料。往年试卷、学长笔记、实验报告，
              按课程归档，开箱即用。
            </p>
          </div>
          <div className="md:justify-self-end">
            <ul className="space-y-2 font-mono text-xs tracking-wider text-ink/60">
              {FEATURES.map((f) => (
                <li key={f}>
                  <span className="mr-2 text-accent">+</span>
                  {f}
                </li>
              ))}
            </ul>
            <MagneticButton href="/library" className="mt-6">
              进入模块
            </MagneticButton>
          </div>
        </div>

        {/* 档案卡轨道：自动循环巡检；reduced-motion 时回退为手动横滚 */}
        <div className="lib-track-wrap relative mt-12 overflow-hidden">
          {error ? (
            <ErrorBanner message={error} onRetry={load} />
          ) : (
            <div
              ref={trackRef}
              className="flex w-max gap-5 py-2 will-change-transform"
            >
              {cards === null
                ? // 加载占位：保持整屏布局稳定，不展示虚构数字
                  [0, 1, 2].map((i) => (
                    <div key={i} className="shrink-0">
                      <div className="flex h-72 w-56 flex-col justify-between border border-ink/25 bg-paper p-5">
                        <div className="flex items-start justify-between">
                          <span className="font-mono text-xs text-ink/30">--</span>
                          <span aria-hidden className="font-mono text-xs text-ink/20">+</span>
                        </div>
                        <div>
                          <div className="h-5 w-24 animate-pulse bg-ink/10" />
                          <div className="mt-3 h-3 w-40 animate-pulse bg-ink/10" />
                          <div className="mt-2 h-3 w-28 animate-pulse bg-ink/10" />
                        </div>
                      </div>
                    </div>
                  ))
                : cards.map((card) => (
                    <div key={card.id} data-lib-card className="shrink-0">
                      <TiltCard>
                        <article className="flex h-72 w-56 flex-col justify-between border border-ink/25 bg-paper p-5">
                          <div className="flex items-start justify-between">
                            <span className="font-mono text-xs text-accent">{card.id}</span>
                            <span aria-hidden className="font-mono text-xs text-ink/40">+</span>
                          </div>
                          <div>
                            <h3 className="font-display text-2xl font-bold">{card.title}</h3>
                            <p className="mt-3 border-t border-line pt-3 font-mono text-[10px] leading-5 tracking-wider text-ink/50">
                              {card.meta}
                              <br />
                              收录 {card.size}
                            </p>
                          </div>
                        </article>
                      </TiltCard>
                    </div>
                  ))}
              {cards !== null && cards.length > 0 ? (
                <div data-lib-card className="shrink-0">
                  <div className="flex h-72 w-40 items-center justify-center border border-dashed border-ink/30 font-mono text-xs tracking-widest text-ink/40">
                    持续收录中…
                  </div>
                </div>
              ) : null}
            </div>
          )}
        </div>

        <div className="mt-8">
          <div className="mb-4 flex items-center justify-between">
            <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
              AUTO-SCAN / 档案卡循环巡检中
            </p>
            <p className="hidden font-mono text-[10px] tracking-[0.3em] text-ink/40 md:block">
              {error
                ? "DATA SOURCE OFFLINE"
                : totalCount === null
                  ? "LOADING…"
                  : `${totalCount} FILES INDEXED`}
            </p>
          </div>
          <AmbientSvg variant="flow" className="text-ink/30" />
        </div>
      </div>
    </section>
  );
}
