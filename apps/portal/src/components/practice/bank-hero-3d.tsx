"use client";

/**
 * Practice bank hero 3D — A/B variants (no GLB).
 *
 * A  Knowledge mesh driven by mastery snapshot (rings / core / orbit)
 * B  Answer sheet: stacked cards + check marks + pencil (quiz metaphor)
 */

import { useMemo, useRef } from "react";
import { Canvas, useFrame } from "@react-three/fiber";
import { Float } from "@react-three/drei";
import * as THREE from "three";

const INK = "#161513";
const ACCENT = "#ff4d00";
const PAPER = "#f2f0ea";

export type BankHeroVariant = "A" | "B";

/** 0–100 subject mastery row (same shape as USER_STATS.mastery). */
export type MasterySubject = {
  label: string;
  value: number;
};

/** Snapshot that drives Variant A. */
export type MasterySnapshot = {
  subjects: MasterySubject[];
  /** Overall accuracy 0–100 → nucleus size / brightness */
  accuracy: number;
  /** Streak days → orbiting cube count */
  streakDays: number;
  /** Total answered → outer mesh density feel via opacity */
  totalQuestions: number;
};

export const EMPTY_MASTERY: MasterySnapshot = {
  subjects: [],
  accuracy: 0,
  streakDays: 0,
  totalQuestions: 0,
};

function clamp01(n: number) {
  return Math.min(1, Math.max(0, n));
}

function pct(n: number) {
  return clamp01(n / 100);
}

/** Slow yaw + pointer lean — same feel as homepage hero-3d. */
function Rig({
  children,
  spin = 0.14,
}: {
  children: React.ReactNode;
  spin?: number;
}) {
  const group = useRef<THREE.Group>(null);
  useFrame((state, delta) => {
    const g = group.current;
    if (!g) return;
    g.rotation.y += delta * spin;
    g.rotation.x = THREE.MathUtils.lerp(
      g.rotation.x,
      state.pointer.y * -0.12,
      0.05
    );
    g.rotation.z = THREE.MathUtils.lerp(
      g.rotation.z,
      state.pointer.x * 0.06,
      0.05
    );
  });
  return <group ref={group}>{children}</group>;
}

function OrbitingCubes({
  count,
  radius = 1.85,
  speed = 0.28,
}: {
  count: number;
  radius?: number;
  speed?: number;
}) {
  const group = useRef<THREE.Group>(null);
  useFrame((_, delta) => {
    if (group.current) group.current.rotation.y += delta * speed;
  });
  const cubes = useMemo(() => {
    const n = Math.max(0, Math.min(6, Math.round(count)));
    return Array.from({ length: n }, (_, i) => {
      const angle = n === 0 ? 0 : (Math.PI * 2 * i) / n;
      return {
        angle,
        y: (i % 2 === 0 ? 0.35 : -0.4) + (i - 1) * 0.12,
        size: 0.11 + (i % 3) * 0.04,
        r: radius + (i % 2) * 0.18,
      };
    });
  }, [count, radius]);
  if (cubes.length === 0) return null;
  return (
    <group ref={group}>
      {cubes.map((c, i) => (
        <mesh
          key={i}
          position={[
            Math.cos(c.angle) * c.r,
            c.y,
            Math.sin(c.angle) * c.r,
          ]}
        >
          <boxGeometry args={[c.size, c.size, c.size]} />
          <meshBasicMaterial wireframe color={ACCENT} />
        </mesh>
      ))}
    </group>
  );
}

/* ─── Variant A: mastery-driven knowledge mesh ────────────────────────── */

function KnowledgeCore({
  accuracy,
  coverage,
}: {
  accuracy: number;
  /** average subject mastery 0–1 */
  coverage: number;
}) {
  const acc = pct(accuracy);
  // Nucleus grows with accuracy; outer cage densifies with coverage
  const nucleusScale = 0.1 + acc * 0.22;
  const outerOpacity = 0.18 + coverage * 0.35;
  const innerOpacity = 0.12 + coverage * 0.28;
  const innerScale = 0.48 + coverage * 0.2;

  return (
    <Float speed={1.0 + acc * 0.6} rotationIntensity={0.15 + acc * 0.15} floatIntensity={0.4 + acc * 0.35}>
      <mesh>
        <icosahedronGeometry args={[1.15, coverage > 0.55 ? 1 : 0]} />
        <meshBasicMaterial
          wireframe
          color={INK}
          transparent
          opacity={outerOpacity}
        />
      </mesh>
      <mesh scale={innerScale}>
        <icosahedronGeometry args={[1.15, 0]} />
        <meshBasicMaterial
          wireframe
          color={INK}
          transparent
          opacity={innerOpacity}
        />
      </mesh>
      <mesh scale={nucleusScale}>
        <octahedronGeometry args={[1, 0]} />
        <meshBasicMaterial
          wireframe
          color={ACCENT}
          transparent
          opacity={0.45 + acc * 0.5}
        />
      </mesh>
    </Float>
  );
}

