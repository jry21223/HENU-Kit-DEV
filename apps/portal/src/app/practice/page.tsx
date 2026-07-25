"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  fetchPracticeBanks,
  fetchPracticeSchools,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { BankSummary, PracticeSchool } from "@/lib/api/types";
import { SCHOOLS, QuizListMeta, type School } from "@/lib/practice/mock";
import {
  getGatewayBanks,
  getGatewaySchools,
  getPracticeGatewayError,
  initPracticeGateway,
} from "@/lib/practice/gateway";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import TransitionLink from "@/components/practice/transition/transition-link";
import BankHero from "@/components/practice/bank-hero";
import BankFilter from "@/components/practice/bank-filter";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";

function ListCard({ list, index }: { list: QuizListMeta; index: number }) {
  return (
    <TransitionLink
      href={`/practice/lists/${list.id}`}
      morph={{ kind: "list", id: list.id, title: list.name, sub: `${list.creator} · ${list.count} 题` }}
      className="group block border border-ink/25 bg-paper p-5 transition-colors hover:border-ink"
    >
      <div className="flex items-start justify-between">
        <span className="font-mono text-xs text-accent">
          L-{String(index + 1).padStart(2, "0")}
        </span>
        <span aria-hidden className="font-mono text-xs text-ink/40">+</span>
      </div>
      <h3 className="mt-3 font-display text-xl font-bold leading-snug group-hover:underline">
        {list.name}
      </h3>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {list.tags.map((t) => (
          <span key={t} className="border border-line px-1.5 py-0.5 font-mono text-[10px] text-ink/60">
            {t}
          </span>
        ))}
      </div>
      <div className="mt-4 border-t border-line pt-3 font-mono text-[10px] tracking-wider text-ink/50">
        {list.creator} · {list.count} 题
      </div>
      <div className="mt-3">
        <div className="mb-1 flex justify-between font-mono text-[10px] text-ink/50">
          <span>完成度</span>
          <span>{list.completion}%</span>
        </div>
        <div className="h-1 w-full bg-ink/10">
          <div className="h-full bg-accent" style={{ width: `${list.completion}%` }} />
        </div>
      </div>
    </TransitionLink>
  );
}

function BankCard({ bank, index }: { bank: BankSummary; index: number }) {
  return (
    <div className="group block border border-ink/25 bg-paper p-5">
      <div className="flex items-start justify-between">
        <span className="font-mono text-xs text-accent">
          B-{String(index + 1).padStart(2, "0")}
        </span>
      </div>
      <h3 className="mt-3 font-display text-xl font-bold leading-snug">{bank.name}</h3>
      <p className="mt-2 font-mono text-[10px] tracking-wider text-ink/50">
        {bank.subject} · {bank.question_count} 题
      </p>
    </div>
  );
}

function asLocalSchools(api: PracticeSchool[]): School[] {
  return api as unknown as School[];
}

type LoadState = "loading" | "ready" | "error";

