import { useCallback, useEffect, useReducer, useRef } from 'react';
import {
  Dices,
  Globe,
  Plus,
  RotateCcw,
  UploadCloud,
  X,
} from 'lucide-react';
import { userApi, wheelApi } from '@/api/client';

const DEFAULT_ITEMS = [
  '板面',
  '香扒饭',
  '摇滚炒鸡',
  '盖浇饭',
  '烤肉拌饭',
  '麻辣烫',
  '麦当劳',
];

const COLORS = [
  '#FF6384',
  '#36A2EB',
  '#FFCE56',
  '#4BC0C0',
  '#9966FF',
  '#FF9F40',
  '#C9CBCF',
  '#E8A0BF',
  '#73C6B6',
  '#F0B27A',
  '#85C1E9',
  '#BB8FCE',
];

type PublicWheel = {
  id: number | string;
  owner_user_id: string;
  owner_name: string;
  items: string[];
  updated_at: string;
  is_public: boolean;
};

function drawWheel(
  canvas: HTMLCanvasElement,
  items: string[],
  rotation: number,
) {
  const ctx = canvas.getContext('2d');
  if (!ctx || items.length === 0) return;

  const dpr = window.devicePixelRatio || 1;
  const size = canvas.width / dpr;
  const cx = size / 2;
  const cy = size / 2;
  const radius = size / 2 - 8;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.save();
  ctx.translate(cx * dpr, cy * dpr);
  ctx.rotate(rotation);
  ctx.translate(-cx * dpr, -cy * dpr);

  const arc = (2 * Math.PI) / items.length;
  const fontSize = Math.max(12, Math.min(16, radius / (items.length * 0.12 + 1.5)));

  for (let i = 0; i < items.length; i++) {
    const startAngle = i * arc - Math.PI / 2;
    const endAngle = startAngle + arc;

    ctx.beginPath();
    ctx.moveTo(cx * dpr, cy * dpr);
    ctx.arc(cx * dpr, cy * dpr, radius * dpr, startAngle, endAngle);
    ctx.closePath();
    ctx.fillStyle = COLORS[i % COLORS.length];
    ctx.fill();
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 2 * dpr;
    ctx.stroke();

    ctx.save();
    ctx.translate(cx * dpr, cy * dpr);
    ctx.rotate(startAngle + arc / 2);
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';
    ctx.fillStyle = '#ffffff';
    ctx.font = `bold ${fontSize * dpr}px "PingFang SC","Microsoft YaHei",sans-serif`;

    const textRadius = radius * 0.65;
    const displayText = items[i].length > 6 ? items[i].slice(0, 5) + '…' : items[i];
    ctx.fillText(displayText, textRadius * dpr, 0);

    ctx.restore();
  }

  ctx.restore();

  ctx.beginPath();
  ctx.arc(cx * dpr, cy * dpr, 36 * dpr, 0, 2 * Math.PI);
  ctx.fillStyle = '#ffffff';
  ctx.fill();
  ctx.strokeStyle = '#e5e7eb';
  ctx.lineWidth = 2 * dpr;
  ctx.stroke();
}

function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3);
}

function formatTime(value: string): string {
  const time = value ? new Date(value) : null;
  if (!time || Number.isNaN(time.getTime())) {
    return value || '';
  }
  return time.toLocaleString();
}

function getOwnerLabel(wheel: PublicWheel) {
  const name = (wheel.owner_name || '').trim();
  return {
    name: name || wheel.owner_user_id,
    id: wheel.owner_user_id,
  };
}

async function ensureUserId(): Promise<string> {
  const cachedUserId = typeof window === 'undefined' ? '' : (localStorage.getItem('user_id')?.trim() || '');
  if (cachedUserId) {
    return cachedUserId;
  }

  const user = await userApi.ensureUser();
  return user.userId;
}

const syncDialog = (
  dialog: HTMLDialogElement | null,
  open: boolean,
) => {
  if (!dialog) return;
  if (open && !dialog.open) {
    dialog.showModal();
  } else if (!open && dialog.open) {
    dialog.close();
  }
};

