import { useEffect, useState } from 'react';
import { Trophy, Medal, User, Crown } from 'lucide-react';
import { userApi } from '@/api/client';
import type { RankItem } from '@/types';

const getRankIcon = (index: number) => {
  if (index === 0) return <Crown className="w-5 h-5 text-yellow-500" />;
  if (index === 1) return <Medal className="w-5 h-5 text-gray-400 dark:text-slate-500" />;
  if (index === 2) return <Medal className="w-5 h-5 text-amber-600" />;
  return <span className="w-5 h-5 flex items-center justify-center text-sm text-gray-400 dark:text-slate-500">{index + 1}</span>;
};

const getRankBg = (index: number) => {
  if (index === 0) return 'bg-gradient-to-r from-yellow-50 to-amber-50 border-yellow-200';
  if (index === 1) return 'bg-gradient-to-r from-gray-50 to-slate-50 border-gray-200 dark:border-slate-700';
  if (index === 2) return 'bg-gradient-to-r from-amber-50 to-orange-50 border-amber-200';
  return 'bg-white dark:bg-slate-800 border-gray-100 dark:border-slate-700';
};

const getRankText = (index: number) => {
  if (index < 3) {
    return {
      name: 'text-gray-800',
      detail: 'text-gray-500',
      accuracyLabel: 'text-gray-500',
      avatarBg: 'bg-gray-100',
      avatarIcon: 'text-gray-400',
    };
  }

  return {
    name: 'text-gray-800 dark:text-slate-100',
    detail: 'text-gray-500 dark:text-slate-400',
    accuracyLabel: 'text-gray-400 dark:text-slate-500',
    avatarBg: 'bg-gray-100 dark:bg-slate-700',
    avatarIcon: 'text-gray-400 dark:text-slate-500',
  };
};

export default function Ranking() {
  const [ranking, setRanking] = useState<RankItem[]>([]);
  const [loading, setLoading] = useState(true);
  
  useEffect(() => {
    userApi.getRanking()
      .then((res) => {
        setRanking(res.ranking);
        setLoading(false);
      })
      .catch(() => {
        setRanking([
          { rank: 1, userId: '2024001', name: '学霸小明', totalAnswers: 1580, correctRate: 94.5 },
          { rank: 2, userId: '2024002', name: '刷题达人', totalAnswers: 1320, correctRate: 89.2 },
          { rank: 3, userId: '2024003', name: '代码高手', totalAnswers: 1150, correctRate: 87.1 },
          { rank: 4, userId: '2024004', name: '努力学习', totalAnswers: 980, correctRate: 82.3 },
          { rank: 5, userId: '2024005', name: '张三', totalAnswers: 720, correctRate: 76.8 },
        ] as any);
        setLoading(false);
      });
  }, []);
  
  return (
    <div className="max-w-2xl mx-auto animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="w-12 h-12 bg-gradient-to-br from-yellow-400 to-amber-500 rounded-xl flex items-center justify-center shadow-lg">
          <Trophy className="w-6 h-6 text-white" />
        </div>
        <div>
          <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100">排行榜</h1>
          <p className="text-sm text-gray-500 dark:text-slate-400">看看谁是最强刷题王</p>
        </div>
      </div>
      
      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3, 4, 5].map((i) => (
            <div key={i} className="h-16 bg-white dark:bg-slate-800 rounded-xl animate-pulse" />
          ))}
        </div>
      ) : ranking.length > 0 ? (
        <div className="space-y-3">
          {ranking.map((item, index) => {
            const rankText = getRankText(index);

            return (
              <div
                key={item.user_id}
                className={`flex items-center gap-4 p-4 rounded-xl border-2 transition-all ${getRankBg(index)}`}
              >
                <div className="flex-shrink-0">
                  {getRankIcon(index)}
                </div>

                <div className={`flex-shrink-0 w-10 h-10 ${rankText.avatarBg} rounded-full flex items-center justify-center`}>
                  <User className={`w-5 h-5 ${rankText.avatarIcon}`} />
                </div>

                <div className="flex-1 min-w-0">
                  <div className={`font-medium ${rankText.name} truncate`}>
                    {item.user_id}
                  </div>
                  <div className={`text-sm ${rankText.detail}`}>
                    答对 {item.correct} / {item.total} 题
                  </div>
                </div>

                <div className="text-right">
                  <div className={`text-lg font-bold ${
                    item.accuracy >= 80 ? 'text-green-600' :
                    item.accuracy >= 60 ? 'text-blue-600' : 'text-gray-600 dark:text-slate-300'
                  }`}>
                    {item.accuracy}%
                  </div>
                  <div className={`text-xs ${rankText.accuracyLabel}`}>正确率</div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="text-center py-12 bg-white dark:bg-slate-800 rounded-xl border border-gray-100 dark:border-slate-700">
          <Trophy className="w-12 h-12 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 dark:text-slate-400">暂无排行榜数据</p>
          <p className="text-sm text-gray-400 dark:text-slate-500 mt-1">快来刷题成为第一名吧！</p>
        </div>
      )}
    </div>
  );
}
