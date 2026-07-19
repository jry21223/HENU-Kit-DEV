import { useEffect, useRef, useState } from 'react';
import { Crown, Medal, Trophy, User } from 'lucide-react';
import { userApi } from '@/api/client';
import {
  QUIZCRAFT_GO_SHADOW_ENABLED,
  shadowPracticeApi,
  shadowRankingApi,
} from '@/api/quizcraftShadowClient';
import type { RankingPage } from '@/generated/quizcraft-api';
import type { RankItem, QuestionBank } from '@/types';

const getRankIcon = (index: number) => {
  if (index === 0) return <Crown className="h-5 w-5 text-yellow-500" />;
  if (index === 1) return <Medal className="h-5 w-5 text-gray-400" />;
  if (index === 2) return <Medal className="h-5 w-5 text-amber-600" />;
  return <span className="flex h-5 w-5 items-center justify-center text-sm text-gray-400">{index + 1}</span>;
};

const Header = () => (
  <div className="mb-6 flex items-center gap-3">
    <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-yellow-400 to-amber-500 shadow-lg">
      <Trophy className="h-6 w-6 text-white" />
    </div>
    <div>
      <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100">排行榜</h1>
      <p className="text-sm text-gray-500 dark:text-slate-400">按服务端确认的答对题数排名</p>
    </div>
  </div>
);

