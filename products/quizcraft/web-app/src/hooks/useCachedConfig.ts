import { useState } from 'react';
import type { AnalysisConfig } from '@/types';

const LEGACY_CACHE_KEY = 'quiz_app_llm_config';
const CACHE_KEY = 'quiz_app_llm_config:v1';
const CACHE_EXPIRY = 30 * 24 * 60 * 60 * 1000; // 30 天过期

interface CachedConfig {
  config: AnalysisConfig;
  timestamp: number;
}

const defaultConfig: AnalysisConfig = {
  provider: 'deepseek',
  apiKey: '',
  apiUrl: '',
  model: '',
};

const readCachedConfig = (): AnalysisConfig => {
  if (typeof window === 'undefined') {
    return defaultConfig;
  }

  try {
    const cached =
      localStorage.getItem(CACHE_KEY) ||
      localStorage.getItem(LEGACY_CACHE_KEY);
    if (!cached) {
      return defaultConfig;
    }

    const data: CachedConfig = JSON.parse(cached);
    if (Date.now() - data.timestamp < CACHE_EXPIRY) {
      if (!localStorage.getItem(CACHE_KEY)) {
        localStorage.setItem(CACHE_KEY, cached);
        localStorage.removeItem(LEGACY_CACHE_KEY);
      }
      return data.config;
    }

    localStorage.removeItem(CACHE_KEY);
    localStorage.removeItem(LEGACY_CACHE_KEY);
  } catch (e) {
    console.error('加载缓存配置失败:', e);
  }

  return defaultConfig;
};

export function useCachedConfig() {
  const [config, setConfig] = useState<AnalysisConfig>(() => readCachedConfig());

  // 保存到 localStorage
  const saveConfig = (newConfig: AnalysisConfig) => {
    setConfig(newConfig);
    try {
      // 安全提示：API Key 明文存储在浏览器 localStorage
      if (newConfig.apiKey) {
        console.warn(
          '⚠️ API Key 已明文存储在浏览器 localStorage。' +
          '在共享或生产环境中请谨慎使用，建议仅在本地开发环境启用此功能。'
        );
      }
      const data: CachedConfig = {
        config: newConfig,
        timestamp: Date.now(),
      };
      localStorage.setItem(CACHE_KEY, JSON.stringify(data));
    } catch (e) {
      console.error('保存配置失败:', e);
    }
  };

  // 清除缓存
  const clearConfig = () => {
    setConfig(defaultConfig);
    localStorage.removeItem(CACHE_KEY);
    localStorage.removeItem(LEGACY_CACHE_KEY);
  };

  // 更新单个字段
  const updateConfig = (updates: Partial<AnalysisConfig>) => {
    const newConfig = { ...config, ...updates };
    saveConfig(newConfig);
  };

  // 获取 API Key 数量（用于显示）
  const keyCount = config.apiKey
    .split(/[,\n;]+/)
    .filter(k => k.trim())
    .length;

  return {
    config,
    setConfig: saveConfig,
    updateConfig,
    clearConfig,
    isLoaded: true,
    keyCount,
  };
}
