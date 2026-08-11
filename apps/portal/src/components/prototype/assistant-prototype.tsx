"use client";

// THROWAWAY PROTOTYPE: three assistant layouts, switchable via ?variant=,
// on /prototype/assistant. It has no production session, API, or mutation wiring.

import {
  AlertTriangle,
  ArrowRight,
  BookOpen,
  Bot,
  Check,
  ChevronDown,
  CircleHelp,
  ExternalLink,
  GraduationCap,
  LockKeyhole,
  Menu,
  MessageCircle,
  Minimize2,
  PanelRightOpen,
  RotateCcw,
  Send,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import PrototypeSwitcher, { type PrototypeVariant } from "./prototype-switcher";

type Surface = "portal" | "console";
type Scenario = "material" | "practice" | "degraded" | "external" | "confirmation" | "result";
type SurfaceState = { open: boolean; scenario: Scenario; note: string };

const SCENARIOS: Array<{ id: Scenario; label: string }> = [
  { id: "material", label: "资料卡" },
  { id: "practice", label: "刷题跳转" },
  { id: "degraded", label: "降级" },
  { id: "external", label: "外链" },
  { id: "confirmation", label: "写入确认" },
  { id: "result", label: "执行结果" },
];

const SURFACE_COPY = {
  portal: {
    assistant: "校园助手",
    principal: "Portal 会话 · 学生身份",
    trust: "只使用 Portal 允许的资料、通知与刷题入口",
    accent: "#ff4d00",
  },
  console: {
    assistant: "运营助手",
    principal: "Console 会话 · 运营身份",
    trust: "独立权限上下文 · 用户侧无法切换至此身份",
    accent: "#171717",
  },
} satisfies Record<Surface, { assistant: string; principal: string; trust: string; accent: string }>;

const INITIAL_STATE: Record<Surface, SurfaceState> = {
  portal: { open: true, scenario: "material", note: "资料检索结果保留在本次 Portal 会话中。" },
  console: { open: true, scenario: "confirmation", note: "Console 会话独立保存，不继承校园助手记录。" },
};

function readVariant(value: string | null): PrototypeVariant {
  return value === "B" || value === "C" ? value : "A";
}

function readSurface(value: string | null): Surface {
  return value === "console" ? "console" : "portal";
}

function SurfacePill({ surface }: { surface: Surface }) {
  const copy = SURFACE_COPY[surface];
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span
        className="grid size-8 shrink-0 place-items-center rounded-full text-white"
        style={{ background: copy.accent }}
        aria-hidden="true"
      >
        {surface === "portal" ? <GraduationCap size={16} /> : <ShieldCheck size={16} />}
      </span>
      <span className="min-w-0">
        <strong className="block truncate text-sm">{copy.assistant}</strong>
        <span className="block truncate text-[11px] text-current/55">{copy.principal}</span>
      </span>
    </div>
  );
}

function TrustStrip({ surface }: { surface: Surface }) {
  return (
    <div className="flex items-start gap-2 border-b border-current/10 bg-current/[0.035] px-4 py-2.5 text-xs leading-5">
      <LockKeyhole className="mt-0.5 shrink-0" size={14} aria-hidden="true" />
      <span>{SURFACE_COPY[surface].trust}</span>
    </div>
  );
}

function ScenarioPicker({ current, onSelect }: { current: Scenario; onSelect: (scenario: Scenario) => void }) {
  return (
    <div className="scrollbar-none flex gap-1 overflow-x-auto border-b border-current/10 px-3 py-2" aria-label="原型状态">
      {SCENARIOS.map((scenario) => (
        <button
          key={scenario.id}
          type="button"
          onClick={() => onSelect(scenario.id)}
          aria-pressed={current === scenario.id}
          className={`shrink-0 rounded-full px-3 py-1.5 text-xs transition ${
            current === scenario.id ? "bg-[#171717] text-white" : "bg-current/[0.055] hover:bg-current/10"
          }`}
        >
          {scenario.label}
        </button>
      ))}
    </div>
  );
}

function Provenance({ owner, freshness = "刚刚" }: { owner: string; freshness?: string }) {
  return (
    <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-current/10 pt-3 text-[11px] text-current/55">
      <span className="inline-flex items-center gap-1"><ShieldCheck size={12} />来源：{owner}</span>
      <span>更新时间：{freshness}</span>
      <span>非模型生成事实</span>
    </div>
  );
}