/**
 * One ring per subject (max 3).
 * - radius tier fixed (readable stack)
 * - tube thickness + opacity scale with mastery %
 * - weak (<60) → accent; else ink
 * - spin speed slightly higher when stronger
 */
function MasteryRings({ subjects }: { subjects: MasterySubject[] }) {
  const group = useRef<THREE.Group>(null);
  const top = useMemo(() => {
    // Keep original order (matches stats list) but only first 3 for visual stack
    return subjects.slice(0, 3).map((s, i) => ({
      ...s,
      value: Math.min(100, Math.max(0, s.value)),
      baseR: 1.4 + i * 0.28,
    }));
  }, [subjects]);

  const avg =
    top.length === 0
      ? 0
      : top.reduce((a, s) => a + s.value, 0) / top.length / 100;

  useFrame((_, delta) => {
    if (!group.current) return;
    // Stronger overall mastery → slightly snappier ring precession
    group.current.rotation.z += delta * (0.05 + avg * 0.1);
    group.current.rotation.x += delta * (0.02 + avg * 0.04);
  });

  if (top.length === 0) {
    // Empty state: faint placeholder ring
    return (
      <group rotation={[Math.PI / 2.6, 0.2, 0]}>
        <mesh>
          <torusGeometry args={[1.6, 0.01, 8, 64]} />
          <meshBasicMaterial color={INK} transparent opacity={0.12} />
        </mesh>
      </group>
    );
  }

  return (
    <group ref={group} rotation={[Math.PI / 2.6, 0.2, 0]}>
      {top.map((s, i) => {
        const t = pct(s.value);
        const weak = s.value < 60;
        const tube = 0.008 + t * 0.022;
        const opacity = 0.12 + t * 0.7;
        return (
          <mesh
            key={s.label}
            rotation={[i * 0.16, i * 0.32, 0]}
            // scale out slightly with mastery so strong subjects read larger
            scale={0.88 + t * 0.18}
          >
            <torusGeometry args={[s.baseR, tube, 10, 72]} />
            <meshBasicMaterial
              color={weak ? ACCENT : INK}
              transparent
              opacity={opacity}
            />
          </mesh>
        );
      })}
    </group>
  );
}

function VariantA({ mastery }: { mastery: MasterySnapshot }) {
  const coverage = useMemo(() => {
    const list = mastery.subjects;
    if (!list.length) return 0;
    return list.reduce((a, s) => a + s.value, 0) / list.length / 100;
  }, [mastery.subjects]);

  // streak → cubes: 0 days = 0, ~7d = 1, ~14d = 2 … cap 5
  const cubeCount = Math.min(
    5,
    Math.max(0, Math.floor(mastery.streakDays / 7) + (mastery.streakDays > 0 ? 1 : 0))
  );
  // more answered → slightly wider orbit
  const orbitR = 2.0 + Math.min(0.4, mastery.totalQuestions / 2000);

  return (
    <Rig spin={0.1 + coverage * 0.08}>
      <KnowledgeCore accuracy={mastery.accuracy} coverage={coverage} />
      <MasteryRings subjects={mastery.subjects} />
      <OrbitingCubes count={cubeCount} radius={orbitR} speed={0.2 + coverage * 0.15} />
    </Rig>
  );
}

/* ─── Variant B: Answer sheet stack ───────────────────────────────────── */

