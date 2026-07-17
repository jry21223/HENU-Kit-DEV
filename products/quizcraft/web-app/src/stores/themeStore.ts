import { create } from 'zustand';

const KEY = 'theme-preference';

function getInitial(): boolean {
  try {
    const v = localStorage.getItem(KEY);
    if (v !== null) return v === 'dark';
  } catch {
    // localStorage can be unavailable in restricted browser contexts.
  }
  return false; // default light
}

function apply(isDark: boolean) {
  document.documentElement.classList.toggle('dark', isDark);
  try {
    localStorage.setItem(KEY, isDark ? 'dark' : 'light');
  } catch {
    // Keep theme toggling working even when persistence is blocked.
  }
  // Notify Android WebView if available
  try {
    const bridge = (window as any).AndroidBridge;
    if (bridge?.setDarkMode) bridge.setDarkMode(isDark);
  } catch {
    // AndroidBridge is optional outside the native app shell.
  }
}

if (typeof document !== 'undefined') {
  apply(getInitial());
}

interface ThemeState {
  isDark: boolean;
  toggle: () => void;
}

export const useThemeStore = create<ThemeState>((set, get) => ({
  isDark: typeof document !== 'undefined'
    ? document.documentElement.classList.contains('dark')
    : false,
  toggle: () => {
    const next = !get().isDark;
    set({ isDark: next });
    apply(next);
  },
}));
