import { useEffect, useState } from 'react';
import { Flag } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import type { FavoriteFolder } from '@/generated/quizcraft-api';
import {
  isQuizcraftAuthenticationError,
  shadowFavoritesApi,
  shadowPracticeApi,
} from '@/api/quizcraftShadowClient';
import { useQuizStore } from '@/stores/quizStore';

export default function Favorites() {
  const navigate = useNavigate();
  const { banks, setBanks, setCurrentBank, startPractice } = useQuizStore();
  const [folders, setFolders] = useState<FavoriteFolder[]>([]);
  const [loading, setLoading] = useState(true);
  const [startingBankId, setStartingBankId] = useState('');
  const [error, setError] = useState('');
  const [requiresLogin, setRequiresLogin] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([shadowPracticeApi.listBanks(), shadowFavoritesApi.overview()])
      .then(([bankResult, favoriteFolders]) => {
        if (cancelled) return;
        setBanks(bankResult.banks);
        setFolders(favoriteFolders);
        setRequiresLogin(false);
      })
      .catch((loadError) => {
        if (!cancelled) {
          const needsLogin = isQuizcraftAuthenticationError(loadError);
          setRequiresLogin(needsLogin);
          setError(needsLogin ? '收藏夹请在练习服务中查看' : '收藏夹暂时加载不出来，请检查网络后重试');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [setBanks]);

  const start = async (folder: FavoriteFolder) => {
    const bank = banks.find((item) => item.bank_id === folder.bank_id);
    if (!bank) {
      setError('这个题库当前不可用');
      return;
    }
    setStartingBankId(folder.bank_id);
    setError('');
    setRequiresLogin(false);
    try {
      const result = await shadowFavoritesApi.start(folder.bank_id, bank.key);
      setCurrentBank(bank.key);
      startPractice(result.questions, bank.key);
      navigate('/quiz');
    } catch (startError) {
      const needsLogin = isQuizcraftAuthenticationError(startError);
      setRequiresLogin(needsLogin);
      setError(needsLogin ? '请前往练习服务使用收藏' : '收藏练习创建失败，请稍后重试');
    } finally {
      setStartingBankId('');
    }
  };

  return (
    <div className="mx-auto max-w-2xl animate-fade-in">
      <h1 className="mb-6 flex items-center gap-2 text-2xl font-bold text-gray-800 dark:text-slate-100">
        <Flag className="h-6 w-6 text-yellow-500" />
        我的收藏
      </h1>
      {loading && <p className="text-sm text-gray-500">正在加载收藏夹…</p>}
      {error && <p role="alert" className="mb-4 rounded-xl bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      {requiresLogin && (
        <a className="mb-4 inline-flex rounded-xl bg-primary-500 px-4 py-2 text-sm font-medium text-white" href="https://henukit.cn/practice/favorites">
          前往练习服务
        </a>
      )}
      {!loading && !error && folders.length === 0 && (
        <p className="rounded-2xl border border-gray-100 bg-white p-6 text-sm text-gray-500 dark:border-slate-700 dark:bg-slate-800">还没有收藏题目。</p>
      )}
      <div className="space-y-3">
        {folders.map((folder) => (
          <section key={folder.bank_id} className="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800">
            <h2 className="font-semibold text-gray-800 dark:text-slate-100">{folder.bank_name}</h2>
            <p className="mt-2 text-sm text-gray-600 dark:text-slate-300">可练习 {folder.available_count} 题</p>
            {folder.unavailable_count > 0 && (
              <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">另有 {folder.unavailable_count} 题暂不可用</p>
            )}
            <button
              type="button"
              disabled={folder.available_count === 0 || startingBankId === folder.bank_id}
              onClick={() => void start(folder)}
              className="mt-4 rounded-xl bg-primary-500 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
            >
              {startingBankId === folder.bank_id ? '创建中…' : '练习这个题库'}
            </button>
          </section>
        ))}
      </div>
    </div>
  );
}
