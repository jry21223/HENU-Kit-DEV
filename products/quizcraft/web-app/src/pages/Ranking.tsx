import { Trophy } from 'lucide-react';

const Header = () => (
  <div className="mb-6 flex items-center gap-3">
    <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-yellow-400 to-amber-500 shadow-lg">
      <Trophy className="h-6 w-6 text-white" />
    </div>
    <div>
      <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100">排行榜</h1>
      <p className="text-sm text-gray-500 dark:text-slate-400">按答对题数排名</p>
    </div>
  </div>
);

export default function Ranking() {
  return <div className="mx-auto max-w-2xl animate-fade-in"><Header /><p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-700">排行榜正在升级中，预计很快恢复，请稍后再来</p></div>;
}
