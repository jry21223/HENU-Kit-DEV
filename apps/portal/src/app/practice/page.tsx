"use client";

import { useEffect, useMemo, useState } from "react";
import { SCHOOLS, QuizListMeta } from "@/lib/practice/mock";
import { hasGateway } from "@/lib/api/client";
import { getGatewaySchools } from "@/lib/practice/gateway";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import TransitionLink from "@/components/practice/transition/transition-link";
import BankHero from "@/components/practice/bank-hero";
import BankFilter from "@/components/practice/bank-filter";

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

export default function PracticeBankPage() {
  usePageEnter(null);

  const [schools, setSchools] = useState(SCHOOLS);
  const [schoolId, setSchoolId] = useState(SCHOOLS[0].id);
  const [majorId, setMajorId] = useState(SCHOOLS[0].majors[0].id);
  const [subjectId, setSubjectId] = useState(SCHOOLS[0].majors[0].subjects[0].id);
  const [query, setQuery] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);

  useEffect(() => {
    if (!hasGateway) return;
    const gw = getGatewaySchools();
    if (gw) setSchools(gw as typeof SCHOOLS);
  }, []);

  const school = schools.find((s) => s.id === schoolId)!;
  const major = school.majors.find((m) => m.id === majorId)!;
  const subject = major.subjects.find((s) => s.id === subjectId)!;

  const searching = query.trim().length > 0;
  const searchHits = useMemo(() => {
    if (!searching) return [];
    const q = query.trim();
    return schools.flatMap((s) =>
      s.majors.flatMap((m) =>
        m.subjects
          .filter((sub) => sub.name.includes(q))
          .map((sub) => ({ subject: sub, school: s.name, major: m.name }))
      )
    );
  }, [query, searching, schools]);

  return (
    <main>
      {/* Hero：标题 / 搜索 / 动态数据 / 3D 石膏头 */}
      <BankHero query={query} onQueryChange={setQuery} />

      {/* 分隔：1px 结构线 + mono 编号 */}
      <div data-block className="border-t border-line">
        <div className="mx-auto flex max-w-[1440px] items-center justify-between px-5 py-3 md:px-8">
          <p className="font-mono text-[10px] tracking-[0.3em] text-ink/50">
            <span className="text-accent">02</span>
            <span className="mx-2">/</span>
            BROWSE
          </p>
          <button
            type="button"
            onClick={() => setFilterOpen((v) => !v)}
            aria-expanded={filterOpen}
            className="border border-ink/30 px-3 py-1.5 font-mono text-xs tracking-widest lg:hidden"
          >
            筛选 {filterOpen ? "−" : "+"}
          </button>
        </div>
        {/* 移动端筛选面板 */}
        {filterOpen && !searching && (
          <div className="border-t border-line px-5 py-6 lg:hidden">
            <BankFilter
              schoolId={schoolId}
              majorId={majorId}
              subjectId={subjectId}
              onSchool={(id) => {
                setSchoolId(id);
                setMajorId(schools.find((s) => s.id === id)!.majors[0].id);
                setSubjectId(schools.find((s) => s.id === id)!.majors[0].subjects[0].id);
              }}
              onMajor={(id) => {
                setMajorId(id);
                setSubjectId(school.majors.find((m) => m.id === id)!.subjects[0].id);
              }}
              onSubject={setSubjectId}
            />
          </div>
        )}
      </div>

      <div className="mx-auto max-w-[1440px] lg:flex">
        {/* 桌面侧边栏 */}
        {!searching && (
          <aside data-block className="hidden w-72 shrink-0 border-r border-line lg:block">
            <div className="sticky top-14 px-6 py-10">
              <BankFilter
                schoolId={schoolId}
                majorId={majorId}
                subjectId={subjectId}
                onSchool={(id) => {
                  setSchoolId(id);
                  setMajorId(schools.find((s) => s.id === id)!.majors[0].id);
                  setSubjectId(schools.find((s) => s.id === id)!.majors[0].subjects[0].id);
                }}
                onMajor={(id) => {
                  setMajorId(id);
                  setSubjectId(school.majors.find((m) => m.id === id)!.subjects[0].id);
                }}
                onSubject={setSubjectId}
              />
            </div>
          </aside>
        )}

        {/* 结果区 */}
        <div data-block className="flex-1 px-5 py-10 md:px-8">
          {!searching ? (
            <>
              <p data-enter className="mb-5 font-mono text-[10px] tracking-[0.3em] text-ink/40">
                {school.name} / {major.name} / {subject.name} · {subject.lists.length} 个题单
              </p>
              <div data-enter className="grid gap-5 md:grid-cols-2">
                {subject.lists.map((list, i) => (
                  <ListCard key={list.id} list={list} index={i} />
                ))}
              </div>
            </>
          ) : searchHits.length === 0 ? (
            <p data-enter className="border border-dashed border-ink/30 px-5 py-12 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
              无匹配科目 / NO RESULT
            </p>
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
          )}
        </div>
      </div>
    </main>
  );
}
