import type { TrainingAttribution } from "@/lib/api/generated/training";

const labels = { audio: "音频", text: "文字", translation: "译文" };

export default function TrainingAttributionList({ records }: { records: TrainingAttribution[] }) {
  return (
    <details className="mt-6 border-t border-line pt-4 text-xs leading-6 text-ink/70">
      <summary className="min-h-11 cursor-pointer py-2 font-mono focus-visible:outline-2 focus-visible:outline-focus-ring">来源与许可</summary>
      <ul className="space-y-4 break-words">
        {records.map(record => (
          <li key={record.component}>
            <p>{labels[record.component]}：{record.attribution}</p>
            <p>原创声明：{record.original ? "原创" : "非原创，按所列许可使用"}</p>
            <p><a href={record.source_url} target="_blank" rel="noopener noreferrer" className="underline underline-offset-4">来源</a> · <a href={record.license_url} target="_blank" rel="noopener noreferrer" className="underline underline-offset-4">{record.license_id}</a></p>
            {record.modifications && <p>改动：{record.modifications}</p>}
          </li>
        ))}
      </ul>
    </details>
  );
}