type FoodWheelUiState = {
  draftItems: string[];
  newItem: string;
  publicWheels: PublicWheel[];
  loading: boolean;
  loadingPublic: boolean;
  spinning: boolean;
  publishing: boolean;
  result: string | null;
  selectedOwner: string;
  errorMessage: string;
};

const initialFoodWheelUiState: FoodWheelUiState = {
  draftItems: [...DEFAULT_ITEMS],
  newItem: '',
  publicWheels: [],
  loading: true,
  loadingPublic: false,
  spinning: false,
  publishing: false,
  result: null,
  selectedOwner: '我的草稿转盘',
  errorMessage: '',
};

const mergeFoodWheelUiState = (
  state: FoodWheelUiState,
  updates: Partial<FoodWheelUiState>,
) => ({
  ...state,
  ...updates,
});

function useFoodWheelController() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const resultDialogRef = useRef<HTMLDialogElement | null>(null);
  const [ui, setUi] = useReducer(
    mergeFoodWheelUiState,
    initialFoodWheelUiState,
  );
  const {
    draftItems,
    newItem,
    publicWheels,
    loading,
    loadingPublic,
    spinning,
    publishing,
    result,
    selectedOwner,
    errorMessage,
  } = ui;
  const rotationRef = useRef(0);
  const animFrameRef = useRef<number | null>(null);
  const compositionRef = useRef(false);

  const initCanvas = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const size = Math.min(360, window.innerWidth - 48);
    const dpr = window.devicePixelRatio || 1;
    canvas.style.width = `${size}px`;
    canvas.style.height = `${size}px`;
    canvas.width = size * dpr;
    canvas.height = size * dpr;
    drawWheel(canvas, draftItems, rotationRef.current);
  }, [draftItems]);

  const fetchPublicWheels = useCallback(async () => {
    try {
      setUi({ loadingPublic: true });
      const payload = await wheelApi.get();
      setUi({ publicWheels: Array.isArray(payload.wheels) ? payload.wheels : [] });
    } catch {
      setUi({ publicWheels: [] });
    } finally {
      setUi({ loadingPublic: false });
    }
  }, []);

  useEffect(() => {
    let mounted = true;

    const load = async () => {
      setUi({ loading: true });
      await fetchPublicWheels();
      if (mounted) {
        setUi({ loading: false });
      }
    };

    void load();
    return () => {
      mounted = false;
    };
  }, [fetchPublicWheels]);

  useEffect(() => {
    initCanvas();
  }, [initCanvas]);

  useEffect(() => {
    const handleResize = () => {
      initCanvas();
    };
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [initCanvas]);

  const cancelSpinAnimation = useCallback(() => {
    if (animFrameRef.current !== null) {
      cancelAnimationFrame(animFrameRef.current);
      animFrameRef.current = null;
    }
  }, []);

  useEffect(() => cancelSpinAnimation, [cancelSpinAnimation]);

  useEffect(() => {
    syncDialog(resultDialogRef.current, result !== null);
  }, [result]);

  const setDraftAndRedraw = (nextItems: string[]) => {
    setUi({ draftItems: nextItems });
    rotationRef.current = 0;
    const canvas = canvasRef.current;
    if (canvas) {
      drawWheel(canvas, nextItems, 0);
    }
  };

  const loadPublicWheel = (wheel: PublicWheel) => {
    const list = Array.isArray(wheel.items) && wheel.items.length > 0
      ? wheel.items
      : [...DEFAULT_ITEMS];
    setUi({ selectedOwner: `查看：${getOwnerLabel(wheel).name}` });
    setDraftAndRedraw(list);
  };

  const handleSpin = () => {
    if (spinning || draftItems.length < 2) return;
    setUi({ errorMessage: '', result: null, spinning: true });

    const totalRotation =
      (Math.random() * 720 + 1800) * (Math.PI / 180); // 5-7 full rotations
    const duration = 3000 + Math.random() * 1500;
    const startRotation = rotationRef.current;
    const startTime = performance.now();

    const animate = (now: number) => {
      const elapsed = now - startTime;
      const progress = Math.min(elapsed / duration, 1);
      const easedProgress = easeOutCubic(progress);
      const currentRotation = startRotation + totalRotation * easedProgress;

      rotationRef.current = currentRotation;
      const canvas = canvasRef.current;
      if (canvas) {
        drawWheel(canvas, draftItems, currentRotation);
      }

      if (progress < 1) {
        animFrameRef.current = requestAnimationFrame(animate);
      } else {
        setUi({ spinning: false });
        const normalized = (-currentRotation % (2 * Math.PI) + 2 * Math.PI) % (2 * Math.PI);
        const arc = (2 * Math.PI) / draftItems.length;
        const index = Math.min(draftItems.length - 1, Math.floor(normalized / arc));
        setUi({ result: draftItems[index] });
      }
    };

    animFrameRef.current = requestAnimationFrame(animate);
  };

  const addItem = () => {
    const trimmed = newItem.trim();
    if (!trimmed) return;
    if (draftItems.includes(trimmed)) return;

    const nextItems = [...draftItems, trimmed];
    setDraftAndRedraw(nextItems);
    setUi({ newItem: '' });
  };

  const removeItem = (index: number) => {
    if (draftItems.length <= 2) return;
    const nextItems = draftItems.filter((_, i) => i !== index);
    setDraftAndRedraw(nextItems);
  };

  const resetItems = () => {
    setDraftAndRedraw([...DEFAULT_ITEMS]);
    setUi({ selectedOwner: '我的草稿转盘' });
  };

  const uploadDraft = async () => {
    if (publishing || draftItems.length < 2) {
      setUi({ errorMessage: '转盘至少要有 2 个选项才可上传' });
      return;
    }

    try {
      setUi({ publishing: true, errorMessage: '' });
      const userId = await ensureUserId();
      await wheelApi.save(draftItems, userId).then(fetchPublicWheels);
      setUi({ selectedOwner: `我的草稿转盘（已上传）${userId ? ` · ${userId}` : ''}` });
      alert('上传成功，已加入公共转盘');
    } catch (error) {
      const message =
        error instanceof Error && error.message
          ? error.message
          : '上传失败，请先检查网络或登录信息';
      if (message.includes('user_id')) {
        setUi({ errorMessage: '上传失败，未能获取到当前用户信息，请先刷新页面后重试' });
      } else if (message.includes('网络') || message.includes('Network') || message.includes('Failed to fetch')) {
        setUi({ errorMessage: '上传失败，请先检查网络' });
      } else {
        setUi({ errorMessage: `上传失败：${message}` });
      }
    } finally {
      setUi({ publishing: false });
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !compositionRef.current) {
      addItem();
    }
  };

  return {
    canvasRef,
    resultDialogRef,
    compositionRef,
    draftItems,
    newItem,
    publicWheels,
    loading,
    loadingPublic,
    spinning,
    publishing,
    result,
    selectedOwner,
    errorMessage,
    setUi,
    setDraftAndRedraw,
    loadPublicWheel,
    handleSpin,
    addItem,
    removeItem,
    resetItems,
    uploadDraft,
    handleKeyDown,
  };
}