function ScenarioCard({
  scenario,
  surface,
  onScenario,
  onNote,
}: {
  scenario: Scenario;
  surface: Surface;
  onScenario: (scenario: Scenario) => void;
  onNote: (note: string) => void;
}) {
  if (scenario === "material") {
    return (
      <article className="rounded-2xl border border-current/15 bg-white p-4 text-[#171717] shadow-sm">
        <div className="flex items-start gap-3">
          <div className="grid size-11 shrink-0 place-items-center rounded-xl bg-[#fff0e8] text-[#d94200]"><BookOpen /></div>
          <div className="min-w-0">
            <p className="text-xs font-medium text-[#d94200]">资料结果 · 已审核公开资料</p>
            <h3 className="mt-1 font-semibold">高等数学 · 极限与连续复习提纲</h3>
            <p className="mt-1 text-sm leading-6 text-black/60">包含知识结构、典型例题与易错点。下载前仍由资料 Owner 验证当前发布状态。</p>
          </div>
        </div>
        <button type="button" onClick={() => onNote("已选择可信资料详情；真实产品将由静态 target registry 解析。")}
          className="mt-4 inline-flex min-h-10 items-center gap-2 rounded-lg bg-[#171717] px-4 text-sm font-medium text-white">
          查看资料详情 <ArrowRight size={15} />
        </button>
        <Provenance owner="Library（原型数据）" freshness="2026-08-11 14:30 CST" />
      </article>
    );
  }

  if (scenario === "practice") {
    return (
      <article className="rounded-2xl border border-current/15 bg-white p-4 text-[#171717] shadow-sm">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-xs font-medium text-indigo-600">可信站内跳转</p>
            <h3 className="mt-1 font-semibold">进入「马克思主义基本原理」练习</h3>
            <p className="mt-1 text-sm leading-6 text-black/60">目标由 HENUKit 注册表固定为 QuizCraft 题库入口；助手不能提供任意 URL。</p>
          </div>
          <Sparkles className="shrink-0 text-indigo-600" />
        </div>
        <div className="mt-4 rounded-xl bg-indigo-50 p-3 text-xs leading-5 text-indigo-950">
          题库版本 QC-MARX-2026.1 · 120 题 · 当前仅演示入口，不宣称实时题库已接通
        </div>
        <button type="button" onClick={() => onNote("目标键 practice.bank 已通过原型注册表校验；未发生真实跳转。")}
          className="mt-3 inline-flex min-h-10 items-center gap-2 rounded-lg bg-indigo-600 px-4 text-sm font-medium text-white">
          前往刷题 <ArrowRight size={15} />
        </button>
        <Provenance owner="QuizCraft（原型数据）" />
      </article>
    );
  }

  if (scenario === "degraded") {
    return (
      <article className="rounded-2xl border border-amber-300 bg-amber-50 p-4 text-amber-950">
        <div className="flex items-start gap-3">
          <AlertTriangle className="mt-0.5 shrink-0" />
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider">部分能力暂不可用</p>
            <h3 className="mt-1 font-semibold">资料 Owner 暂未返回结果</h3>
            <p className="mt-1 text-sm leading-6 text-amber-900/75">没有替换为示例资料，也不会把空响应当作“未找到”。已保留你的问题，可以稍后重试。</p>
          </div>
        </div>
        <button type="button" onClick={() => onNote("已保留原问题并模拟重试；没有调用真实服务。")}
          className="mt-4 inline-flex min-h-10 items-center gap-2 rounded-lg border border-amber-400 bg-white px-4 text-sm font-medium">
          <RotateCcw size={15} /> 保留问题并重试
        </button>
      </article>
    );
  }

  if (scenario === "external") {
    return (
      <article className="rounded-2xl border border-current/15 bg-white p-4 text-[#171717] shadow-sm">
        <p className="text-xs font-medium text-emerald-700">已登记的外部目标</p>
        <h3 className="mt-1 font-semibold">河南大学官网 · 校历</h3>
        <p className="mt-1 text-sm leading-6 text-black/60">下一步将离开 HENUKit。目标域名由 Gateway 注册表预先配置，不来自模型输出。</p>
        <div className="mt-3 flex items-center gap-2 rounded-xl bg-emerald-50 p-3 font-mono text-xs text-emerald-950">
          <ShieldCheck size={14} /> henu.edu.cn
        </div>
        <button type="button" onClick={() => onNote("已确认外部目标提示；原型不会打开新窗口。")}
          className="mt-3 inline-flex min-h-10 items-center gap-2 rounded-lg bg-emerald-700 px-4 text-sm font-medium text-white">
          继续前往外部网站 <ExternalLink size={15} />
        </button>
      </article>
    );
  }

  if (scenario === "confirmation") {
    const operation = surface === "portal" ? "将账户通知 N-1042 标为已读" : "将通知版本 NV-203 提交复核";
    return (
      <article className="rounded-2xl border-2 border-[#171717] bg-white p-4 text-[#171717] shadow-sm">
        <div className="flex items-start gap-3">
          <div className="grid size-10 shrink-0 place-items-center rounded-full bg-[#171717] text-white"><CircleHelp size={19} /></div>
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider">需要你的明确确认</p>
            <h3 className="mt-1 font-semibold">{operation}</h3>
            <p className="mt-1 text-sm leading-6 text-black/60">发送聊天文字“确认”不会执行。请核对对象、影响和数据 Owner 后，点击下方可信控件。</p>
          </div>
        </div>
        <dl className="mt-4 grid gap-2 rounded-xl bg-black/[0.045] p-3 text-xs sm:grid-cols-2">
          <div><dt className="text-black/50">执行方</dt><dd className="mt-0.5 font-medium">{surface === "portal" ? "Account Portfolio" : "Notice"}</dd></div>
          <div><dt className="text-black/50">确认有效期</dt><dd className="mt-0.5 font-medium">2 分钟 · 单次使用</dd></div>
          <div><dt className="text-black/50">影响</dt><dd className="mt-0.5 font-medium">改变真实状态（此处仅模拟）</dd></div>
          <div><dt className="text-black/50">审计</dt><dd className="mt-0.5 font-medium">记录请求、操作者与 Owner 结果</dd></div>
        </dl>
        <div className="mt-4 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <button type="button" onClick={() => onNote("操作已取消；没有产生 Owner 请求。")}
            className="min-h-11 rounded-lg border border-black/25 px-4 text-sm font-medium">取消</button>
          <button type="button" onClick={() => { onScenario("result"); onNote("已用可信按钮完成本地模拟；未调用任何真实 API。"); }}
            className="min-h-11 rounded-lg bg-[#171717] px-4 text-sm font-medium text-white">模拟确认并执行</button>
        </div>
        <p className="mt-3 text-center text-[11px] text-black/45">PROTOTYPE · 不执行真实操作 · 不代表 Console 写权限已获批准</p>
      </article>
    );
  }

  return (
    <article className="rounded-2xl border border-emerald-300 bg-emerald-50 p-4 text-emerald-950">
      <div className="flex items-start gap-3">
        <div className="grid size-10 shrink-0 place-items-center rounded-full bg-emerald-700 text-white"><Check size={19} /></div>
        <div>
          <p className="text-xs font-semibold uppercase tracking-wider">Owner 已确认 · 原型结果</p>
          <h3 className="mt-1 font-semibold">操作结果已经记录</h3>
          <p className="mt-1 text-sm leading-6 text-emerald-900/75">结果卡只展示 Owner 返回的最终状态；超时或结果未知时不会伪报成功。</p>
        </div>
      </div>
      <dl className="mt-4 grid gap-2 rounded-xl bg-white/70 p-3 text-xs sm:grid-cols-2">
        <div><dt className="text-emerald-900/55">结果</dt><dd className="mt-0.5 font-medium">succeeded（模拟）</dd></div>
        <div><dt className="text-emerald-900/55">请求 ID</dt><dd className="mt-0.5 font-mono">req_demo_311</dd></div>
        <div><dt className="text-emerald-900/55">幂等键</dt><dd className="mt-0.5 font-mono">idem_demo_once</dd></div>
        <div><dt className="text-emerald-900/55">完成时间</dt><dd className="mt-0.5 font-medium">刚刚</dd></div>
      </dl>
      <button type="button" onClick={() => onScenario("confirmation")}
        className="mt-4 inline-flex min-h-10 items-center gap-2 rounded-lg border border-emerald-400 bg-white px-4 text-sm font-medium">
        返回确认卡
      </button>
    </article>
  );
}

