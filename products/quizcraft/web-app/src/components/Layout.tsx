import { useState } from 'react';
import { BookOpen, Heart, MessageSquare, Moon, Sun, Trophy } from 'lucide-react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import clsx from 'clsx';

import { QUIZCRAFT_GO_SHADOW_ENABLED } from '@/api/quizcraftShadowClient';
import { useThemeStore } from '@/stores/themeStore';

const baseNavigation = [
  { path: '/practice', icon: BookOpen, label: '刷题' },
  { path: '/ranking', icon: Trophy, label: '排行榜' },
];

const navigation = QUIZCRAFT_GO_SHADOW_ENABLED
  ? [
      baseNavigation[0],
      { path: '/favorites', icon: Heart, label: '收藏' },
      { path: '/feedback', icon: MessageSquare, label: '纠错' },
      baseNavigation[1],
    ]
  : baseNavigation;

export default function Layout() {
  const location = useLocation();
  const { isDark, toggle: toggleTheme } = useThemeStore();
  const [navigationOpen, setNavigationOpen] = useState(false);

  return (
    <div className="flex min-h-screen flex-col bg-gradient-to-br from-blue-50 via-gray-50 to-white dark:from-slate-950 dark:via-slate-900 dark:to-slate-950">
      <header className="sticky top-0 z-50 border-b border-gray-200 bg-white/85 backdrop-blur-md dark:border-slate-700 dark:bg-slate-900/85">
        <div className="mx-auto flex min-h-14 max-w-5xl items-center justify-between gap-3 px-4">
          <Link to="/practice" className="flex items-center gap-2" onClick={() => setNavigationOpen(false)}>
            <img src="/apple-touch-icon.png" alt="练习" className="h-8 w-8 rounded-lg object-cover" />
            <span className="font-bold text-gray-800 dark:text-slate-100">河大 Kit · 练习服务</span>
          </Link>

          <nav id="practice-navigation" aria-label="练习导航" className={clsx('items-center gap-1', navigationOpen ? 'flex' : 'hidden sm:flex')}>
            {navigation.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                onClick={() => setNavigationOpen(false)}
                className={clsx(
                  'flex min-h-10 items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                  location.pathname === item.path
                    ? 'bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-slate-400 dark:hover:bg-slate-800',
                )}
              >
                <item.icon className="h-4 w-4" aria-hidden="true" />
                <span>{item.label}</span>
              </Link>
            ))}
            <button
              type="button"
              onClick={toggleTheme}
              className="rounded-lg p-2 text-gray-600 hover:bg-gray-100 dark:text-slate-400 dark:hover:bg-slate-800"
              aria-label={isDark ? '切换到亮色模式' : '切换到暗黑模式'}
            >
              {isDark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
          </nav>

          <button
            type="button"
            className="rounded-lg border border-gray-200 px-3 py-2 text-sm text-gray-700 sm:hidden dark:border-slate-700 dark:text-slate-200"
            aria-expanded={navigationOpen}
            aria-controls="practice-navigation"
            onClick={() => setNavigationOpen((open) => !open)}
          >
            导航
          </button>
        </div>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}