export default function PracticeBankPage() {
  usePageEnter(null);

  const [schools, setSchools] = useState<School[]>([]);
  const [banks, setBanks] = useState<BankSummary[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);

  const [schoolId, setSchoolId] = useState("");
  const [majorId, setMajorId] = useState("");
  const [subjectId, setSubjectId] = useState("");
  const [query, setQuery] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      try {
        const resp = await fetchPracticeSchools();
        const next = asLocalSchools(resp.schools);
        setSchools(next);
        setBanks([]);
        if (next[0]) {
          setSchoolId(next[0].id);
          setMajorId(next[0].majors[0]?.id ?? "");
          setSubjectId(next[0].majors[0]?.subjects[0]?.id ?? "");
        }
        setLoadState("ready");
        return;
      } catch {
        // try banks, then gateway cache / mock
      }

      try {
        const banksResp = await fetchPracticeBanks();
        if (banksResp?.banks?.length) {
          setSchools([]);
          setBanks(banksResp.banks);
          setLoadState("ready");
          return;
        }
      } catch {
        /* continue */
      }

      await initPracticeGateway();
      const cachedSchools = getGatewaySchools();
      const cachedBanks = getGatewayBanks();
      if (cachedSchools?.length) {
        const next = asLocalSchools(cachedSchools);
        setSchools(next);
        setBanks([]);
        if (next[0]) {
          setSchoolId(next[0].id);
          setMajorId(next[0].majors[0]?.id ?? "");
          setSubjectId(next[0].majors[0]?.subjects[0]?.id ?? "");
        }
        setLoadState("ready");
        return;
      }
      if (cachedBanks?.length) {
        setSchools([]);
        setBanks(cachedBanks);
        setLoadState("ready");
        return;
      }
      if (mockAllowed) {
        setSchools(SCHOOLS);
        setBanks([]);
        setSchoolId(SCHOOLS[0].id);
        setMajorId(SCHOOLS[0].majors[0].id);
        setSubjectId(SCHOOLS[0].majors[0].subjects[0].id);
        setLoadState("ready");
        return;
      }
      setError(
        getPracticeGatewayError() ||
          "刷题接口不可用，生产环境已禁用 mock 回退。"
      );
      setLoadState("error");
    } catch (e) {
      if (mockAllowed) {
        setSchools(SCHOOLS);
        setSchoolId(SCHOOLS[0].id);
        setMajorId(SCHOOLS[0].majors[0].id);
        setSubjectId(SCHOOLS[0].majors[0].subjects[0].id);
        setLoadState("ready");
        return;
      }
      setError(formatPortalError(e));
      setLoadState("error");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const school = schools.find((s) => s.id === schoolId) ?? schools[0];
  const major = school?.majors.find((m) => m.id === majorId) ?? school?.majors[0];
  const subject =
    major?.subjects.find((s) => s.id === subjectId) ?? major?.subjects[0];

  const searching = query.trim().length > 0;
  const searchHits = useMemo(() => {
    if (!searching || schools.length === 0) return [];
    const q = query.trim();
    return schools.flatMap((s) =>
      s.majors.flatMap((m) =>
        m.subjects
          .filter((sub) => sub.name.includes(q))
          .map((sub) => ({ subject: sub, school: s.name, major: m.name }))
      )
    );
  }, [query, searching, schools]);

  const filteredBanks = useMemo(() => {
    if (!searching) return banks;
    const q = query.trim();
    return banks.filter(
      (b) => b.name.includes(q) || b.subject.includes(q)
    );
  }, [banks, query, searching]);

  const useHierarchy = schools.length > 0;

  return (
    <main>
      <BankHero query={query} onQueryChange={setQuery} />

      <div data-block className="border-t border-line">
        <div className="mx-auto flex max-w-[1440px] items-center justify-between px-5 py-3 md:px-8">
          <p className="font-mono text-[10px] tracking-[0.3em] text-ink/50">
            <span className="text-accent">02</span>
            <span className="mx-2">/</span>
            BROWSE
          </p>
          {useHierarchy && (
            <button
              type="button"
              onClick={() => setFilterOpen((v) => !v)}
              aria-expanded={filterOpen}
              className="border border-ink/30 px-3 py-1.5 font-mono text-xs tracking-widest lg:hidden"
            >
              筛选 {filterOpen ? "−" : "+"}
            </button>
          )}
        </div>
        {useHierarchy && filterOpen && !searching && school && major && (
          <div className="border-t border-line px-5 py-6 lg:hidden">
            <BankFilter
              schoolId={school.id}
              majorId={major.id}
              subjectId={subject?.id ?? ""}
              schools={schools}
              onSchool={(id) => {
                setSchoolId(id);
                const s = schools.find((x) => x.id === id)!;
                setMajorId(s.majors[0]?.id ?? "");
                setSubjectId(s.majors[0]?.subjects[0]?.id ?? "");
              }}
              onMajor={(id) => {
                setMajorId(id);
                setSubjectId(school.majors.find((m) => m.id === id)?.subjects[0]?.id ?? "");
              }}
              onSubject={setSubjectId}
            />
          </div>
        )}
      </div>

      <div className="mx-auto max-w-[1440px] px-5 pt-6 md:px-8">
        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mb-6" />
        )}
      </div>

      {loadState === "loading" ? (
        <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
          <LoadingBlock label="加载题库" />
        </div>
      ) : (
        <div className="mx-auto max-w-[1440px] lg:flex">
          {useHierarchy && !searching && school && major && (
            <aside data-block className="hidden w-72 shrink-0 border-r border-line lg:block">
              <div className="sticky top-14 px-6 py-10">
                <BankFilter
                  schoolId={school.id}
                  majorId={major.id}
                  subjectId={subject?.id ?? ""}
                  schools={schools}
                  onSchool={(id) => {
                    setSchoolId(id);
                    const s = schools.find((x) => x.id === id)!;
                    setMajorId(s.majors[0]?.id ?? "");
                    setSubjectId(s.majors[0]?.subjects[0]?.id ?? "");
                  }}
                  onMajor={(id) => {
                    setMajorId(id);
                    setSubjectId(school.majors.find((m) => m.id === id)?.subjects[0]?.id ?? "");
                  }}
                  onSubject={setSubjectId}
                />
              </div>
            </aside>
          )}

          <div data-block className="flex-1 px-5 py-10 md:px-8">
            {loadState === "error" ? (
              <EmptyBlock label="接口不可用" />
            ) : useHierarchy && school && major && subject ? (
              !searching ? (
                <>
                  <p data-enter className="mb-5 font-mono text-[10px] tracking-[0.3em] text-ink/40">
                    {school.name} / {major.name} / {subject.name} · {subject.lists.length} 个题单
                  </p>
                  {subject.lists.length === 0 ? (
                    <EmptyBlock label="暂无题单" />
                  ) : (
                    <div data-enter className="grid gap-5 md:grid-cols-2">
                      {subject.lists.map((list, i) => (
                        <ListCard key={list.id} list={list} index={i} />
                      ))}
                    </div>
                  )}
                </>
              ) : searchHits.length === 0 ? (
                <EmptyBlock label="无匹配科目" />
              ) : (
                searchHits.map(({ subject: sub, school: schoolName, major: majorName }) => (
                  <div key={sub.id} data-enter className="mb-10">
                    <p className="mb-4 font-mono text-[10px] tracking-[0.3em] text-ink/40">
                      {schoolName} / {majorName} / <span className="text-accent">{sub.name}</span>
                    </p>
                    <div className="grid gap-5 md:grid-cols-2">
                      {sub.lists.map((list, i) => (
                        <ListCard key={list.id} list={list} index={i} />
                      ))}
                    </div>
                  </div>
                ))
              )
            ) : banks.length > 0 ? (
              filteredBanks.length === 0 ? (
                <EmptyBlock label="无匹配题库" />
              ) : (
                <div data-enter className="grid gap-5 md:grid-cols-2">
                  {filteredBanks.map((b, i) => (
                    <BankCard key={b.id} bank={b} index={i} />
                  ))}
                </div>
              )
            ) : (
              <EmptyBlock label="暂无题库" />
            )}
          </div>
        </div>
      )}
    </main>
  );
}
