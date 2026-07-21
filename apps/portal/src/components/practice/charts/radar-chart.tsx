import { RADAR_DATA, r2 } from "@/lib/practice/mock";

const SIZE = 240;
const C = SIZE / 2;
const R = 84;

function polar(index: number, total: number, radius: number) {
  const a = (Math.PI * 2 * index) / total - Math.PI / 2;
  return { x: r2(C + Math.cos(a) * radius), y: r2(C + Math.sin(a) * radius) };
}

/** 科目雷达图（手写 SVG，accent 半透明数据多边形） */
export default function RadarChart() {
  const n = RADAR_DATA.length;
  const rings = [1 / 3, 2 / 3, 1];
  const dataPoints = RADAR_DATA.map(
    (d, i) => `${polar(i, n, (d.value / 100) * R).x},${polar(i, n, (d.value / 100) * R).y}`
  ).join(" ");

  return (
    <svg viewBox={`0 0 ${SIZE} ${SIZE}`} className="mx-auto w-full max-w-xs" role="img" aria-label="科目能力雷达图">
      {/* 网格 */}
      {rings.map((f) => (
        <polygon
          key={f}
          points={RADAR_DATA.map((_, i) => {
            const p = polar(i, n, R * f);
            return `${p.x},${p.y}`;
          }).join(" ")}
          fill="none"
          className="stroke-line"
          strokeWidth="1"
        />
      ))}
      {/* 轴线 + 标签 */}
      {RADAR_DATA.map((d, i) => {
        const edge = polar(i, n, R);
        const label = polar(i, n, R + 16);
        return (
          <g key={d.subject}>
            <line x1={C} y1={C} x2={edge.x} y2={edge.y} className="stroke-line" strokeWidth="1" />
            <text
              x={label.x}
              y={label.y}
              textAnchor="middle"
              dominantBaseline="middle"
              className="fill-ink/60"
              fontSize="9"
            >
              {d.subject}
            </text>
          </g>
        );
      })}
      {/* 数据多边形 */}
      <polygon
        data-radar-poly
        points={dataPoints}
        className="fill-accent/25 stroke-accent"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
      {RADAR_DATA.map((d, i) => {
        const p = polar(i, n, (d.value / 100) * R);
        return <circle key={d.subject} cx={p.x} cy={p.y} r="2.5" className="fill-accent" />;
      })}
    </svg>
  );
}