function ConversationBody({
  surface,
  state,
  onScenario,
  onNote,
}: {
  surface: Surface;
  state: SurfaceState;
  onScenario: (scenario: Scenario) => void;
  onNote: (note: string) => void;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ScenarioPicker current={state.scenario} onSelect={onScenario} />
      <div className="flex-1 space-y-4 overflow-y-auto p-4 sm:p-5">
        <div className="ml-auto max-w-[88%] rounded-2xl rounded-br-sm bg-[#171717] px-4 py-3 text-sm text-white">
          帮我找到可信资料，并说明来源；如果需要操作，请先让我确认。
        </div>
        <div className="max-w-[92%] rounded-2xl rounded-bl-sm bg-current/[0.055] px-4 py-3 text-sm leading-6">
          我会只展示已登记的数据 Owner 结果和可信目标。当前是 <strong>{SURFACE_COPY[surface].assistant}</strong>，不会切换或继承另一表面的权限。
        </div>
        <ScenarioCard scenario={state.scenario} surface={surface} onScenario={onScenario} onNote={onNote} />
        <p role="status" className="rounded-xl border border-dashed border-current/20 px-3 py-2 text-xs leading-5 text-current/60">
          当前上下文：{state.note}
        </p>
      </div>
      <div className="border-t border-current/10 p-3 pb-20 sm:pb-3">
        <div className="flex items-center gap-2 rounded-xl border border-current/15 bg-white p-2 text-[#171717]">
          <input
            aria-label="向助手发送消息"
            placeholder="输入问题（输入“确认”不会执行操作）"
            className="min-w-0 flex-1 bg-transparent px-2 py-1.5 text-sm outline-none"
          />
          <button type="button" onClick={() => onNote("消息输入仅演示界面；未连接 LangBot 或 Gateway。")}
            className="grid size-9 shrink-0 place-items-center rounded-lg bg-[#171717] text-white" aria-label="发送演示消息">
            <Send size={15} />
          </button>
        </div>
      </div>
    </div>
  );
}