function Sheet({
  y,
  rotZ,
  depth,
  accentEdge = false,
}: {
  y: number;
  rotZ: number;
  depth: number;
  accentEdge?: boolean;
}) {
  return (
    <group position={[0, y, depth]} rotation={[0.08, 0.35, rotZ]}>
      <mesh>
        <boxGeometry args={[1.7, 2.15, 0.04]} />
        <meshBasicMaterial wireframe color={INK} transparent opacity={0.38} />
      </mesh>
      <mesh position={[0, 0, -0.005]}>
        <boxGeometry args={[1.62, 2.05, 0.01]} />
        <meshBasicMaterial color={PAPER} transparent opacity={0.55} />
      </mesh>
      <mesh position={[0, 0.85, 0.03]}>
        <boxGeometry args={[1.35, 0.02, 0.01]} />
        <meshBasicMaterial color={accentEdge ? ACCENT : INK} />
      </mesh>
      {[-0.35, -0.05, 0.25, 0.55].map((ly, i) => (
        <mesh key={ly} position={[0.1, ly, 0.03]}>
          <boxGeometry args={[1.1 - i * 0.08, 0.012, 0.008]} />
          <meshBasicMaterial color={INK} transparent opacity={0.25} />
        </mesh>
      ))}
      {[-0.35, -0.05, 0.25].map((cy, i) => (
        <group key={cy} position={[-0.62, cy, 0.04]}>
          <mesh>
            <boxGeometry args={[0.14, 0.14, 0.01]} />
            <meshBasicMaterial wireframe color={INK} transparent opacity={0.5} />
          </mesh>
          {i < 2 ? (
            <group position={[0, 0, 0.02]} rotation={[0, 0, -0.4]}>
              <mesh position={[-0.02, -0.02, 0]}>
                <boxGeometry args={[0.06, 0.015, 0.008]} />
                <meshBasicMaterial color={ACCENT} />
              </mesh>
              <mesh position={[0.03, 0.01, 0]} rotation={[0, 0, 1.1]}>
                <boxGeometry args={[0.1, 0.015, 0.008]} />
                <meshBasicMaterial color={ACCENT} />
              </mesh>
            </group>
          ) : null}
        </group>
      ))}
    </group>
  );
}

function Pencil() {
  const ref = useRef<THREE.Group>(null);
  useFrame((state) => {
    if (!ref.current) return;
    ref.current.position.y =
      -0.15 + Math.sin(state.clock.elapsedTime * 1.1) * 0.06;
  });
  return (
    <group
      ref={ref}
      position={[1.15, -0.2, 0.55]}
      rotation={[0.2, 0, -0.55]}
    >
      <mesh>
        <cylinderGeometry args={[0.045, 0.045, 1.35, 6]} />
        <meshBasicMaterial wireframe color={INK} transparent opacity={0.55} />
      </mesh>
      <mesh position={[0, -0.78, 0]}>
        <coneGeometry args={[0.045, 0.22, 6]} />
        <meshBasicMaterial wireframe color={ACCENT} transparent opacity={0.9} />
      </mesh>
      <mesh position={[0, 0.72, 0]}>
        <cylinderGeometry args={[0.05, 0.05, 0.12, 6]} />
        <meshBasicMaterial color={ACCENT} transparent opacity={0.7} />
      </mesh>
    </group>
  );
}

function VariantB() {
  return (
    <Rig spin={0.1}>
      <Float speed={0.9} rotationIntensity={0.12} floatIntensity={0.35}>
        <group position={[0, 0.05, 0]}>
          <Sheet y={0.18} rotZ={-0.12} depth={-0.25} />
          <Sheet y={0.05} rotZ={0.04} depth={0} />
          <Sheet y={-0.08} rotZ={0.14} depth={0.28} accentEdge />
          <Pencil />
        </group>
      </Float>
      <OrbitingCubes count={2} radius={2.35} speed={0.2} />
    </Rig>
  );
}

/* ─── Canvas shell ────────────────────────────────────────────────────── */

export default function BankHero3D({
  active = true,
  variant = "A",
  mastery = EMPTY_MASTERY,
}: {
  active?: boolean;
  variant?: BankHeroVariant;
  mastery?: MasterySnapshot;
}) {
  return (
    <Canvas
      frameloop={active ? "always" : "never"}
      camera={{ position: [0, 0.1, 4.4], fov: 38 }}
      dpr={[1, 1.8]}
      gl={{ antialias: true, alpha: true }}
      className="!pointer-events-none"
      style={{ background: "transparent" }}
    >
      <hemisphereLight args={[PAPER, INK, 0.35]} />
      <ambientLight intensity={0.55} />
      {variant === "A" ? <VariantA mastery={mastery} /> : <VariantB />}
    </Canvas>
  );
}