function ShadowRanking() {
  const [scope, setScope] = useState<'overall' | 'bank'>('overall');
  const [period, setPeriod] = useState<'weekly' | 'lifetime'>('weekly');
  const [banks, setBanks] = useState<QuestionBank[]>([]);
  const [bankId, setBankId] = useState('');
  const [page, setPage] = useState<RankingPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [nickname, setNickname] = useState('');
  const [avatar, setAvatar] = useState('scholar-blue');
  const [visible, setVisible] = useState(true);
  const [profileMessage, setProfileMessage] = useState('');
  const [profileSaving, setProfileSaving] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const requestVersionRef = useRef(0);

  useEffect(() => {
    void shadowPracticeApi.listBanks().then(({ banks: result }) => {
      setBanks(result);
      setBankId((current) => current || result[0]?.bank_id || '');
    }).catch(() => setError('题库暂时无法加载'));
  }, []);

  useEffect(() => {
    if (scope === 'bank' && !bankId) return;
    const requestVersion = ++requestVersionRef.current;
    setLoading(true);
    setError('');
    void shadowRankingApi.get(scope, period, bankId).then((result) => {
      if (requestVersion === requestVersionRef.current) setPage(result);
    }).catch(() => {
      if (requestVersion === requestVersionRef.current) {
        setPage(null);
        setError('排行榜暂时无法加载');
      }
    }).finally(() => {
      if (requestVersion === requestVersionRef.current) setLoading(false);
    });
  }, [scope, period, bankId, refreshVersion]);

  const saveProfile = async () => {
    if (profileSaving) return;
    setProfileSaving(true);
    setProfileMessage('');
    try {
      await shadowRankingApi.updateProfile({ visible, nickname: nickname.trim(), system_avatar: avatar });
      setProfileMessage(visible ? '公开资料已保存' : '已退出公开排行');
      setRefreshVersion((value) => value + 1);
    } catch {
      setProfileMessage('保存失败，请登录并检查昵称');
    } finally {
      setProfileSaving(false);
    }
  };

  return (
    <div className="mx-auto max-w-2xl animate-fade-in">
      <Header />
      <div className="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
        {(['overall', 'bank'] as const).map((value) => <button key={value} type="button" onClick={() => setScope(value)} className={`rounded-xl px-3 py-2 text-sm font-medium ${scope === value ? 'bg-primary-500 text-white' : 'bg-white text-gray-600 dark:bg-slate-800 dark:text-slate-300'}`}>{value === 'overall' ? '全站' : '按题库'}</button>)}
        {(['weekly', 'lifetime'] as const).map((value) => <button key={value} type="button" onClick={() => setPeriod(value)} className={`rounded-xl px-3 py-2 text-sm font-medium ${period === value ? 'bg-primary-500 text-white' : 'bg-white text-gray-600 dark:bg-slate-800 dark:text-slate-300'}`}>{value === 'weekly' ? '本周' : '历史'}</button>)}
      </div>
      {scope === 'bank' && <select aria-label="选择排行题库" value={bankId} onChange={(event) => setBankId(event.target.value)} className="mb-4 w-full rounded-xl border border-gray-200 bg-white p-3 dark:border-slate-700 dark:bg-slate-800">{banks.map((bank) => <option key={bank.bank_id} value={bank.bank_id}>{bank.name}</option>)}</select>}
      {error && <p role="alert" className="mb-4 rounded-xl bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      {loading ? <div className="h-20 animate-pulse rounded-xl bg-white dark:bg-slate-800" /> : page?.entries.length ? (
        <div className="space-y-3">{page.entries.map((entry, index) => <div key={`${entry.rank}-${entry.nickname}-${entry.system_avatar}-${index}`} className="flex items-center gap-4 rounded-xl border border-gray-100 bg-white p-4 dark:border-slate-700 dark:bg-slate-800"><div>{getRankIcon(entry.rank - 1)}</div><div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary-50 text-primary-500" title={entry.system_avatar}><User className="h-5 w-5" /></div><div className="min-w-0 flex-1"><p className="truncate font-medium text-gray-800 dark:text-slate-100">{entry.nickname}</p><p className="text-xs text-gray-400">系统头像：{entry.system_avatar}</p></div><div className="text-right"><p className="text-lg font-bold text-primary-600">{entry.correct_answer_count}</p><p className="text-xs text-gray-400">答对题数</p></div></div>)}</div>
      ) : <div className="rounded-xl bg-white py-10 text-center text-gray-500 dark:bg-slate-800">暂无公开排行数据</div>}

      <section className="mt-8 rounded-2xl border border-gray-100 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
        <h2 className="font-semibold text-gray-800 dark:text-slate-100">我的公开资料</h2>
        <input aria-label="公开昵称" value={nickname} maxLength={32} onChange={(event) => setNickname(event.target.value)} placeholder="1-32 字昵称" className="mt-3 w-full rounded-xl border border-gray-200 p-3 dark:border-slate-600 dark:bg-slate-900" />
        <select aria-label="系统头像" value={avatar} onChange={(event) => setAvatar(event.target.value)} className="mt-3 w-full rounded-xl border border-gray-200 p-3 dark:border-slate-600 dark:bg-slate-900"><option value="scholar-blue">蓝色学者</option><option value="coder-green">绿色代码</option><option value="reader-amber">琥珀读者</option><option value="owl-purple">紫色猫头鹰</option></select>
        <label className="mt-3 flex items-center gap-2 text-sm text-gray-600 dark:text-slate-300"><input type="checkbox" checked={visible} onChange={(event) => setVisible(event.target.checked)} />参与公开排行</label>
        <button type="button" disabled={profileSaving} onClick={() => void saveProfile()} className="mt-4 rounded-xl bg-primary-500 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-60">{profileSaving ? '保存中…' : '保存公开资料'}</button>
        {profileMessage && <p className="mt-2 text-sm text-gray-500">{profileMessage}</p>}
      </section>
    </div>
  );
}

function LegacyRanking() {
  const [ranking, setRanking] = useState<RankItem[]>([]);
  const [loading, setLoading] = useState(true);
  useEffect(() => { void userApi.getRanking().then((res) => setRanking(res.ranking)).catch(() => setRanking([])).finally(() => setLoading(false)); }, []);
  return <div className="mx-auto max-w-2xl animate-fade-in"><Header />{loading ? <div className="h-20 animate-pulse rounded-xl bg-white" /> : <div className="space-y-3">{ranking.map((item, index) => <div key={item.user_id} className="flex items-center gap-4 rounded-xl border bg-white p-4 dark:bg-slate-800"><div>{getRankIcon(index)}</div><div className="flex-1"><p className="font-medium">{item.user_id}</p><p className="text-sm text-gray-500">答对 {item.correct} / {item.total} 题</p></div><strong>{item.accuracy}%</strong></div>)}</div>}</div>;
}

export default function Ranking() {
  return QUIZCRAFT_GO_SHADOW_ENABLED ? <ShadowRanking /> : <LegacyRanking />;
}