function AssistantHeader({ surface, onClose, compact = false }: { surface: Surface; onClose: () => void; compact?: boolean }) {
  return (
    <header className={`flex items-center justify-between gap-3 border-b border-current/10 ${compact ? "px-3 py-2" : "px-4 py-3"}`}>
      <SurfacePill surface={surface} />
      <button type="button" onClick={onClose} className="grid size-9 shrink-0 place-items-center rounded-full hover:bg-current/10" aria-label="收起助手">
        {compact ? <ChevronDown size={17} /> : <Minimize2 size={17} />}
      </button>
    </header>
  );
}

function Launcher({ surface, onOpen, label = true }: { surface: Surface; onOpen: () => void; label?: boolean }) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="inline-flex min-h-12 items-center gap-2 rounded-full px-4 text-sm font-semibold text-white shadow-xl transition hover:-translate-y-0.5"
      style={{ background: SURFACE_COPY[surface].accent }}
    >
      <MessageCircle size={19} /> {label && SURFACE_COPY[surface].assistant}
    </button>
  );
}

function VariantA({ surface, state, update }: VariantProps) {
  return (
    <>
      {!state.open && <div className="fixed bottom-20 right-4 z-40 sm:right-7"><Launcher surface={surface} onOpen={() => update({ open: true })} /></div>}
      {state.open && (
        <div className="fixed inset-0 z-40 flex items-end justify-end bg-black/20 p-0 backdrop-blur-[2px] sm:items-stretch sm:bg-transparent sm:p-4 sm:pt-20 sm:backdrop-blur-none">
          <section className="flex h-[92dvh] w-full flex-col overflow-hidden rounded-t-3xl bg-[#f8f7f3] text-[#171717] shadow-2xl sm:h-full sm:max-w-[430px] sm:rounded-3xl sm:border sm:border-black/10" aria-label={`${SURFACE_COPY[surface].assistant}抽屉`}>
            <AssistantHeader surface={surface} onClose={() => update({ open: false })} />
            <TrustStrip surface={surface} />
            <ConversationBody surface={surface} state={state} onScenario={(scenario) => update({ scenario })} onNote={(note) => update({ note })} />
          </section>
        </div>
      )}
    </>
  );
}

