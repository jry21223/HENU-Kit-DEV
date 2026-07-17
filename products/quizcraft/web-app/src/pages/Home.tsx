import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { BookOpen, Zap, Target, BarChart3, ArrowRight, Brain } from 'lucide-react';
import { bankApi } from '@/api/client';
import { useQuizStore } from '@/stores/quizStore';

const FEATURES = [
  {
    icon: Zap,
    title: '智能刷题',
    desc: '随机、难题、章节三种模式，针对性练习',
    color: 'bg-yellow-100 text-yellow-700',
  },
  {
    icon: Target,
    title: '难题攻克',
    desc: '自动识别低正确率题目，重点突破',
    color: 'bg-red-100 text-red-700',
  },
  {
    icon: BarChart3,
    title: '数据统计',
    desc: '实时统计答题情况，追踪学习进度',
    color: 'bg-green-100 text-green-700',
  },
  {
    icon: Brain,
    title: 'AI 解析',
    desc: '上传题库自动生成答案解析',
    color: 'bg-purple-100 text-purple-700',
  },
];

export default function Home() {
  const { banks, setBanks, user } = useQuizStore();
  const [loading, setLoading] = useState(true);
  
  useEffect(() => {
    bankApi.getList().then((res) => {
      setBanks(res.banks);
      setLoading(false);
    }).catch(() => {
      setLoading(false);
    });
  }, [setBanks]);
  
  return (
    <div className="space-y-8 animate-fade-in">
      {/* 欢迎区 */}
      <section className="text-center py-8">
        <h1 className="text-3xl sm:text-4xl font-bold text-gray-800 dark:text-slate-100 mb-3">
          欢迎使用刷题助手
        </h1>
        <p className="text-gray-500 dark:text-slate-400 dark:text-slate-500 max-w-md mx-auto">
          高效刷题、智能分析、AI 解析，助你轻松通过考试
        </p>
        
        {user && (
          <div className="mt-4 inline-flex items-center gap-2 px-4 py-2 bg-white dark:bg-slate-800 rounded-full shadow-sm dark:shadow-slate-900/30 border border-gray-100 dark:border-slate-700">
            <span className="text-gray-500 dark:text-slate-400 dark:text-slate-500">欢迎回来，</span>
            <span className="font-medium text-gray-800 dark:text-slate-100">{user.name}</span>
            <span className="text-gray-300">|</span>
            <span className="text-primary-600 dark:text-primary-300 font-medium">
              正确率 {user.rate}%
            </span>
          </div>
        )}
      </section>
      
      {/* 题库列表 */}
      <section>
        <h2 className="text-lg font-semibold text-gray-800 dark:text-slate-100 mb-4 flex items-center gap-2">
          <BookOpen className="w-5 h-5 text-primary-500 dark:text-primary-400" />
          选择题库
        </h2>
        
        {loading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            {[1, 2].map((i) => (
              <div key={i} className="h-32 bg-white dark:bg-slate-800 rounded-xl animate-pulse" />
            ))}
          </div>
        ) : banks.length > 0 ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            {banks.map((bank) => (
              <Link
                key={bank.key}
                to="/practice"
                className="group bg-white dark:bg-slate-800 rounded-xl p-5 border border-gray-100 dark:border-slate-700 shadow-sm dark:shadow-slate-900/30 hover:shadow-md transition-all"
              >
                <div className="flex items-start justify-between">
                  <div>
                    <h3 className="font-semibold text-gray-800 dark:text-slate-100 group-hover:text-primary-600 dark:text-primary-300 transition-colors">
                      {bank.name}
                    </h3>
                    <p className="text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500 mt-1">
                      共 {bank.total} 道题 · {bank.chapters.length} 个章节
                    </p>
                  </div>
                  <div
                    className="w-10 h-10 rounded-lg flex items-center justify-center text-white"
                    style={{ backgroundColor: bank.color }}
                  >
                    <BookOpen className="w-5 h-5" />
                  </div>
                </div>
                <div className="mt-4 flex items-center text-sm text-primary-600 dark:text-primary-300 font-medium">
                  开始练习
                  <ArrowRight className="w-4 h-4 ml-1 group-hover:translate-x-1 transition-transform" />
                </div>
              </Link>
            ))}
          </div>
        ) : (
          <div className="text-center py-8 text-gray-400 dark:text-slate-500 bg-white dark:bg-slate-800 rounded-xl">
            暂无可用题库
          </div>
        )}
      </section>
      
      {/* 功能特点 */}
      <section>
        <h2 className="text-lg font-semibold text-gray-800 dark:text-slate-100 mb-4">功能特点</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {FEATURES.map((feature) => (
            <div
              key={feature.title}
              className="flex items-start gap-3 bg-white dark:bg-slate-800 rounded-xl p-4 border border-gray-100 dark:border-slate-700"
            >
              <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${feature.color}`}>
                <feature.icon className="w-5 h-5" />
              </div>
              <div>
                <h3 className="font-medium text-gray-800 dark:text-slate-100">{feature.title}</h3>
                <p className="text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500 mt-0.5">{feature.desc}</p>
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
