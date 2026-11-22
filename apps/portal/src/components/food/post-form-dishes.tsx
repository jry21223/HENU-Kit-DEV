export const MAX_DISHES = 6;

export interface DishDraft {
  name: string;
  price: string;
  reason: string;
}

export default function PostFormDishes({ dishes, addDish, updateDish, removeDish }: {
  dishes: DishDraft[];
  addDish: () => void;
  updateDish: (index: number, patch: Partial<DishDraft>) => void;
  removeDish: (index: number) => void;
}) {
  return (
    <section className="border-b border-line py-6 md:py-8">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs text-ink/60">
            推荐菜品（可选）
          </p>
          <p className="mt-1 font-mono text-xs text-ink/60">
            至多 {MAX_DISHES} 道；菜名必填，价格与理由可留空。
          </p>
        </div>
        {dishes.length < MAX_DISHES && (
          <button
            type="button"
            onClick={addDish}
            className="min-h-11 border border-line px-3 py-1.5 font-mono text-xs transition-colors hover:border-ink"
          >
            + 加一道
          </button>
        )}
      </div>
      <div className="mt-4 space-y-3">
        {dishes.map((dish, index) => (
          <div
            key={index}
            className="grid gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,2fr)_auto]"
          >
            <input
              aria-label={`菜品 ${index + 1} 名称`}
              maxLength={80}
              value={dish.name}
              onChange={(event) =>
                updateDish(index, { name: event.target.value })
              }
              placeholder="菜名（必填）"
              className="min-h-11 w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/60 focus:border-ink"
            />
            <input
              aria-label={`菜品 ${index + 1} 价格`}
              maxLength={40}
              value={dish.price}
              onChange={(event) =>
                updateDish(index, { price: event.target.value })
              }
              placeholder="参考价格"
              className="min-h-11 w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/60 focus:border-ink"
            />
            <input
              aria-label={`菜品 ${index + 1} 理由`}
              maxLength={200}
              value={dish.reason}
              onChange={(event) =>
                updateDish(index, { reason: event.target.value })
              }
              placeholder="推荐理由"
              className="min-h-11 w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/60 focus:border-ink"
            />
            <button
              type="button"
              onClick={() => removeDish(index)}
              className="inline-flex min-h-11 min-w-11 items-center justify-center self-center font-mono text-xs text-ink/60 transition-colors hover:text-accent-text"
              aria-label={`删除菜品 ${index + 1}`}
            >
              ×
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}