function VariantB({ surface, state, update }: VariantProps) {
  return (
    <div className="mx-auto w-full max-w-6xl px-4 pb-28 pt-4 sm:px-7">
      <button
        type="button"
        onClick={() => update({ open: !state.open })}
        className="flex w-full items-center justify-between gap-3 rounded-2xl border border-current/15 bg-white px-4 py-3 text-left text-[#171717] shadow-sm"
      >
        <SurfacePill surface={surface} />
        <span className="hidden text-xs text-black/50 sm:block">上下文已保留 · 点击{state.open ? "收起" : "继续"}</span>
        <PanelRightOpen size={18} />
      </button>
      {state.open && (
        <section className="mt-3 grid min-h-[620px] overflow-hidden rounded-3xl border border-current/15 bg-[#faf9f6] text-[#171717] shadow-xl lg:grid-cols-[minmax(280px,0.72fr)_minmax(0,1.28fr)]" aria-label={`${SURFACE_COPY[surface].assistant}双栏`}>
          <div className="flex min-h-0 flex-col border-b border-black/10 lg:border-b-0 lg:border-r">
            <AssistantHeader surface={surface} onClose={() => update({ open: false })} compact />
            <TrustStrip surface={surface} />
            <div className="space-y-3 p-4 text-sm leading-6">
              <p className="rounded-2xl bg-black/[0.055] p-3">会话主题：资料与学习路径</p>
              <p className="rounded-2xl bg-black px-3 py-2.5 text-white">先找可信资料，再给我刷题入口。</p>
              <p className="text-xs text-black/55">这列保留对话脉络；右侧只呈现当前可信结果与动作。</p>
              <p role="status" className="rounded-xl border border-dashed border-black/20 p-3 text-xs text-black/60">{state.note}</p>
            </div>
            <div className="mt-auto border-t border-black/10 p-3">
              <div className="flex rounded-xl border border-black/15 bg-white p-2"><input className="min-w-0 flex-1 px-2 text-sm outline-none" placeholder="继续当前话题" /><Send size={16} /></div>
            </div>
          </div>
          <div className="flex min-h-0 flex-col">
            <ScenarioPicker current={state.scenario} onSelect={(scenario) => update({ scenario })} />
            <div className="flex-1 overflow-y-auto p-4 sm:p-6">
              <div className="mb-4 flex items-center justify-between gap-3"><div><p className="text-xs text-black/50">当前可信结果</p><h2 className="font-semibold">信息与下一步分开呈现</h2></div><Bot className="text-black/45" /></div>
              <ScenarioCard scenario={state.scenario} surface={surface} onScenario={(scenario) => update({ scenario })} onNote={(note) => update({ note })} />
            </div>
          </div>
        </section>
      )}
    </div>
  );
}

function VariantC({ surface, state, update }: VariantProps) {
  if (!state.open) {
    return <div className="grid min-h-[65vh] place-items-center px-4"><div className="text-center"><Bot className="mx-auto mb-4" size={40} /><h2 className="text-2xl font-semibold">继续你的独立助手会话</h2><p className="mt-2 text-sm text-current/55">上次上下文仍在当前表面内存中。</p><div className="mt-5"><Launcher surface={surface} onOpen={() => update({ open: true })} /></div></div></div>;
  }

  return (
    <section className="mx-auto grid min-h-[calc(100dvh-9rem)] w-full max-w-[1500px] gap-0 px-0 pb-24 pt-3 text-[#171717] lg:grid-cols-[260px_minmax(0,1fr)_300px] lg:px-5" aria-label={`${SURFACE_COPY[surface].assistant}上下文工作台`}>
      <aside className="hidden rounded-l-3xl border border-r-0 border-black/10 bg-[#171717] p-5 text-white lg:flex lg:flex-col">
        <SurfacePill surface={surface} />
        <p className="mt-8 text-xs uppercase tracking-[0.2em] text-white/45">会话上下文</p>
        <button className="mt-3 rounded-xl bg-white/10 p-3 text-left text-sm" type="button">资料与刷题路径<br /><span className="text-xs text-white/50">刚刚 · 6 条消息</span></button>
        <button className="mt-2 rounded-xl p-3 text-left text-sm text-white/55" type="button">校园网络帮助<br /><span className="text-xs">昨天 · 3 条消息</span></button>
        <p className="mt-auto text-xs leading-5 text-white/50">Portal 与 Console 各自维护独立会话；此列表不会跨表面出现。</p>
      </aside>
      <div className="flex min-h-[680px] min-w-0 flex-col border-y border-black/10 bg-[#faf9f6] lg:min-h-0">
        <AssistantHeader surface={surface} onClose={() => update({ open: false })} />
        <TrustStrip surface={surface} />
        <ConversationBody surface={surface} state={state} onScenario={(scenario) => update({ scenario })} onNote={(note) => update({ note })} />
      </div>
      <aside className="border border-black/10 bg-white p-4 lg:rounded-r-3xl">
        <p className="text-xs font-semibold uppercase tracking-[0.18em] text-black/45">信任检查器</p>
        <div className="mt-4 space-y-3 text-xs leading-5">
          <div className="rounded-xl bg-black/[0.045] p-3"><strong className="block">身份</strong><span className="text-black/55">{SURFACE_COPY[surface].principal}</span></div>
          <div className="rounded-xl bg-black/[0.045] p-3"><strong className="block">当前状态</strong><span className="text-black/55">{SCENARIOS.find((item) => item.id === state.scenario)?.label}</span></div>
          <div className="rounded-xl bg-black/[0.045] p-3"><strong className="block">执行边界</strong><span className="text-black/55">只允许静态工具与目标；原型没有后端连接。</span></div>
          <div className="rounded-xl border border-dashed border-black/20 p-3"><strong className="block">观察提示</strong><span className="text-black/55">这个方案把来源、身份和后果持续放在视野内，但占用更多页面空间。</span></div>
        </div>
      </aside>
    </section>
  );
}

