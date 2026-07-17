import { useEffect, useReducer } from 'react';
import { useNavigate } from 'react-router-dom';
import { BookOpen } from 'lucide-react';
import { bankApi, practiceApi, userApi } from '@/api/client';
import { useQuizStore } from '@/stores/quizStore';
import type { PracticeMode } from '@/types';
import { IS_OPS_MODE } from '@/config/appMode';
import {
  PracticeBankSelector,
  PracticeIdentity,
  PracticeModeSelector,
  PracticeSettings,
} from '@/components/practice/PracticeFormSections';

const readStoredUserId = () => {
  if (typeof window === 'undefined') return '';
  return localStorage.getItem('user_id')?.trim() || '';
};

type PracticeUiState = {
  mode: PracticeMode;
  count: number;
  unlimitedCount: boolean;
  chapterId: string;
  threshold: number;
  loading: boolean;
  displayUserId: string;
  userIdInput: string;
};

const createInitialPracticeUiState = (): PracticeUiState => ({
  mode: 'random',
  count: 20,
  unlimitedCount: false,
  chapterId: '',
  threshold: 50,
  loading: false,
  displayUserId: IS_OPS_MODE ? '' : readStoredUserId(),
  userIdInput: IS_OPS_MODE ? readStoredUserId() : '',
});

const mergePracticeUiState = (
  state: PracticeUiState,
  updates: Partial<PracticeUiState>,
) => ({
  ...state,
  ...updates,
});

export default function Practice() {
  const navigate = useNavigate();
  const { 
    banks, 
    setBanks, 
    setUser, 
    currentBank, 
    setCurrentBank, 
    startPractice 
  } = useQuizStore();
  
  const [ui, setUi] = useReducer(
    mergePracticeUiState,
    undefined,
    createInitialPracticeUiState,
  );
  const {
    mode,
    count,
    unlimitedCount,
    chapterId,
    threshold,
    loading,
    displayUserId,
    userIdInput,
  } = ui;
  
  useEffect(() => {
    bankApi.getList().then((res) => {
      setBanks(res.banks);
      const activeBank = useQuizStore.getState().currentBank;
      const activeBankExists = res.banks.some((bank) => bank.key === activeBank);
      if (res.banks.length > 0 && (!activeBank || !activeBankExists)) {
        setCurrentBank(res.banks[0].key);
      }
    }).catch(() => {
      // Mock data for local development preview
      setBanks([
        { key: 'java', name: 'Java 题库', total: 286, chapters: [{ id: 'ch1', name: '基础语法' }, { id: 'ch2', name: '面向对象' }, { id: 'ch3', name: '集合框架' }], color: '#E57373' },
        { key: 'web', name: 'Web 前端', total: 142, chapters: [{ id: 'ch1', name: 'HTML/CSS' }, { id: 'ch2', name: 'JavaScript' }], color: '#64B5F6' },
        { key: 'python', name: 'Python 题库', total: 198, chapters: [{ id: 'ch1', name: '基础语法' }, { id: 'ch2', name: '数据结构' }], color: '#81C784' },
        { key: 'os', name: '操作系统', total: 105, chapters: [{ id: 'ch1', name: '进程管理' }, { id: 'ch2', name: '内存管理' }], color: '#FFB74D' },
      ] as any);
      const activeBank = useQuizStore.getState().currentBank;
      if (!activeBank) setCurrentBank('java');
    });
  }, [setBanks, setCurrentBank]);

  const currentBankData = banks.find((b) => b.key === currentBank);
  const handleStart = async () => {
    if (!currentBank) return;

    if (IS_OPS_MODE) {
      const normalizedId = userIdInput.trim();
      if (!normalizedId) {
        alert('请输入ID');
        return;
      }
    }

    setUi({ loading: true });
    try {
      const normalizedUserId = userIdInput.trim();
      const savedUserId = readStoredUserId();

      if (IS_OPS_MODE) {
        localStorage.setItem('user_id', normalizedUserId);
      } else {
        if (!savedUserId) {
          const user = await userApi.ensureUser();
          setUser(user);
          setUi({ displayUserId: user.userId });
        }
      }

      let questions: any[];
      try {
        const requestCount = mode === 'chapter' || (mode === 'random' && unlimitedCount) ? 0 : count;
        const result = await practiceApi.start(currentBank, {
          mode,
          count: requestCount,
          chapterId: mode === 'chapter' ? chapterId : undefined,
          threshold: mode === 'hard' ? threshold : undefined,
        });
        questions = result.questions;
      } catch {
        // Mock questions for local dev preview
        questions = Array.from({ length: count }, (_, i) => ({
          id: `preview-${i}`,
          type: 'single' as const,
          chapter: '模拟章节',
          chapter_id: 'mock-ch1',
          content: `以下 Java 代码的输出是什么？\n\n\`\`\`java\npublic class Test {\n    public static void main(String[] args) {\n        String s1 = "hello";\n        String s2 = new String("hello");\n        System.out.println(s1 == s2);\n    }\n}\n\`\`\`\n\n提示：\`==\` 比较的是 **引用地址**，\`.equals()\` 比较的是 **内容**。`,
          options: ['`true` — 因为字符串内容相同', '`false` — 因为 `new String()` 创建了新对象', '编译错误 — `==` 不能用于字符串比较', '运行时异常'],
          answer: 1,
          analysis: '正确答案是 **B**。\n\n解析：`s1` 指向 *字符串常量池* 中的 `"hello"`，而 `s2` 通过 `new String()` 在 *堆内存* 中创建了一个 **新对象**。因此 `==` 比较引用地址时返回 `false`。\n\n```java\nString s1 = "hello";           // 常量池\nString s2 = new String("hello"); // 堆内存新对象\n\ns1 == s2       // false — 不同引用\ns1.equals(s2)  // true  — 内容相同\n```\n\n**最佳实践**：始终使用 `.equals()` 比较字符串内容。',
        }));
      }

      startPractice(questions, currentBank);
      navigate('/quiz');
    } catch (error) {
      alert('开始练习失败: ' + (error as Error).message);
    } finally {
      setUi({ loading: false });
    }
  };
  
  return (
    <div className="max-w-2xl mx-auto animate-fade-in">
      <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100 mb-6 flex items-center gap-2">
        <BookOpen className="w-6 h-6 text-primary-500 dark:text-primary-400" />
        开始练习
      </h1>
      
      <PracticeIdentity displayUserId={displayUserId} userIdInput={userIdInput} onUserIdChange={(value) => setUi({ userIdInput: value })} />
      <PracticeBankSelector banks={banks} currentBank={currentBank} onSelect={setCurrentBank} />
      <PracticeModeSelector mode={mode} onSelect={(value) => setUi({ mode: value })} />
      <PracticeSettings
        mode={mode}
        count={count}
        unlimitedCount={unlimitedCount}
        chapterId={chapterId}
        threshold={threshold}
        currentBank={currentBankData}
        onCountChange={(value) => setUi({ count: value })}
        onUnlimitedCountChange={(value) => setUi({ unlimitedCount: value })}
        onChapterChange={(value) => setUi({ chapterId: value })}
        onThresholdChange={(value) => setUi({ threshold: value })}
      />
      
      {/* 开始按钮 */}
      <button
        type="button"
        onClick={handleStart}
        disabled={loading || (mode === 'chapter' && !chapterId) || (IS_OPS_MODE && !userIdInput.trim())}
        className="w-full py-3 bg-primary-500 text-white font-medium rounded-xl hover:bg-primary-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        {loading ? '加载中...' : '开始练习'}
      </button>
    </div>
  );
}
