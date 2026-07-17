import { useCallback, useEffect, useReducer, useRef } from 'react';
import { AnimatePresence, LazyMotion, domAnimation, m } from 'framer-motion';
import {
  Upload,
  FileText,
  Loader2,
  CheckCircle2,
  Wand2,
  Download,
  ChevronDown,
  X,
  Zap,
  AlertCircle,
  Clock,
  Key,
  Save,
  Plus,
  Trash2,
  Copy,
  PackagePlus,
  RefreshCw,
} from 'lucide-react';
import {
  adminApi,
  analysisApi,
  bankApi,
  buildBrowserURL,
  buildWebSocketURL,
} from '@/api/client';
import { useCachedConfig } from '@/hooks/useCachedConfig';
import { useManagedWebSocket } from '@/hooks/useManagedWebSocket';
import { useQuizStore } from '@/stores/quizStore';
import type { ParsedQuestion, QuestionType } from '@/types';

type BuilderStep = 'select' | 'parse' | 'review' | 'analyze';

interface ProgressInfo {
  current: number;
  total: number;
  percentage: number;
  message: string;
}

interface EditableQuestion extends ParsedQuestion {
  options?: string[];
  analysis?: string;
  chapter?: string;
}

type EditableQuestionInput =
  | EditableQuestion
  | (Partial<EditableQuestion> & Record<string, unknown>);

const DEFAULT_BANK_COLOR = '#1976d2';

type ExtractUiState = {
  step: BuilderStep;
  file: File | null;
  questions: EditableQuestion[];
  expandedQ: string | null;
  bankName: string;
  bankKey: string;
  bankColor: string;
  overwriteExisting: boolean;
  showKeyInput: boolean;
  isProcessing: boolean;
  isSaving: boolean;
  progress: ProgressInfo | null;
  error: string;
  success: string;
  analyzeTime: number;
  adminTokenInput: string;
  adminAuthenticated: boolean;
  adminSessionLoading: boolean;
};

type ExtractUiAction =
  | Partial<ExtractUiState>
  | ((state: ExtractUiState) => Partial<ExtractUiState>);

const createInitialExtractUiState = (): ExtractUiState => ({
  step: 'select',
  file: null,
  questions: [],
  expandedQ: null,
  bankName: '',
  bankKey: '',
  bankColor: DEFAULT_BANK_COLOR,
  overwriteExisting: false,
  showKeyInput: false,
  isProcessing: false,
  isSaving: false,
  progress: null,
  error: '',
  success: '',
  analyzeTime: 0,
  adminTokenInput: '',
  adminAuthenticated: false,
  adminSessionLoading: true,
});

const mergeExtractUiState = (
  state: ExtractUiState,
  action: ExtractUiAction,
) => ({
  ...state,
  ...(typeof action === 'function' ? action(state) : action),
});

const QUESTION_TYPE_OPTIONS: Array<{ value: QuestionType; label: string }> = [
  { value: 'single', label: '单选题' },
  { value: 'multi', label: '多选题' },
  { value: 'judge', label: '判断题' },
  { value: 'blank', label: '填空题' },
];