type VariantProps = {
  surface: Surface;
  state: SurfaceState;
  update: (patch: Partial<SurfaceState>) => void;
};

function HostShell({ surface, children }: { surface: Surface; children: React.ReactNode }) {
  if (surface === "portal") {
    return (
      <div className="min-h-screen bg-paper text-ink">
        <header className="flex h-16 items-center justify-between border-b border-line px-5 sm:px-8">
          <span className="font-display text-xl font-bold">henukit<span className="text-accent">®</span></span>
          <nav className="hidden gap-6 font-mono text-xs tracking-widest sm:flex"><span>资料库</span><span>智能刷题</span><span>校园生活</span></nav>
          <span className="font-mono text-xs">PORTAL</span>
        </header>
        {children}
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[#f6f6f6] text-[#171717]">
      <header className="flex h-16 items-center gap-3 border-b border-black/10 bg-white px-4 sm:px-6">
        <Menu size={19} className="lg:hidden" />
        <span className="grid size-8 place-items-center rounded-lg bg-[#171717] text-xs font-semibold text-white">H</span>
        <span className="text-sm font-semibold">HENUKit Console</span>
        <span className="ml-auto rounded-full bg-emerald-50 px-3 py-1.5 text-xs font-medium text-emerald-800">权限已验证</span>
      </header>
      {children}
    </div>
  );
}

export default function AssistantPrototype() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const variant = readVariant(searchParams.get("variant"));
  const surface = readSurface(searchParams.get("surface"));
  const [surfaceStates, setSurfaceStates] = useState(INITIAL_STATE);

  const currentState = surfaceStates[surface];
  const status = useMemo(
    () => `variant=${variant} · surface=${surface} · panel=${currentState.open ? "open" : "closed"} · state=${currentState.scenario}`,
    [currentState.open, currentState.scenario, surface, variant],
  );

  function selectSurface(nextSurface: Surface) {
    const params = new URLSearchParams(searchParams.toString());
    params.set("surface", nextSurface);
    router.replace(`?${params.toString()}`, { scroll: false });
  }

  function update(patch: Partial<SurfaceState>) {
    setSurfaceStates((states) => ({ ...states, [surface]: { ...states[surface], ...patch } }));
  }

  const variantProps = { surface, state: currentState, update };

  return (
    <HostShell surface={surface}>
      <div className="sticky top-0 z-30 border-b border-current/10 bg-inherit/95 px-3 py-2 backdrop-blur sm:px-6">
        <div className="mx-auto flex max-w-[1500px] flex-wrap items-center justify-between gap-2">
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-current/45">Throwaway prototype · HC-311</p>
            <p className="hidden text-xs text-current/60 sm:block">不连接 Gateway、LangBot 或任何产品 Owner</p>
          </div>
          <div className="flex rounded-full border border-current/15 bg-white p-1 text-[#171717]" aria-label="选择助手表面">
            {(["portal", "console"] as const).map((item) => (
              <button key={item} type="button" onClick={() => selectSurface(item)} aria-pressed={surface === item}
                className={`rounded-full px-3 py-1.5 text-xs font-medium ${surface === item ? "bg-[#171717] text-white" : "text-black/55"}`}>
                {item === "portal" ? "Portal 校园助手" : "Console 运营助手"}
              </button>
            ))}
          </div>
          {process.env.NODE_ENV !== "production" && <code className="w-full text-center text-[10px] text-current/45 sm:w-auto">{status}</code>}
        </div>
      </div>

      {variant === "A" && <VariantA {...variantProps} />}
      {variant === "B" && <VariantB {...variantProps} />}
      {variant === "C" && <VariantC {...variantProps} />}

      <PrototypeSwitcher current={variant} />
    </HostShell>
  );
}