type FoodWheelController = ReturnType<typeof useFoodWheelController>;

export default function FoodWheel() {
  const controller = useFoodWheelController();

  return <FoodWheelView controller={controller} />;
}

function FoodWheelView({ controller }: { controller: FoodWheelController }) {
  return (
    <div className="max-w-3xl mx-auto animate-fade-in">
      <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100 mb-6 flex items-center gap-2">
        <Dices className="w-6 h-6 text-primary-500 dark:text-primary-400" />
        美食推荐转盘
      </h1>

      <div className="card py-3 text-center text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500">
        {controller.loading ? '正在加载公共转盘...' : '默认仅显示上传到公共区的转盘，草稿编辑不会自动公开'}
      </div>

      <WheelPanel controller={controller} />
      <FoodWheelResultDialog controller={controller} />
      <PublicWheelList controller={controller} />
      <DraftEditor controller={controller} />
    </div>
  );
}

function WheelPanel({ controller }: { controller: FoodWheelController }) {
  const {
    canvasRef,
    draftItems,
    errorMessage,
    handleSpin,
    loading,
    publishing,
    selectedOwner,
    spinning,
    uploadDraft,
  } = controller;

  return (
    <div className="card flex flex-col items-center py-6 mb-6">
      <div className="text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500 mb-3">当前编辑：{selectedOwner}</div>
      <div className="relative">
        <div className="absolute -top-2 left-1/2 -translate-x-1/2 z-10">
          <div
            className="w-0 h-0"
            style={{
              borderLeft: '12px solid transparent',
              borderRight: '12px solid transparent',
              borderTop: '20px solid #1976d2',
              filter: 'drop-shadow(0 2px 2px rgba(0,0,0,0.2))',
            }}
          />
        </div>
        <canvas ref={canvasRef} className="block" />
        <button
          type="button"
          onClick={handleSpin}
          disabled={spinning || draftItems.length < 2}
          className="absolute top-1/2 left-1/2 z-10 flex h-[72px] w-[72px] -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-primary-50 dark:bg-primary-900/300 text-sm font-bold text-white shadow-lg transition-all hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {spinning ? (
            <span className="animate-spin">
              <Dices className="w-5 h-5" />
            </span>
          ) : (
            '转！'
          )}
        </button>
      </div>

      {draftItems.length < 2 && !loading ? (
        <p className="text-sm text-red-500 mt-4">至少需要 2 个选项才能转</p>
      ) : null}

      <div className="mt-4 flex flex-wrap gap-2 justify-center">
        <button
          type="button"
          onClick={uploadDraft}
          disabled={publishing || draftItems.length < 2}
          className="btn-primary"
        >
          <UploadCloud className="w-4 h-4 mr-1 inline" />
          {publishing ? '上传中...' : '上传到公共区域'}
        </button>
        <button type="button" onClick={handleSpin} className="btn-secondary">
          再来一次
        </button>
      </div>

      {errorMessage ? <p className="text-sm text-red-500 mt-3">{errorMessage}</p> : null}
    </div>
  );
}

