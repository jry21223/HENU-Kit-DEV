import { ChevronRight, ListOrdered, Shuffle, Target } from 'lucide-react';
import type { PracticeMode, QuestionBank } from '@/types';
import { IS_OPS_MODE } from '@/config/appMode';

const PRACTICE_MODES = [
  { key: 'random' as PracticeMode, label: '随机模式', desc: '从题库中随机抽取题目', icon: Shuffle },
  { key: 'hard' as PracticeMode, label: '难题模式', desc: '专攻正确率低的题目', icon: Target },
  { key: 'chapter' as PracticeMode, label: '章节模式', desc: '按章节进行针对性练习', icon: ListOrdered },
];

type PracticeIdentityProps = {
  displayUserId: string;
  userIdInput: string;
  onUserIdChange: (value: string) => void;
};

export function PracticeIdentity({
  displayUserId,
  userIdInput,
  onUserIdChange,
}: PracticeIdentityProps) {
  if (!IS_OPS_MODE) {
    return (
      <div className="bg-white dark:bg-slate-800 rounded-xl border border-gray-100 dark:border-slate-700 p-4 mb-6">
        <div className="text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">排行榜将直接显示你的 ID</div>
        <p className="text-sm text-gray-600 dark:text-slate-300">
          {displayUserId ? `当前 ID：${displayUserId}` : '首次开始练习时会自动生成一个 ID。'}
        </p>
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-slate-800 rounded-xl border border-gray-100 dark:border-slate-700 p-4 mb-6">
      <label htmlFor="practice-user-id" className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">输入你的 ID</label>
      <input
        id="practice-user-id"
        type="text"
        value={userIdInput}
        onChange={(event) => onUserIdChange(event.target.value)}
        placeholder="请输入学号/工号/自定义ID"
        className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 bg-white dark:bg-slate-800 dark:text-slate-200 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent"
      />
      <p className="text-xs text-gray-500 dark:text-slate-400 mt-2">该 ID 会用于排行榜统计，可随时修改</p>
    </div>
  );
}

type PracticeBankSelectorProps = {
  banks: QuestionBank[];
  currentBank: string;
  onSelect: (bankKey: string) => void;
};

export function PracticeBankSelector({ banks, currentBank, onSelect }: PracticeBankSelectorProps) {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-xl border border-gray-100 dark:border-slate-700 p-5 mb-6">
      <div className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-3">选择题库</div>
      <div className="grid grid-cols-2 gap-3">
        {banks.map((bank) => (
          <button
            type="button"
            key={bank.key}
            onClick={() => onSelect(bank.key)}
            className={`p-3 rounded-lg border-2 text-left transition-all ${
              currentBank === bank.key
                ? 'border-primary-500 bg-primary-50 dark:bg-slate-500 dark:border-primary-400'
                : 'border-gray-100 dark:border-slate-600 dark:bg-slate-900 hover:border-gray-200 dark:hover:border-slate-500'
            }`}
          >
            <div className="font-medium text-gray-800 dark:text-slate-100">{bank.name}</div>
            <div className="text-xs text-gray-500 dark:text-slate-400 mt-1">{bank.total} 题</div>
          </button>
        ))}
      </div>
    </div>
  );
}

type PracticeModeSelectorProps = {
  mode: PracticeMode;
  onSelect: (mode: PracticeMode) => void;
};

