import { SCHOOLS, type School } from "@/lib/practice/mock";
import { cn } from "@/lib/cn";

/**
 * 学院→专业→科目 分组筛选（侧边栏与移动面板共用）。
 * 选中项：橙色左边条 + 加粗；逻辑状态由父组件持有。
 * schools 可由 API 注入；缺省回退 mock（仅本地 dev allow-mock 场景）。
 */
export default function BankFilter({
  schoolId,
  majorId,
  subjectId,
  schools = SCHOOLS,
  onSchool,
  onMajor,
  onSubject,
}: {
  schoolId: string;
  majorId: string;
  subjectId: string;
  schools?: School[];
  onSchool: (id: string) => void;
  onMajor: (id: string) => void;
  onSubject: (id: string) => void;
}) {
  const list = schools.length > 0 ? schools : SCHOOLS;
  const school = list.find((s) => s.id === schoolId) ?? list[0];
  const major = school?.majors.find((m) => m.id === majorId) ?? school?.majors[0];

  const itemCls = (active: boolean) =>
    cn(
      "block w-full border-l-2 py-1.5 pl-3 text-left text-sm transition-colors",
      active
        ? "border-accent font-semibold text-ink"
        : "border-transparent text-ink/55 hover:border-ink/30 hover:text-ink"
    );

  if (!school || !major) {
    return (
      <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
        暂无筛选数据
      </p>
    );
  }

  return (
    <div className="space-y-7">
      <div>
        <p className="mb-2 font-mono text-[10px] tracking-[0.3em] text-ink/40">
          S / 学院
        </p>
        {list.map((s) => (
          <button key={s.id} type="button" onClick={() => onSchool(s.id)} className={itemCls(s.id === schoolId)}>
            {s.name}
          </button>
        ))}
      </div>
      <div>
        <p className="mb-2 font-mono text-[10px] tracking-[0.3em] text-ink/40">
          M / 专业
        </p>
        {school.majors.map((m) => (
          <button key={m.id} type="button" onClick={() => onMajor(m.id)} className={itemCls(m.id === majorId)}>
            {m.name}
          </button>
        ))}
      </div>
      <div>
        <p className="mb-2 font-mono text-[10px] tracking-[0.3em] text-ink/40">
          C / 科目
        </p>
        {major.subjects.map((sub) => (
          <button key={sub.id} type="button" onClick={() => onSubject(sub.id)} className={itemCls(sub.id === subjectId)}>
            {sub.name}
          </button>
        ))}
      </div>
    </div>
  );
}
