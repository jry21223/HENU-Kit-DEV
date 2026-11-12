import type { TrainingClozeUnit } from "@/lib/api/generated/training";
import { INSET_FOCUS_RING, revealKeyboardFocus } from "@/lib/navigation/scroller-focus";

function blankNumber(unit: TrainingClozeUnit, blankID: string) {
 const questionID=unit.blanks.find(blank=>blank.blank_id===blankID)?.question_id;
 return unit.questions.findIndex(question=>question.question_id===questionID)+1;
}

export default function ClozePassage({ unit, activeQuestionID, selected, onSelect, evidenceParts = [] }: {
  unit: TrainingClozeUnit;
  activeQuestionID: string;
  selected: Record<string, number | undefined>;
  onSelect: (questionID: string) => void;
  evidenceParts?: string[];
}) {
  return <section aria-label="完形全文" className="mt-6 border-y border-line py-5 lg:sticky lg:top-24 lg:self-start">
    <h2 className="font-display text-xl font-bold">阅读全文</h2>
    <p className="mt-2 text-xs leading-6 text-ink/70">选择文中的空格或下方题号作答。字母表示你的选择，全文保留空格。</p>
    <div data-cloze-scroll tabIndex={0} onFocus={revealKeyboardFocus} aria-label="完形文章，可滚动阅读" className="mt-4 max-h-[45svh] lg:max-h-[calc(100svh-15rem)] overflow-y-auto overscroll-contain whitespace-pre-wrap break-words pr-2 text-base leading-9 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus-ring" lang="en">
      {unit.parts.map(part => {
        const highlighted = evidenceParts.includes(part.part_id);
        if (part.kind === "text") return <span key={part.part_id} id={`cloze-part-${part.part_id}`} className={highlighted ? "bg-accent/10 underline decoration-accent underline-offset-4" : undefined}>{part.text}</span>;
        const blank = unit.blanks.find(item => item.blank_id === part.blank_id);
        const number = blankNumber(unit,part.blank_id);
        const answer = blank ? selected[blank.question_id] : undefined;
        return <button key={part.part_id} id={`cloze-${part.blank_id}`} type="button" aria-label={`第 ${number} 空，${answer === undefined ? "未选择" : `已选择 ${String.fromCharCode(65 + answer)}`}`} aria-pressed={blank?.question_id === activeQuestionID} onClick={() => { if (blank) onSelect(blank.question_id); }} className={`mx-1 inline-flex min-h-11 min-w-14 items-center justify-center border-b-2 px-2 font-mono text-sm ${INSET_FOCUS_RING} ${blank?.question_id === activeQuestionID ? "border-accent text-accent-text" : "border-ink/50"} ${highlighted ? "bg-accent/10" : ""}`}>{number}{answer === undefined ? " ___" : ` (${String.fromCharCode(65 + answer)})`}</button>;
      })}
    </div>
  </section>;
}

export function ClozeEvidence({unit,partIDs}:{unit:TrainingClozeUnit;partIDs:string[]}) {
 return <p className="mt-2 whitespace-pre-wrap" lang="en">{unit.parts.filter(part=>partIDs.includes(part.part_id)).map(part=>part.kind==="text"?part.text:`[第 ${blankNumber(unit,part.blank_id)} 空]`).join("")}</p>;
}