const createQuestionId = () =>
  `draft_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;

const letterFromIndex = (index: number) => String.fromCharCode(65 + index);

const indexFromLetter = (letter: string) => {
  const code = letter.toUpperCase().charCodeAt(0) - 65;
  return code >= 0 && code < 26 ? code : -1;
};

const normalizeJudgeAnswerText = (value: unknown) => {
  if (value === true) return '对';
  if (value === false) return '错';

  const text = String(value ?? '').trim().toLowerCase();
  if (!text) return '';
  if (['对', '正确', '是', 'true', '1', '√', 'yes', 'y'].includes(text)) {
    return '对';
  }
  if (['错', '错误', '否', 'false', '0', '×', 'no', 'n'].includes(text)) {
    return '错';
  }
  return '';
};

const extractChoiceIndexes = (value: unknown) => {
  if (Array.isArray(value)) {
    const indexes = new Set<number>();
    for (const item of value) {
      const index = Number(item);
      if (Number.isInteger(index) && index >= 0 && index < 26) {
        indexes.add(index);
      }
    }
    return Array.from(indexes).sort((a, b) => a - b);
  }

  if (typeof value === 'number' && Number.isInteger(value) && value >= 0 && value < 26) {
    return [value];
  }

  const letters = String(value ?? '')
    .toUpperCase()
    .match(/[A-Z]/g) ?? [];

  const indexes = new Set<number>();
  for (const letter of letters) {
    const index = indexFromLetter(letter);
    if (index >= 0) {
      indexes.add(index);
    }
  }
  return Array.from(indexes).sort((a, b) => a - b);
};

const indexesToAnswerText = (indexes: number[]) =>
  Array.from(new Set(indexes))
    .sort((a, b) => a - b)
    .map(letterFromIndex)
    .join('');

const normalizeAnswerText = (value: unknown, type: QuestionType) => {
  if (type === 'blank') {
    return String(value ?? '').replace(/\s+/g, ' ').trim();
  }
  if (type === 'judge') {
    return normalizeJudgeAnswerText(value);
  }

  const indexes = extractChoiceIndexes(value);
  if (type === 'single') {
    return indexes.length ? letterFromIndex(indexes[0]) : '';
  }
  return indexesToAnswerText(indexes);
};

const slugifyBankKey = (value: string) =>
  value
    .trim()
    .toLowerCase()
    .replace(/[^\w\u4e00-\u9fff-]+/g, '_')
    .replace(/^_+|_+$/g, '');

const createEmptyQuestion = (index: number): EditableQuestion => ({
  id: createQuestionId(),
  number: String(index),
  type: 'single',
  content: '',
  options: ['', ''],
  answer: '',
  analysis: '',
  chapter: '默认章节',
});

const normalizeEditableQuestion = (
  raw: EditableQuestionInput,
  index: number
): EditableQuestion => {
  const type: QuestionType =
    raw.type === 'multi' || raw.type === 'judge' || raw.type === 'blank'
      ? raw.type
      : 'single';

  const options =
    type === 'judge' || type === 'blank'
      ? []
      : Array.isArray(raw.options) && raw.options.length > 0
        ? raw.options.map((option) => String(option ?? ''))
        : ['', ''];

  return {
    id: String(raw.id || createQuestionId()),
    number: String(raw.number ?? index),
    type,
    content: String(raw.content ?? '').trim(),
    options,
    answer: normalizeAnswerText(raw.answer, type),
    analysis: String(raw.analysis ?? '').trim(),
    chapter: String(raw.chapter ?? '默认章节').trim() || '默认章节',
  };
};

const reindexQuestions = (questions: EditableQuestion[]) =>
  questions.map((question, index) =>
    normalizeEditableQuestion({ ...question, number: String(index + 1) }, index + 1)
  );

const mergeAnalyzedQuestions = (
  current: EditableQuestion[],
  incoming: Array<Record<string, unknown>>
) => {
  const incomingMap = new Map(
    incoming
      .map((question, index) => {
        const normalized = normalizeEditableQuestion(
          question as Partial<EditableQuestion> & Record<string, unknown>,
          index + 1
        );
        return [normalized.id, normalized] as const;
      })
  );

  return current.map((question, index) =>
    incomingMap.has(question.id)
      ? normalizeEditableQuestion(
          { ...question, ...incomingMap.get(question.id)! },
          index + 1
        )
      : normalizeEditableQuestion(question, index + 1)
  );
};

const getTypeBadgeClass = (type: QuestionType) => {
  if (type === 'single') return 'bg-blue-100 text-blue-700';
  if (type === 'multi') return 'bg-purple-100 text-purple-700';
  if (type === 'judge') return 'bg-orange-100 text-orange-700';
  return 'bg-green-100 text-green-700';
};

function useExtractController() {
  const { config, updateConfig, clearConfig, keyCount } = useCachedConfig();
  const { connect: connectWebSocket, close: closeWebSocket } = useManagedWebSocket();
  const { setBanks } = useQuizStore();

  const [ui, setUi] = useReducer(
    mergeExtractUiState,
    undefined,
    createInitialExtractUiState,
  );
  const {
    step,
    file,
    questions,
    expandedQ,
    bankName,
    bankKey,
    bankColor,
    overwriteExisting,
    showKeyInput,
    isProcessing,
    isSaving,
    progress,
    error,
    success,
    analyzeTime,
    adminTokenInput,
    adminAuthenticated,
    adminSessionLoading,
  } = ui;

  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const keyTouchedRef = useRef(false);

  const closeAsyncResources = useCallback(() => {
    closeWebSocket();
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, [closeWebSocket]);

  useEffect(() => {
    return closeAsyncResources;
  }, [closeAsyncResources]);

  useEffect(() => {
    let active = true;
    void adminApi
      .getSession()
      .then((session) => {
        if (active) {
          setUi({
            adminAuthenticated: session.authenticated,
            adminSessionLoading: false,
          });
        }
      })
      .catch(() => {
        if (active) {
          setUi({ adminAuthenticated: false, adminSessionLoading: false });
        }
      });
    return () => {
      active = false;
    };
  }, []);

  const clearAsyncState = useCallback(() => {
    closeAsyncResources();
    setUi({ progress: null, analyzeTime: 0, isProcessing: false });
  }, [closeAsyncResources]);

  const resetWorkspace = useCallback(() => {
    clearAsyncState();
    setUi({
      step: 'select',
      file: null,
      questions: [],
      expandedQ: null,
      bankName: '',
      bankKey: '',
      bankColor: DEFAULT_BANK_COLOR,
      overwriteExisting: false,
      error: '',
      success: '',
    });
    keyTouchedRef.current = false;
  }, [clearAsyncState]);

  const applyQuestions = useCallback((items: EditableQuestionInput[]) => {
    const normalized = reindexQuestions(
      items.map((question, index) =>
        normalizeEditableQuestion(
          question as Partial<EditableQuestion> & Record<string, unknown>,
          index + 1
        )
      )
    );
    setUi({
      questions: normalized,
      expandedQ: normalized[0]?.id ?? null,
    });
    return normalized;
  }, []);

  const patchQuestions = useCallback(
    (updater: (current: EditableQuestion[]) => EditableQuestion[]) => {
      setUi((current) => {
        const next = reindexQuestions(updater(current.questions));
        const expandedQ =
          current.expandedQ && next.some((question) => question.id === current.expandedQ)
            ? current.expandedQ
            : next[0]?.id ?? null;
        return { questions: next, expandedQ };
      });
    },
    []
  );

  const handleBankNameChange = (value: string) => {
    setUi({ bankName: value });
    if (!keyTouchedRef.current) {
      setUi({ bankKey: slugifyBankKey(value) });
    }
  };

  const startBlankBank = () => {
    clearAsyncState();
    const draftName = '新建题库';
    setUi({
      step: 'review',
      file: null,
      error: '',
      success: '',
      bankColor: DEFAULT_BANK_COLOR,
      overwriteExisting: false,
      bankName: draftName,
      bankKey: slugifyBankKey(draftName),
    });
    keyTouchedRef.current = false;
    applyQuestions([createEmptyQuestion(1)]);
  };

  const processFile = useCallback(async (selectedFile: File) => {
    if (!adminAuthenticated) {
      setUi({ error: '请先建立后台管理会话' });
      return;
    }
    clearAsyncState();

    const inferredName = selectedFile.name.replace(/\.[^/.]+$/, '');
    setUi({
      file: selectedFile,
      step: 'parse',
      error: '',
      success: '',
      bankName: inferredName,
      bankKey: slugifyBankKey(inferredName),
      bankColor: DEFAULT_BANK_COLOR,
      overwriteExisting: false,
    });
    keyTouchedRef.current = false;

    try {
      const result = await analysisApi.parseFile(selectedFile);
      const normalized = applyQuestions(result.questions);
      if (normalized.length === 0) {
        throw new Error('未识别到可编辑的题目');
      }
      setUi({
        step: 'review',
        success: `已导入 ${normalized.length} 道题目，可以在线继续编辑`,
      });
    } catch (err) {
      setUi({
        step: 'select',
        error: '文件解析失败: ' + (err as Error).message,
      });
    }
  }, [adminAuthenticated, applyQuestions, clearAsyncState]);

  const handleFileDrop = useCallback(async (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    const droppedFile = event.dataTransfer.files[0];
    if (droppedFile) {
      await processFile(droppedFile);
    }
  }, [processFile]);

  const handleFileSelect = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFile = event.target.files?.[0];
    if (selectedFile) {
      await processFile(selectedFile);
      event.target.value = '';
    }
  };

  const updateQuestion = (
    questionId: string,
    updater: (question: EditableQuestion) => EditableQuestion
  ) => {
    patchQuestions((current) =>
      current.map((question) =>
        question.id === questionId ? updater(question) : question
      )
    );
  };

  const addQuestion = () => {
    patchQuestions((current) => [...current, createEmptyQuestion(current.length + 1)]);
    setUi({ success: '' });
  };

  const duplicateQuestion = (questionId: string) => {
    patchQuestions((current) => {
      const index = current.findIndex((question) => question.id === questionId);
      if (index < 0) return current;
      const duplicated = normalizeEditableQuestion(
        {
          ...current[index],
          id: createQuestionId(),
        },
        index + 2
      );
      const next = [...current];
      next.splice(index + 1, 0, duplicated);
      return next;
    });
    setUi({ success: '' });
  };

  const removeQuestion = (questionId: string) => {
    if (questions.length <= 1) {
      setUi({ error: '至少保留 1 道题目' });
      return;
    }
    patchQuestions((current) =>
      current.filter((question) => question.id !== questionId)
    );
    setUi({ success: '' });
  };

  const changeQuestionType = (questionId: string, nextType: QuestionType) => {
    updateQuestion(questionId, (question) => {
      if (nextType === 'judge') {
        return {
          ...question,
          type: 'judge',
          options: [],
          answer: normalizeJudgeAnswerText(question.answer) || '',
        };
      }

      if (nextType === 'blank') {
        return {
          ...question,
          type: 'blank',
          options: [],
          answer: normalizeAnswerText(question.answer, 'blank'),
        };
      }

      const currentIndexes = extractChoiceIndexes(question.answer);
      const options =
        question.options && question.options.length >= 2
          ? question.options
          : ['', ''];

      return {
        ...question,
        type: nextType,
        options,
        answer:
          nextType === 'single'
            ? currentIndexes.length
              ? letterFromIndex(currentIndexes[0])
              : ''
            : indexesToAnswerText(currentIndexes),
      };
    });
  };

  const addOption = (questionId: string) => {
    updateQuestion(questionId, (question) => ({
      ...question,
      options: [...(question.options ?? []), ''],
    }));
  };

  const removeOption = (questionId: string, optionIndex: number) => {
    updateQuestion(questionId, (question) => {
      const options = (question.options ?? []).filter((_, index) => index !== optionIndex);
      if (options.length < 2) {
        setUi({ error: '选择题至少保留 2 个选项' });
        return question;
      }

      if (question.type === 'single') {
        const currentIndex = extractChoiceIndexes(question.answer)[0];
        let nextAnswer = question.answer;
        if (currentIndex === optionIndex) {
          nextAnswer = '';
        } else if (currentIndex > optionIndex) {
          nextAnswer = letterFromIndex(currentIndex - 1);
        }
        return {
          ...question,
          options,
          answer: nextAnswer,
        };
      }

      const adjustedIndexes: number[] = [];
      for (const index of extractChoiceIndexes(question.answer)) {
        if (index === optionIndex) continue;
        adjustedIndexes.push(index > optionIndex ? index - 1 : index);
      }

      return {
        ...question,
        options,
        answer: indexesToAnswerText(adjustedIndexes),
      };
    });
  };

  const setSingleAnswer = (questionId: string, optionIndex: number) => {
    updateQuestion(questionId, (question) => ({
      ...question,
      answer: letterFromIndex(optionIndex),
    }));
  };

  const toggleMultiAnswer = (questionId: string, optionIndex: number) => {
    updateQuestion(questionId, (question) => {
      const currentIndexes = extractChoiceIndexes(question.answer);
      const nextIndexes = currentIndexes.includes(optionIndex)
        ? currentIndexes.filter((index) => index !== optionIndex)
        : [...currentIndexes, optionIndex];

      return {
        ...question,
        answer: indexesToAnswerText(nextIndexes),
      };
    });
  };

  const setJudgeAnswer = (questionId: string, value: '对' | '错') => {
    updateQuestion(questionId, (question) => ({
      ...question,
      answer: value,
    }));
  };

  const setBlankAnswer = (questionId: string, value: string) => {
    updateQuestion(questionId, (question) => ({
      ...question,
      answer: value,
    }));
  };

  const validateBeforePersist = () => {
    if (!bankName.trim()) {
      setUi({ error: '请先填写题库名称' });
      return false;
    }
    if (questions.length === 0) {
      setUi({ error: '题库里至少需要 1 道题目' });
      return false;
    }
    const missingBlankAnswer = questions.find(
      (question) =>
        question.type === 'blank' && !String(question.answer ?? '').trim()
    );
    if (missingBlankAnswer) {
      setUi({ error: `第 ${missingBlankAnswer.number} 题填空题答案不能为空` });
      return false;
    }
    return true;
  };

  const validateAdminSession = () => {
    if (!adminAuthenticated) {
      setUi({ error: '请先建立后台管理会话' });
      return false;
    }
    return true;
  };

  const handleExport = async () => {
    if (!validateBeforePersist()) return;
    if (!validateAdminSession()) return;

    setUi({ error: '', success: '' });
    try {
      const result = await analysisApi.exportBank(questions, bankName.trim());
      window.open(buildBrowserURL(result.download_url), '_blank');
      setUi({ success: '标准 JSON 已生成，浏览器正在下载' });
    } catch (err) {
      setUi({ error: '导出失败: ' + (err as Error).message });
    }
  };

  const handleSaveBank = async () => {
    if (!validateBeforePersist()) return;
    if (!validateAdminSession()) return;

    setUi({ error: '', success: '', isSaving: true });

    try {
      const result = await bankApi.save({
        name: bankName.trim(),
        key: bankKey.trim() || slugifyBankKey(bankName),
        color: bankColor,
        questions,
        overwrite: overwriteExisting,
      });

      const latestBanks = await bankApi.getList();
      setBanks(latestBanks.banks);
      setUi({ bankKey: result.bank.key });
      keyTouchedRef.current = true;
      setUi({ success: `题库已保存到 ${result.file}，现在可以在刷题页直接使用` });
    } catch (err) {
      setUi({ error: '保存失败: ' + (err as Error).message });
    } finally {
      setUi({ isSaving: false });
    }
  };

  const handleGenerateAnalysis = async () => {
    if (!validateAdminSession()) return;
    if (!config.apiKey.trim()) {
      setUi({ error: '请输入 API Key' });
      return;
    }

    const pendingQuestions = questions.filter((question) => !question.analysis?.trim());
    if (pendingQuestions.length === 0) {
      setUi({ success: '当前所有题目都已经有解析了' });
      return;
    }

    setUi({
      error: '',
      success: '',
      isProcessing: true,
      step: 'analyze',
      analyzeTime: 0,
      progress: {
      current: 0,
      total: pendingQuestions.length,
      percentage: 0,
      message: '正在连接...',
      },
    });

    const startTime = Date.now();
    timerRef.current = setInterval(() => {
      setUi({ analyzeTime: Math.floor((Date.now() - startTime) / 1000) });
    }, 1000);

    const clientId = `extract_${Date.now()}`;
    const wsUrl = buildWebSocketURL(`/ws/analyze/${clientId}`);
    const ws = connectWebSocket(wsUrl);

    ws.onopen = () => {
      ws.send(JSON.stringify({
        questions: pendingQuestions,
        config,
      }));
    };

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      switch (data.type) {
        case 'progress':
          setUi({ progress: {
            current: data.current,
            total: data.total,
            percentage: data.percentage,
            message: data.message,
          } });
          break;
        case 'complete':
          setUi((current) => ({
            questions: mergeAnalyzedQuestions(current.questions, data.questions ?? []),
          }));
          clearAsyncState();
          setUi({ step: 'review', success: 'AI 解析已补齐到当前题库草稿' });
          break;
        case 'error':
          clearAsyncState();
          setUi({ step: 'review', error: data.error || 'AI 解析失败' });
          break;
      }
    };

    ws.onerror = async () => {
      try {
        setUi({ progress: {
          current: 0,
          total: pendingQuestions.length,
          percentage: 0,
          message: 'WebSocket 失败，切换备用模式...',
        } });

        const result = await analysisApi.generateAnalysis(pendingQuestions, config);
        setUi((current) => ({
          questions: mergeAnalyzedQuestions(current.questions, result.questions ?? []),
          success: '已通过备用模式补齐 AI 解析',
        }));
      } catch (err) {
        setUi({ error: '生成解析失败: ' + (err as Error).message });
      } finally {
        clearAsyncState();
        setUi({ step: 'review' });
      }
    };
  };

  const totalQuestions = questions.length;
  const pendingAnalysisCount = questions.filter((question) => !question.analysis?.trim()).length;
  const singleCount = questions.filter((question) => question.type === 'single').length;
  const multiCount = questions.filter((question) => question.type === 'multi').length;
  const judgeCount = questions.filter((question) => question.type === 'judge').length;
  const blankCount = questions.filter((question) => question.type === 'blank').length;
  const estimatedTime = progress ? Math.ceil((progress.total - progress.current) * 2) : 0;

  const handleAdminLogin = async () => {
    const token = adminTokenInput.trim();
    if (!token) {
      setUi({ error: '请输入后台管理 Token' });
      return;
    }

    setUi({ adminSessionLoading: true, error: '', success: '' });
    try {
      await adminApi.login(token);
      setUi({
        adminAuthenticated: true,
        adminTokenInput: '',
        success: '后台管理会话已建立，刷新页面后仍可继续使用',
      });
    } catch (err) {
      setUi({
        adminAuthenticated: false,
        error: '后台登录失败: ' + (err as Error).message,
      });
    } finally {
      setUi({ adminSessionLoading: false });
    }
  };

  const handleAdminLogout = async () => {
    setUi({ adminSessionLoading: true, error: '', success: '' });
    try {
      await adminApi.logout();
      setUi({
        adminAuthenticated: false,
        adminTokenInput: '',
        success: '后台管理会话已退出',
      });
    } catch (err) {
      setUi({ error: '退出后台会话失败: ' + (err as Error).message });
    } finally {
      setUi({ adminSessionLoading: false });
    }
  };

  const handleBankKeyChange = (value: string) => {
    setUi({ bankKey: value });
    keyTouchedRef.current = true;
  };

  const toggleQuestionExpansion = (questionId: string) => {
    setUi((current) => ({
      expandedQ: current.expandedQ === questionId ? null : questionId,
    }));
  };

  return {
    step,
    file,
    questions,
    expandedQ,
    bankName,
    bankKey,
    bankColor,
    overwriteExisting,
    showKeyInput,
    isProcessing,
    isSaving,
    progress,
    error,
    success,
    analyzeTime,
    adminTokenInput,
    adminAuthenticated,
    adminSessionLoading,
    config,
    keyCount,
    totalQuestions,
    pendingAnalysisCount,
    singleCount,
    multiCount,
    judgeCount,
    blankCount,
    estimatedTime,
    setAdminTokenInput: (value: string) => setUi({ adminTokenInput: value }),
    handleAdminLogin,
    handleAdminLogout,
    clearError: () => setUi({ error: '' }),
    clearSuccess: () => setUi({ success: '' }),
    setBankColor: (value: string) => setUi({ bankColor: value }),
    setOverwriteExisting: (value: boolean) => setUi({ overwriteExisting: value }),
    setShowKeyInput: (value: boolean) => setUi({ showKeyInput: value }),
    updateConfig,
    clearConfig,
    startBlankBank,
    handleFileDrop,
    handleFileSelect,
    handleBankNameChange,
    handleBankKeyChange,
    addQuestion,
    handleExport,
    handleSaveBank,
    resetWorkspace,
    handleGenerateAnalysis,
    duplicateQuestion,
    removeQuestion,
    updateQuestion,
    changeQuestionType,
    setBlankAnswer,
    addOption,
    removeOption,
    setSingleAnswer,
    toggleMultiAnswer,
    setJudgeAnswer,
    toggleQuestionExpansion,
  };
}

type ExtractController = ReturnType<typeof useExtractController>;

export default function Extract() {
  const controller = useExtractController();

  return (
    <LazyMotion features={domAnimation}>
      <ExtractView controller={controller} />
    </LazyMotion>
  );
}

function ExtractView({ controller }: { controller: ExtractController }) {
  return (
    <div className="max-w-5xl mx-auto animate-fade-in space-y-6">
      <div className="mb-2">
        <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100 mb-2 flex items-center gap-2">
          <Zap className="w-6 h-6 text-yellow-500" />
          题库工坊
        </h1>
        <p className="text-gray-500 dark:text-slate-400">
          在线新建题库、编写题目、上传导入文件，并导出或保存为标准 JSON
        </p>
      </div>

      <AdminTokenCard controller={controller} />
      <StatusAlerts controller={controller} />

      {controller.step === 'select' && <SelectStep controller={controller} />}
      {controller.step === 'parse' && <ParseStep file={controller.file} />}
      {controller.step === 'analyze' && controller.progress && (
        <AnalyzeStep controller={controller} />
      )}
      {controller.step === 'review' && <ReviewStep controller={controller} />}
    </div>
  );
}

function AdminTokenCard({ controller }: { controller: ExtractController }) {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 p-4">
      <div className="mb-2 flex items-center justify-between gap-3">
        <span className="text-sm font-medium text-gray-700 dark:text-slate-200">
          后台管理会话
        </span>
        {controller.adminAuthenticated && (
          <span className="text-xs font-medium text-emerald-600 dark:text-emerald-300">
            已登录
          </span>
        )}
      </div>
      {controller.adminAuthenticated ? (
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-gray-500 dark:text-slate-400">
            权限由 HttpOnly 会话 Cookie 提供，页面脚本无法读取管理密钥。
          </p>
          <button
            type="button"
            onClick={controller.handleAdminLogout}
            disabled={controller.adminSessionLoading}
            className="shrink-0 px-3 py-2 rounded-lg border border-gray-200 dark:border-slate-600 text-sm text-gray-600 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
          >
            退出会话
          </button>
        </div>
      ) : (
        <div className="flex flex-col gap-2 sm:flex-row">
          <label htmlFor="admin-token" className="sr-only">
            后台管理 Token
          </label>
          <input
            id="admin-token"
            type="password"
            autoComplete="current-password"
            value={controller.adminTokenInput}
            onChange={(event) => controller.setAdminTokenInput(event.target.value)}
            placeholder="输入 ADMIN_TOKEN 以建立会话"
            disabled={controller.adminSessionLoading}
            className="flex-1 px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg text-sm disabled:opacity-50"
          />
          <button
            type="button"
            onClick={controller.handleAdminLogin}
            disabled={controller.adminSessionLoading || !controller.adminTokenInput.trim()}
            className="shrink-0 px-3 py-2 rounded-lg bg-primary-500 text-sm font-medium text-white hover:bg-primary-600 disabled:opacity-50"
          >
            {controller.adminSessionLoading ? '验证中...' : '建立会话'}
          </button>
        </div>
      )}
      {!controller.adminAuthenticated && (
        <p className="mt-2 text-xs text-gray-400 dark:text-slate-500">
          Token 只用于本次登录交换，成功后不会保存在浏览器脚本或构建产物中。
        </p>
      )}
    </div>
  );
}

function StatusAlerts({ controller }: { controller: ExtractController }) {
  return (
    <>
      {controller.error && (
        <m.div
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-red-50 border border-red-200 rounded-xl p-4 flex items-center gap-3"
        >
          <AlertCircle className="w-5 h-5 text-red-500" />
          <span className="text-red-700">{controller.error}</span>
          <button
            type="button"
            onClick={controller.clearError}
            className="ml-auto text-red-400 hover:text-red-600"
            aria-label="关闭错误提示"
          >
            <X className="w-4 h-4" />
          </button>
        </m.div>
      )}

      {controller.success && (
        <m.div
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-green-50 border border-green-200 rounded-xl p-4 flex items-center gap-3"
        >
          <CheckCircle2 className="w-5 h-5 text-green-500" />
          <span className="text-green-700">{controller.success}</span>
          <button
            type="button"
            onClick={controller.clearSuccess}
            className="ml-auto text-green-400 hover:text-green-600"
            aria-label="关闭成功提示"
          >
            <X className="w-4 h-4" />
          </button>
        </m.div>
      )}
    </>
  );
}

function SelectStep({ controller }: { controller: ExtractController }) {
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <button
        type="button"
        onClick={controller.startBlankBank}
        className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 p-6 text-left shadow-sm dark:shadow-slate-900/30 hover:shadow-md transition-all"
      >
        <div className="w-12 h-12 bg-primary-50 text-primary-600 rounded-xl flex items-center justify-center mb-4">
          <PackagePlus className="w-6 h-6" />
        </div>
        <h2 className="text-lg font-semibold text-gray-800 dark:text-slate-100 mb-2">在线新建题库</h2>
        <p className="text-sm text-gray-500 dark:text-slate-400 leading-6">
          从空白题库开始，在线添加题目、选项、答案和解析，编辑完直接保存到系统。
        </p>
      </button>

      <div
        onDragOver={(event) => event.preventDefault()}
        onDrop={controller.handleFileDrop}
        className="bg-white dark:bg-slate-800 rounded-2xl border-2 border-dashed border-gray-300 p-6 shadow-sm dark:shadow-slate-900/30 hover:border-primary-400 hover:bg-primary-50/30 transition-colors"
      >
        <input
          type="file"
          accept=".txt,.json,.pdf,.doc,.docx"
          onChange={controller.handleFileSelect}
          className="hidden"
          id="bank-builder-file-input"
        />
        <label htmlFor="bank-builder-file-input" className="cursor-pointer block text-left">
          <div className="w-12 h-12 bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400 rounded-xl flex items-center justify-center mb-4">
            <Upload className="w-6 h-6" />
          </div>
          <h2 className="text-lg font-semibold text-gray-800 dark:text-slate-100 mb-2">上传导入题库</h2>
          <p className="text-sm text-gray-500 dark:text-slate-400 leading-6">
            支持 JSON、PDF、Word、TXT。导入后会进入在线编辑区，你可以继续修改并保存。
          </p>
          <p className="text-xs text-gray-400 dark:text-slate-500 mt-3">
            拖拽文件到这里，或点击选择文件
          </p>
        </label>
      </div>
    </div>
  );
}

function ParseStep({ file }: { file: File | null }) {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 p-8 text-center">
      <Loader2 className="w-12 h-12 text-primary-500 dark:text-primary-400 mx-auto mb-4 animate-spin" />
      <p className="text-lg font-medium text-gray-800 dark:text-slate-100 mb-2">正在解析文件...</p>
      <p className="text-sm text-gray-500 dark:text-slate-400">{file?.name}</p>
    </div>
  );
}

function AnalyzeStep({ controller }: { controller: ExtractController }) {
  const { progress } = controller;
  if (!progress) return null;

  const elapsed = `${String(Math.floor(controller.analyzeTime / 60)).padStart(2, '0')}:${String(controller.analyzeTime % 60).padStart(2, '0')}`;

  return (
    <div className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 p-8">
      <div className="text-center mb-6">
        <div className="relative inline-flex items-center justify-center">
          <svg className="w-20 h-20 transform -rotate-90">
            <circle cx="40" cy="40" r="36" stroke="#e5e7eb" strokeWidth="4" fill="none" />
            <circle
              cx="40"
              cy="40"
              r="36"
              stroke="#1976d2"
              strokeWidth="4"
              fill="none"
              strokeDasharray={`${progress.percentage * 2.26} 226`}
              className="transition-all duration-300"
            />
          </svg>
          <div className="absolute inset-0 flex items-center justify-center">
            <span className="text-2xl font-bold text-primary-600">
              {Math.round(progress.percentage)}%
            </span>
          </div>
        </div>
        <h2 className="text-xl font-bold text-gray-800 dark:text-slate-100 mt-4">AI 正在生成解析...</h2>
        <p className="text-gray-500 dark:text-slate-400 mt-1">{progress.message}</p>
      </div>

      <div className="space-y-2">
        <div className="flex justify-between text-sm">
          <span className="text-gray-600 dark:text-slate-300">进度</span>
          <span className="font-medium">{progress.current} / {progress.total}</span>
        </div>
        <div className="h-2 bg-gray-100 dark:bg-slate-700 rounded-full overflow-hidden">
          <m.div
            className="h-full origin-left bg-primary-500"
            initial={{ scaleX: 0 }}
            animate={{ scaleX: progress.percentage / 100 }}
            transition={{ duration: 0.3 }}
          />
        </div>
      </div>

      <div className="grid grid-cols-3 gap-4 mt-6 pt-6 border-t border-gray-100 dark:border-slate-700">
        <div className="text-center">
          <div className="flex items-center justify-center gap-1 text-gray-500 dark:text-slate-400 text-sm mb-1">
            <Clock className="w-4 h-4" />
            <span>已用时间</span>
          </div>
          <div className="font-mono font-medium">{elapsed}</div>
        </div>
        <div className="text-center">
          <div className="text-gray-500 dark:text-slate-400 text-sm mb-1">速度</div>
          <div className="font-medium">
            {progress.current > 0 ? (controller.analyzeTime / progress.current).toFixed(1) : '-'} 秒/题
          </div>
        </div>
        <div className="text-center">
          <div className="text-gray-500 dark:text-slate-400 text-sm mb-1">预估剩余</div>
          <div className="font-medium">~{controller.estimatedTime} 秒</div>
        </div>
      </div>
    </div>
  );
}

function ReviewStep({ controller }: { controller: ExtractController }) {
  return (
    <div className="space-y-6">
      <BankMetadataPanel controller={controller} />
      <AiAnalysisPanel controller={controller} />
      <QuestionEditorList controller={controller} />
    </div>
  );
}

function BankMetadataPanel({ controller }: { controller: ExtractController }) {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 p-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h2 className="text-xl font-semibold text-gray-800 dark:text-slate-100 flex items-center gap-2">
            <FileText className="w-5 h-5 text-primary-500 dark:text-primary-400" />
            在线题库编辑
          </h2>
          <p className="text-sm text-gray-500 dark:text-slate-400 mt-1">
            题库会统一保存为标准 JSON 结构，并自动加入首页和练习页的题库列表。
          </p>
        </div>
        <BankStats controller={controller} />
      </div>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4 mt-6">
        <div className="xl:col-span-2">
          <label htmlFor="bank-name" className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
            题库名称
          </label>
          <input
            id="bank-name"
            type="text"
            value={controller.bankName}
            onChange={(event) => controller.handleBankNameChange(event.target.value)}
            placeholder="例如：马克思主义基本原理"
            className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
          />
        </div>
        <div>
          <label htmlFor="bank-key" className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
            题库代号
          </label>
          <input
            id="bank-key"
            type="text"
            value={controller.bankKey}
            onChange={(event) => controller.handleBankKeyChange(event.target.value)}
            placeholder="例如：marxism"
            className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
          />
        </div>
        <div>
          <label htmlFor="bank-color" className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
            主题颜色
          </label>
          <div className="flex items-center gap-3">
            <input
              id="bank-color"
              type="color"
              value={controller.bankColor}
              onChange={(event) => controller.setBankColor(event.target.value)}
              className="h-10 w-14 rounded border border-gray-200 dark:border-slate-600 bg-white dark:bg-slate-800 p-1"
            />
            <span className="text-sm text-gray-500 dark:text-slate-400">{controller.bankColor}</span>
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3 mt-6">
        <button
          type="button"
          onClick={controller.addQuestion}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-primary-500 text-white hover:bg-primary-600"
        >
          <Plus className="w-4 h-4" />
          新增题目
        </button>
        <button
          type="button"
          onClick={controller.handleExport}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg border border-gray-200 dark:border-slate-600 text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:bg-slate-700"
        >
          <Download className="w-4 h-4" />
          下载标准 JSON
        </button>
        <button
          type="button"
          onClick={controller.handleSaveBank}
          disabled={controller.isSaving}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg border border-primary-200 text-primary-700 hover:bg-primary-50 disabled:opacity-60"
        >
          {controller.isSaving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
          保存到题库
        </button>
        <button
          type="button"
          onClick={controller.resetWorkspace}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg border border-gray-200 dark:border-slate-600 text-gray-700 dark:text-slate-200 hover:bg-gray-50 dark:bg-slate-700"
        >
          <RefreshCw className="w-4 h-4" />
          重新选择来源
        </button>
      </div>

      <label className="mt-4 inline-flex items-center gap-2 text-sm text-gray-600 dark:text-slate-300">
        <input
          type="checkbox"
          checked={controller.overwriteExisting}
          onChange={(event) => controller.setOverwriteExisting(event.target.checked)}
          className="rounded border-gray-300"
        />
        允许覆盖同名题库代号
      </label>
    </div>
  );
}

function BankStats({ controller }: { controller: ExtractController }) {
  return (
    <div className="flex flex-wrap gap-2">
      <span className="px-3 py-1 rounded-full bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-200 text-sm">
        共 {controller.totalQuestions} 题
      </span>
      <span className="px-3 py-1 rounded-full bg-blue-100 text-blue-700 text-sm">
        单选 {controller.singleCount}
      </span>
      <span className="px-3 py-1 rounded-full bg-purple-100 text-purple-700 text-sm">
        多选 {controller.multiCount}
      </span>
      <span className="px-3 py-1 rounded-full bg-green-100 text-green-700 text-sm">
        填空 {controller.blankCount}
      </span>
      <span className="px-3 py-1 rounded-full bg-orange-100 text-orange-700 text-sm">
        判断 {controller.judgeCount}
      </span>
    </div>
  );
}

function AiAnalysisPanel({ controller }: { controller: ExtractController }) {
  const { config } = controller;
  const providerName =
    config.provider === 'openai'
      ? 'OpenAI'
      : config.provider === 'deepseek'
        ? 'DeepSeek'
        : 'SiliconFlow';

  return (
    <div className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 p-5">
      <div className="flex items-center gap-2 mb-4">
        <Wand2 className="w-5 h-5 text-primary-500 dark:text-primary-400" />
        <h3 className="font-medium">AI 解析（可选）</h3>
        <span className="px-2 py-0.5 bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-300 rounded text-xs">
          待补解析 {controller.pendingAnalysisCount} 题
        </span>
      </div>

      {controller.keyCount > 0 && !controller.showKeyInput && (
        <div className="mb-4 p-3 bg-gray-50 dark:bg-slate-700 rounded-lg">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Key className="w-4 h-4 text-gray-400 dark:text-slate-500" />
              <span className="text-sm text-gray-600 dark:text-slate-300">{providerName}</span>
              <span className="text-xs text-gray-400 dark:text-slate-500">
                {controller.keyCount} 个 API Key 已保存
              </span>
            </div>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => controller.setShowKeyInput(true)}
                className="text-xs px-2 py-1 text-primary-600 hover:bg-primary-50 rounded"
              >
                修改
              </button>
              <button
                type="button"
                onClick={controller.clearConfig}
                className="text-xs px-2 py-1 text-red-600 hover:bg-red-50 rounded"
              >
                清除
              </button>
            </div>
          </div>
        </div>
      )}

      {(controller.showKeyInput || controller.keyCount === 0) && (
        <AiConfigForm controller={controller} />
      )}

      <button
        type="button"
        onClick={controller.handleGenerateAnalysis}
        disabled={controller.isProcessing || controller.keyCount === 0}
        className="mt-4 inline-flex items-center gap-2 px-4 py-2 bg-primary-500 text-white rounded-lg hover:bg-primary-600 disabled:opacity-50"
      >
        {controller.isProcessing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Wand2 className="w-4 h-4" />}
        {controller.keyCount > 0 ? '为缺失题目生成解析' : '请先输入 API Key'}
      </button>
    </div>
  );
}

function AiConfigForm({ controller }: { controller: ExtractController }) {
  const { config } = controller;

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <select
          aria-label="AI 服务商"
          value={config.provider}
          onChange={(event) =>
            controller.updateConfig({
              provider: event.target.value as ExtractController['config']['provider'],
            })
          }
          className="px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg text-sm"
        >
          <option value="deepseek">DeepSeek（推荐）</option>
          <option value="openai">OpenAI</option>
          <option value="siliconflow">SiliconFlow</option>
        </select>

        <input
          aria-label="模型名称"
          type="text"
          value={config.model ?? ''}
          onChange={(event) => controller.updateConfig({ model: event.target.value })}
          placeholder="模型名称（可选）"
          className="px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg text-sm"
        />
      </div>

      <div className="relative">
        <input
          aria-label="API Key"
          type="password"
          value={config.apiKey}
          onChange={(event) => controller.updateConfig({ apiKey: event.target.value })}
          placeholder="输入 API Key（支持逗号/分号/换行；也支持 deepseek:sk-xxx）"
          className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg text-sm pr-20"
        />
        {config.apiKey && (
          <button
            type="button"
            onClick={() => controller.setShowKeyInput(false)}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-xs px-2 py-1 bg-primary-500 text-white rounded hover:bg-primary-600 flex items-center gap-1"
          >
            <Save className="w-3 h-3" />
            保存
          </button>
        )}
      </div>

      <p className="text-xs text-gray-400 dark:text-slate-500">
        支持多 API 异步：`sk1,sk2` 或 `deepseek:sk` 或 `openai|sk|url|model`
      </p>
    </div>
  );
}

function QuestionEditorList({ controller }: { controller: ExtractController }) {
  return (
    <div className="space-y-3">
      {controller.questions.map((question, index) => (
        <QuestionEditorCard
          key={question.id}
          controller={controller}
          question={question}
          index={index}
          isExpanded={controller.expandedQ === question.id}
        />
      ))}
    </div>
  );
}

function QuestionEditorCard({
  controller,
  question,
  index,
  isExpanded,
}: {
  controller: ExtractController;
  question: EditableQuestion;
  index: number;
  isExpanded: boolean;
}) {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 overflow-hidden">
      <QuestionEditorHeader
        question={question}
        index={index}
        isExpanded={isExpanded}
        onToggle={() => controller.toggleQuestionExpansion(question.id)}
      />

      <AnimatePresence>
        {isExpanded && (
          <m.div
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            className="border-t border-gray-100 dark:border-slate-700"
          >
            <QuestionExpandedEditor controller={controller} question={question} />
          </m.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function QuestionEditorHeader({
  question,
  index,
  isExpanded,
  onToggle,
}: {
  question: EditableQuestion;
  index: number;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="w-full flex items-center gap-3 p-4 hover:bg-gray-50 dark:bg-slate-700 transition-colors"
    >
      <span className="w-7 h-7 bg-gray-100 dark:bg-slate-700 rounded-full flex items-center justify-center text-xs font-medium text-gray-600 dark:text-slate-300">
        {index + 1}
      </span>
      <span className={`px-2 py-0.5 rounded text-xs ${getTypeBadgeClass(question.type)}`}>
        {QUESTION_TYPE_OPTIONS.find((item) => item.value === question.type)?.label}
      </span>
      <span className="text-xs text-gray-400 dark:text-slate-500 shrink-0">{question.chapter || '默认章节'}</span>
      <span className="flex-1 text-left truncate text-gray-700 dark:text-slate-200">
        {question.content || '未填写题干'}
      </span>
      {question.analysis?.trim() && (
        <span className="px-2 py-0.5 bg-green-100 text-green-600 rounded text-xs">
          已解析
        </span>
      )}
      <ChevronDown className={`w-5 h-5 text-gray-400 dark:text-slate-500 transition-transform ${isExpanded ? 'rotate-180' : ''}`} />
    </button>
  );
}

function QuestionExpandedEditor({
  controller,
  question,
}: {
  controller: ExtractController;
  question: EditableQuestion;
}) {
  const fieldPrefix = `question-${question.id}`;

  return (
    <div className="p-5 space-y-4">
      <div className="flex flex-wrap gap-3 justify-end">
        <button
          type="button"
          onClick={() => controller.duplicateQuestion(question.id)}
          className="inline-flex items-center gap-1.5 text-sm text-gray-600 dark:text-slate-300 hover:text-primary-600"
        >
          <Copy className="w-4 h-4" />
          复制题目
        </button>
        <button
          type="button"
          onClick={() => controller.removeQuestion(question.id)}
          className="inline-flex items-center gap-1.5 text-sm text-red-600 hover:text-red-700"
        >
          <Trash2 className="w-4 h-4" />
          删除题目
        </button>
      </div>

      <QuestionDetailsFields
        controller={controller}
        question={question}
        fieldPrefix={fieldPrefix}
      />
      <AnswerEditor controller={controller} question={question} fieldPrefix={fieldPrefix} />
      <QuestionAnalysisField
        controller={controller}
        question={question}
        fieldPrefix={fieldPrefix}
      />
    </div>
  );
}

function QuestionDetailsFields({
  controller,
  question,
  fieldPrefix,
}: {
  controller: ExtractController;
  question: EditableQuestion;
  fieldPrefix: string;
}) {
  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <label htmlFor={`${fieldPrefix}-chapter`} className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
            章节名称
          </label>
          <input
            id={`${fieldPrefix}-chapter`}
            aria-label="章节名称"
            type="text"
            value={question.chapter || ''}
            onChange={(event) =>
              controller.updateQuestion(question.id, (current) => ({
                ...current,
                chapter: event.target.value,
              }))
            }
            placeholder="例如：第一章 导论"
            className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
          />
        </div>
        <div>
          <label htmlFor={`${fieldPrefix}-type`} className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
            题目类型
          </label>
          <select
            id={`${fieldPrefix}-type`}
            value={question.type}
            onChange={(event) =>
              controller.changeQuestionType(question.id, event.target.value as QuestionType)
            }
            className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
          >
            {QUESTION_TYPE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div>
        <label htmlFor={`${fieldPrefix}-content`} className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
          题干
        </label>
        <textarea
          id={`${fieldPrefix}-content`}
          aria-label="题干"
          value={question.content}
          onChange={(event) =>
            controller.updateQuestion(question.id, (current) => ({
              ...current,
              content: event.target.value,
            }))
          }
          rows={3}
          placeholder="请输入题目内容"
          className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
        />
      </div>
    </>
  );
}

function AnswerEditor({
  controller,
  question,
  fieldPrefix,
}: {
  controller: ExtractController;
  question: EditableQuestion;
  fieldPrefix: string;
}) {
  if (question.type === 'blank') {
    return (
      <BlankAnswerEditor
        controller={controller}
        question={question}
        fieldPrefix={fieldPrefix}
      />
    );
  }

  if (question.type === 'judge') {
    return <JudgeAnswerEditor controller={controller} question={question} />;
  }

  return <ChoiceAnswerEditor controller={controller} question={question} />;
}

function BlankAnswerEditor({
  controller,
  question,
  fieldPrefix,
}: {
  controller: ExtractController;
  question: EditableQuestion;
  fieldPrefix: string;
}) {
  return (
    <div>
      <label htmlFor={`${fieldPrefix}-blank-answer`} className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
        正确答案
      </label>
      <input
        id={`${fieldPrefix}-blank-answer`}
        aria-label="正确答案"
        type="text"
        value={String(question.answer ?? '')}
        onChange={(event) => controller.setBlankAnswer(question.id, event.target.value)}
        placeholder="请输入填空题答案"
        className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
      />
    </div>
  );
}

function ChoiceAnswerEditor({
  controller,
  question,
}: {
  controller: ExtractController;
  question: EditableQuestion;
}) {
  const selectedSingleAnswer = extractChoiceIndexes(question.answer)[0];
  const selectedMultiAnswer = extractChoiceIndexes(question.answer);
  const selectedMultiAnswerSet = new Set(selectedMultiAnswer);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="block text-sm font-medium text-gray-700 dark:text-slate-200">
          选项与答案
        </span>
        <button
          type="button"
          onClick={() => controller.addOption(question.id)}
          className="inline-flex items-center gap-1 text-sm text-primary-600 hover:text-primary-700"
        >
          <Plus className="w-4 h-4" />
          新增选项
        </button>
      </div>

      {(question.options ?? []).map((option, optionIndex) => (
        <ChoiceOptionRow
          key={optionIndex}
          controller={controller}
          question={question}
          option={option}
          optionIndex={optionIndex}
          checked={
            question.type === 'single'
              ? selectedSingleAnswer === optionIndex
              : selectedMultiAnswerSet.has(optionIndex)
          }
        />
      ))}

      <div className="text-xs text-gray-400 dark:text-slate-500">
        当前答案：{question.answer || '未设置'}
      </div>
    </div>
  );
}

function ChoiceOptionRow({
  controller,
  question,
  option,
  optionIndex,
  checked,
}: {
  controller: ExtractController;
  question: EditableQuestion;
  option: string;
  optionIndex: number;
  checked: boolean;
}) {
  const optionLetter = letterFromIndex(optionIndex);

  return (
    <div className="flex items-center gap-3">
      <input
        aria-label={`设为答案 ${optionLetter}`}
        type={question.type === 'single' ? 'radio' : 'checkbox'}
        name={`answer-${question.id}`}
        checked={checked}
        onChange={() =>
          question.type === 'single'
            ? controller.setSingleAnswer(question.id, optionIndex)
            : controller.toggleMultiAnswer(question.id, optionIndex)
        }
        className="h-4 w-4"
      />
      <span className="w-6 text-sm font-medium text-gray-500 dark:text-slate-400">
        {optionLetter}.
      </span>
      <input
        aria-label={`选项 ${optionLetter} 内容`}
        type="text"
        value={option}
        onChange={(event) =>
          controller.updateQuestion(question.id, (current) => {
            const nextOptions = [...(current.options ?? [])];
            nextOptions[optionIndex] = event.target.value;
            return {
              ...current,
              options: nextOptions,
            };
          })
        }
        placeholder={`选项 ${optionLetter}`}
        className="flex-1 px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
      />
      <button
        type="button"
        onClick={() => controller.removeOption(question.id, optionIndex)}
        className="text-gray-400 dark:text-slate-500 hover:text-red-500 disabled:opacity-40"
        disabled={(question.options ?? []).length <= 2}
        aria-label={`删除选项 ${optionLetter}`}
      >
        <Trash2 className="w-4 h-4" />
      </button>
    </div>
  );
}

function JudgeAnswerEditor({
  controller,
  question,
}: {
  controller: ExtractController;
  question: EditableQuestion;
}) {
  return (
    <div>
      <div className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
        正确答案
      </div>
      <div className="grid grid-cols-2 gap-3">
        {(['对', '错'] as const).map((value) => (
          <button
            type="button"
            key={value}
            onClick={() => controller.setJudgeAnswer(question.id, value)}
            className={`py-3 rounded-xl border-2 font-medium transition-colors ${
              question.answer === value
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:text-primary-300'
                : 'border-gray-200 dark:border-slate-600 text-gray-600 dark:text-slate-300 hover:border-primary-300'
            }`}
          >
            {value}
          </button>
        ))}
      </div>
    </div>
  );
}

function QuestionAnalysisField({
  controller,
  question,
  fieldPrefix,
}: {
  controller: ExtractController;
  question: EditableQuestion;
  fieldPrefix: string;
}) {
  return (
    <div>
      <label htmlFor={`${fieldPrefix}-analysis`} className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
        解析（可选）
      </label>
      <textarea
        id={`${fieldPrefix}-analysis`}
        aria-label="解析"
        value={question.analysis || ''}
        onChange={(event) =>
          controller.updateQuestion(question.id, (current) => ({
            ...current,
            analysis: event.target.value,
          }))
        }
        rows={3}
        placeholder="可以手写解析，也可以稍后用 AI 批量补齐"
        className="w-full px-3 py-2 border border-gray-200 dark:border-slate-600 rounded-lg"
      />
    </div>
  );
}