function FoodWheelResultDialog({ controller }: { controller: FoodWheelController }) {
  const { draftItems, handleSpin, result, resultDialogRef, setUi } = controller;

  return (
    <dialog
      ref={resultDialogRef}
      aria-labelledby="food-wheel-result-title"
      onCancel={() => setUi({ result: null })}
      onClose={() => setUi({ result: null })}
      className="w-[calc(100%-2rem)] max-w-sm rounded-2xl bg-white dark:bg-slate-800 p-6 text-center shadow-xl backdrop:bg-black/50"
    >
      {result !== null && (
        <>
          <button
            type="button"
            onClick={() => setUi({ result: null })}
            className="float-right rounded-lg p-1.5 text-gray-500 dark:text-slate-400 dark:text-slate-500 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
            aria-label="关闭"
          >
            <X className="h-5 w-5" />
          </button>
          <div className="text-5xl mb-4">🎉</div>
          <h2 id="food-wheel-result-title" className="text-xl font-bold text-gray-800 dark:text-slate-100 mb-2">抽中了</h2>
          <p
            className="text-3xl font-extrabold mb-6"
            style={{ color: COLORS[draftItems.indexOf(result) % COLORS.length] }}
          >
            {result}！
          </p>
          <button
            type="button"
            onClick={() => {
              setUi({ result: null });
              handleSpin();
            }}
            className="btn-primary w-full text-base"
          >
            再来一次
          </button>
        </>
      )}
    </dialog>
  );
}