export function PracticeModeSelector({ mode, onSelect }: PracticeModeSelectorProps) {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-xl border border-gray-100 dark:border-slate-700 p-5 mb-6">
      <div className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-3">练习模式</div>
      <div className="space-y-3">
        {PRACTICE_MODES.map((item) => (
          <button
            type="button"
            key={item.key}
            onClick={() => onSelect(item.key)}
            className={`w-full flex items-center gap-3 p-3 rounded-lg border-2 transition-all ${
              mode === item.key
                ? 'border-primary-500 bg-primary-50 dark:bg-slate-500 dark:border-primary-400'
                : 'border-gray-100 dark:border-slate-600 dark:bg-slate-900 hover:border-gray-200 dark:hover:border-slate-500'
            }`}
          >
            <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${
              mode === item.key ? 'bg-primary-500 dark:bg-primary-600 text-white' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400'
            }`}>
              <item.icon className="w-5 h-5" />
            </div>
            <div className="text-left flex-1">
              <div className="font-medium text-gray-800 dark:text-slate-100">{item.label}</div>
              <div className={`text-xs ${mode === item.key ? 'text-gray-500 dark:text-slate-300' : 'text-gray-500 dark:text-slate-400'}`}>{item.desc}</div>
            </div>
            <ChevronRight className={`w-5 h-5 ${mode === item.key ? 'text-primary-500 dark:text-primary-400' : 'text-gray-300'}`} />
          </button>
        ))}
      </div>
    </div>
  );
}

type PracticeSettingsProps = {
  mode: PracticeMode;
  count: number;
  unlimitedCount: boolean;
  chapterId: string;
  threshold: number;
  currentBank: QuestionBank | undefined;
  onCountChange: (count: number) => void;
  onUnlimitedCountChange: (value: boolean) => void;
  onChapterChange: (chapterId: string) => void;
  onThresholdChange: (threshold: number) => void;
};

export function PracticeSettings({
  mode,
  count,
  unlimitedCount,
  chapterId,
  threshold,
  currentBank,
  onCountChange,
  onUnlimitedCountChange,
  onChapterChange,
  onThresholdChange,
}: PracticeSettingsProps) {
  const countLabel = mode === 'random' && unlimitedCount ? '不限（全部）' : `${count} 道`;

  return (
    <div className="bg-white dark:bg-slate-800 rounded-xl border border-gray-100 dark:border-slate-700 p-5 mb-6">
      <div className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-3">设置</div>
      {mode !== 'chapter' && (
        <div className="mb-4">
          <div className="flex justify-between text-sm mb-2">
            <span className="text-gray-600 dark:text-slate-300">题目数量</span>
            <span className="font-medium text-gray-800 dark:text-slate-100">{countLabel}</span>
          </div>
          {mode === 'random' && (
            <button
              type="button"
              onClick={() => onUnlimitedCountChange(!unlimitedCount)}
              className={`mb-3 px-3 py-1.5 text-xs rounded-lg border transition-colors ${
                unlimitedCount
                  ? 'bg-primary-50 dark:bg-primary-900/20 border-primary-500 text-primary-700 dark:text-primary-300'
                  : 'bg-white dark:bg-slate-800 border-gray-200 dark:border-slate-600 text-gray-600 dark:text-slate-300 hover:border-gray-300 dark:hover:border-slate-500'
              }`}
            >
              {unlimitedCount ? '已启用不限（抽取全部）' : '启用不限（抽取全部）'}
            </button>
          )}
          <input
            aria-label="题目数量"
            type="range"
            min={5}
            max={mode === 'random' ? Math.max(5, currentBank?.total || 100) : 50}
            step={mode === 'random' ? 1 : 5}
            value={count}
            onChange={(event) => onCountChange(Number(event.target.value))}
            disabled={mode === 'random' && unlimitedCount}
            className="w-full h-2 bg-gray-200 dark:bg-slate-500 rounded-lg appearance-none cursor-pointer accent-primary-500"
          />
        </div>
      )}
      {mode === 'chapter' && currentBank && (
        <div className="mb-4">
          <label htmlFor="practice-chapter" className="block text-sm text-gray-600 dark:text-slate-300 mb-2">选择章节</label>
          <select
            id="practice-chapter"
            value={chapterId}
            onChange={(event) => onChapterChange(event.target.value)}
            className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 bg-white dark:bg-slate-800 dark:text-slate-200 rounded-lg focus:ring-2 focus:ring-primary-500"
          >
            <option value="">请选择章节</option>
            {currentBank.chapters.map((chapter) => <option key={chapter.id} value={chapter.id}>{chapter.name}</option>)}
          </select>
        </div>
      )}
      {mode === 'hard' && (
        <div>
          <div className="flex justify-between text-sm mb-2">
            <span className="text-gray-600 dark:text-slate-300">正确率阈值</span>
            <span className="font-medium text-gray-800 dark:text-slate-100">&lt; {threshold}%</span>
          </div>
          <input
            aria-label="正确率阈值"
            type="range"
            min={30}
            max={70}
            step={5}
            value={threshold}
            onChange={(event) => onThresholdChange(Number(event.target.value))}
            className="w-full h-2 bg-gray-200 dark:bg-slate-500 rounded-lg appearance-none cursor-pointer accent-primary-500"
          />
          <p className="text-xs text-gray-500 dark:text-slate-400 mt-1">只选择正确率低于 {threshold}% 的题目</p>
        </div>
      )}
    </div>
  );
}