function PublicWheelList({ controller }: { controller: FoodWheelController }) {
  const { loadPublicWheel, loadingPublic, publicWheels } = controller;

  return (
    <div className="card mb-6">
      <h2 className="text-base font-semibold text-gray-800 dark:text-slate-100 mb-4 flex items-center gap-2">
        <Globe className="w-4 h-4 text-primary-500 dark:text-primary-400" />
        公共转盘（已上传）
      </h2>

      {loadingPublic ? (
        <p className="text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500">加载公共转盘中...</p>
      ) : publicWheels.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500">当前还没有公开转盘</p>
      ) : (
        <div className="space-y-3">
          {publicWheels.map((wheel) => (
            <PublicWheelCard
              key={`${wheel.owner_user_id}-${wheel.id}`}
              wheel={wheel}
              onLoad={loadPublicWheel}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function PublicWheelCard({
  onLoad,
  wheel,
}: {
  onLoad: (wheel: PublicWheel) => void;
  wheel: PublicWheel;
}) {
  const owner = getOwnerLabel(wheel);

  return (
    <div className="rounded-xl border border-gray-100 dark:border-slate-700 bg-white dark:bg-slate-800 p-3">
      <div className="text-sm text-gray-700 dark:text-slate-200">
        <div className="font-semibold">{owner.name}</div>
        <div className="text-xs text-gray-500 dark:text-slate-400 dark:text-slate-500">ID: {wheel.id} · {owner.id}</div>
        <div className="text-xs text-gray-500 dark:text-slate-400 dark:text-slate-500">更新时间：{formatTime(wheel.updated_at)}</div>
      </div>
      <div className="text-xs text-gray-600 dark:text-slate-300 mt-2">
        选项预览：{wheel.items.slice(0, 3).join('、')}
        {wheel.items.length > 3 ? '…' : ''}
        （共 {wheel.items.length} 个）
      </div>
      <button
        type="button"
        onClick={() => onLoad(wheel)}
        className="mt-3 btn-secondary text-xs"
      >
        <RotateCcw className="w-3 h-3 mr-1" />
        加载并查看
      </button>
    </div>
  );
}

function DraftEditor({ controller }: { controller: FoodWheelController }) {
  const {
    addItem,
    compositionRef,
    draftItems,
    handleKeyDown,
    newItem,
    removeItem,
    resetItems,
    setDraftAndRedraw,
    setUi,
  } = controller;

  return (
    <div className="card mb-6">
      <h2 className="text-base font-semibold text-gray-800 dark:text-slate-100 mb-3 flex items-center gap-2">
        <Plus className="w-4 h-4 text-primary-500 dark:text-primary-400" />
        草稿编辑（未上传不显示）
      </h2>

      <div className="flex items-center gap-2 mb-4">
        <input
          aria-label="新增菜名"
          value={newItem}
          onChange={(e) => setUi({ newItem: e.target.value })}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => {
            compositionRef.current = true;
          }}
          onCompositionEnd={(e) => {
            compositionRef.current = false;
            setUi({ newItem: (e.target as HTMLInputElement).value });
          }}
          placeholder="输入菜名"
          className="flex-1 px-3 py-2 border border-gray-200 dark:border-slate-700 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-transparent"
        />
        <button
          type="button"
          onClick={addItem}
          className="btn-secondary whitespace-nowrap"
        >
          添加
        </button>
      </div>

      <ul className="space-y-2 mb-4">
        {draftItems.map((item, index) => (
          <li
            key={item}
            className="flex items-center justify-between bg-gray-50 dark:bg-slate-700 rounded-lg px-3 py-2"
          >
            <span className="text-sm text-gray-700 dark:text-slate-200">{item}</span>
            <button
              type="button"
              onClick={() => removeItem(index)}
              className="text-gray-500 dark:text-slate-400 dark:text-slate-500 hover:text-red-500 transition-colors"
            >
              删除
            </button>
          </li>
        ))}
      </ul>

      <div className="flex flex-wrap gap-2">
        <button type="button" onClick={resetItems} className="btn-secondary">
          <RotateCcw className="w-4 h-4 mr-1" />
          恢复默认
        </button>
        <button
          type="button"
          onClick={() => setDraftAndRedraw(DEFAULT_ITEMS)}
          className="btn-secondary"
        >
          清空到默认
        </button>
      </div>
    </div>
  );
}
