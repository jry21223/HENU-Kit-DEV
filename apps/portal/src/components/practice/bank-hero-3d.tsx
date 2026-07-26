"use client";

/**
 * Practice bank hero 3D — mastery-driven knowledge mesh (no GLB).
 *
 * - subject rings: faint full track + progress arc (arc = mastery %)
 * - nucleus: accuracy → size / brightness
 * - outer cage: coverage average → density / opacity
 * - orbit cubes: streak (quiet secondary cue)
 */

import { useEffect, useMemo, useRef } from "react";
import { Canvas, useFrame } from "@react-three/fiber";
import { Float } from "@react-three/drei";
import * as THREE from "three";

const INK = "#161513";
const ACCENT = "#ff4d00";
const PAPER = "#f2f0ea";

/** Lerp speed ≈ settle in ~0.55s at 60fps */
const LERP = 6.5;
/** Rebuild torus arc only when display % moves ≥ this */
const ARC_REBUILD_STEP = 0.012;

/** 0–100 subject mastery row (same shape as USER_STATS.mastery). */
export type MasterySubject = {
  label: string;
  value: number;
};

/** Snapshot that drives the knowledge mesh. */
export type MasterySnapshot = {
  subjects: MasterySubject[];
  /** Overall accuracy 0–100 → nucleus size / brightness */
  accuracy: number;
  /** Streak days → orbiting cube count */
  streakDays: number;
  /** Total answered → orbit radius slightly */
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

function damp(current: number, target: number, delta: number, speed = LERP) {
  return THREE.MathUtils.damp(current, target, speed, delta);
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

/**
 * Streak cue only — kept small/quiet so rings stay primary.
 * Cap 4; smaller wire cubes outside the ring stack.
 */
function OrbitingCubes({
  count,
  radius = 2.15,
  speed = 0.22,
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
    const n = Math.max(0, Math.min(4, Math.round(count)));
    return Array.from({ length: n }, (_, i) => {
      const angle = n === 0 ? 0 : (Math.PI * 2 * i) / n;
      return {
        angle,
        y: (i % 2 === 0 ? 0.55 : -0.55) + (i - 1) * 0.08,
        size: 0.07 + (i % 2) * 0.025,
        r: radius + (i % 2) * 0.12,
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
          <meshBasicMaterial
            wireframe
            color={ACCENT}
            transparent
            opacity={0.55}
          />
        </mesh>
      ))}
    </group>
  );
}

function KnowledgeCore({
  accuracy,
  coverage,
}: {
  accuracy: number;
  /** average subject mastery 0–1 */
  coverage: number;
}) {
  const outerRef = useRef<THREE.Mesh>(null);
  const midRef = useRef<THREE.Mesh>(null);
  const nucleusRef = useRef<THREE.Mesh>(null);
  const outerMat = useRef<THREE.MeshBasicMaterial>(null);
  const midMat = useRef<THREE.MeshBasicMaterial>(null);
  const nucleusMat = useRef<THREE.MeshBasicMaterial>(null);

  const target = useRef({ acc: pct(accuracy), cov: coverage });
  const cur = useRef({ acc: pct(accuracy), cov: coverage });

  useEffect(() => {
    target.current = { acc: pct(accuracy), cov: coverage };
  }, [accuracy, coverage]);

  useFrame((_, delta) => {
    const t = target.current;
    const c = cur.current;
    c.acc = damp(c.acc, t.acc, delta);
    c.cov = damp(c.cov, t.cov, delta);

    const nucleusScale = 0.12 + c.acc * 0.28;
    const midScale = 0.42 + c.cov * 0.28;
    const outerScale = 0.92 + c.cov * 0.18;

    if (nucleusRef.current) nucleusRef.current.scale.setScalar(nucleusScale);
    if (midRef.current) midRef.current.scale.setScalar(midScale);
    if (outerRef.current) outerRef.current.scale.setScalar(outerScale);
    if (nucleusMat.current) nucleusMat.current.opacity = 0.5 + c.acc * 0.48;
    if (midMat.current) midMat.current.opacity = 0.1 + c.cov * 0.32;
    if (outerMat.current) outerMat.current.opacity = 0.14 + c.cov * 0.38;
  });

  const acc0 = pct(accuracy);
  const detail = coverage > 0.55 ? 1 : 0;

  return (
    <Float
      speed={1.0 + acc0 * 0.55}
      rotationIntensity={0.12 + acc0 * 0.12}
      floatIntensity={0.35 + acc0 * 0.4}
    >
      <mesh ref={outerRef}>
        <icosahedronGeometry args={[1.15, detail]} />
        <meshBasicMaterial
          ref={outerMat}
          wireframe
          color={INK}
          transparent
          opacity={0.14 + coverage * 0.38}
        />
      </mesh>
      <mesh ref={midRef} scale={0.42 + coverage * 0.28}>
        <icosahedronGeometry args={[1.15, 0]} />
        <meshBasicMaterial
          ref={midMat}
          wireframe
          color={INK}
          transparent
          opacity={0.1 + coverage * 0.32}
        />
      </mesh>
      <mesh ref={nucleusRef} scale={0.12 + acc0 * 0.28}>
        <octahedronGeometry args={[1, 0]} />
        <meshBasicMaterial
          ref={nucleusMat}
          wireframe
          color={ACCENT}
          transparent
          opacity={0.5 + acc0 * 0.48}
        />
      </mesh>
    </Float>
  );
}

/**
 * Single subject ring:
 * - full faint track (always complete)
 * - progress arc = 2π × mastery
 * - weak (<60) → accent fill; else ink
 * - tube / opacity / arc lerped when mastery changes
 */
function SubjectRing({
  label,
  value,
  baseR,
  tilt,
}: {
  label: string;
  value: number;
  baseR: number;
  tilt: [number, number, number];
}) {
  const group = useRef<THREE.Group>(null);
  const progressMesh = useRef<THREE.Mesh>(null);
  const progressMat = useRef<THREE.MeshBasicMaterial>(null);
  const tickMat = useRef<THREE.MeshBasicMaterial>(null);
  const targetT = useRef(pct(value));
  const curT = useRef(pct(value));
  const builtT = useRef(-1);

  useEffect(() => {
    targetT.current = pct(value);
  }, [value]);

  useFrame((_, delta) => {
    curT.current = damp(curT.current, targetT.current, delta);
    const t = curT.current;
    const weak = t < 0.6;

    if (
      builtT.current < 0 ||
      Math.abs(t - builtT.current) >= ARC_REBUILD_STEP ||
      builtT.current < 0.6 !== weak
    ) {
      builtT.current = t;
      const mesh = progressMesh.current;
      if (mesh) {
        const prev = mesh.geometry;
        const arc = Math.max(0.04, Math.PI * 2 * Math.max(0.02, t));
        const tube = 0.014 + t * 0.016;
        mesh.geometry = new THREE.TorusGeometry(baseR, tube, 10, 96, arc);
        prev.dispose();
      }
    }

    if (progressMat.current) {
      progressMat.current.color.set(weak ? ACCENT : INK);
      progressMat.current.opacity = 0.35 + t * 0.55;
    }
    if (tickMat.current) {
      tickMat.current.color.set(weak ? ACCENT : INK);
      tickMat.current.opacity = 0.4 + t * 0.45;
    }
    if (group.current) {
      group.current.scale.setScalar(0.94 + t * 0.1);
    }
  });

  const t0 = pct(value);
  const weak0 = value < 60;
  const arc0 = Math.max(0.04, Math.PI * 2 * Math.max(0.02, t0));
  const tube0 = 0.014 + t0 * 0.016;

  return (
    <group ref={group} rotation={tilt} userData={{ label }}>
      <mesh>
        <torusGeometry args={[baseR, 0.006, 6, 72]} />
        <meshBasicMaterial color={INK} transparent opacity={0.1} />
      </mesh>
      <mesh ref={progressMesh}>
        <torusGeometry args={[baseR, tube0, 10, 96, arc0]} />
        <meshBasicMaterial
          ref={progressMat}
          color={weak0 ? ACCENT : INK}
          transparent
          opacity={0.35 + t0 * 0.55}
        />
      </mesh>
      <mesh position={[baseR, 0, 0]} rotation={[0, 0, Math.PI / 2]}>
        <boxGeometry args={[0.09, 0.012, 0.012]} />
        <meshBasicMaterial
          ref={tickMat}
          color={weak0 ? ACCENT : INK}
          transparent
          opacity={0.55}
        />
      </mesh>
    </group>
  );
}

function MasteryRings({ subjects }: { subjects: MasterySubject[] }) {
  const group = useRef<THREE.Group>(null);
  const top = useMemo(() => {
    return subjects.slice(0, 3).map((s, i) => ({
      label: s.label,
      value: Math.min(100, Math.max(0, s.value)),
      baseR: 1.38 + i * 0.3,
      tilt: [i * 0.14, i * 0.28, 0] as [number, number, number],
    }));
  }, [subjects]);

  const avgTarget = useRef(
    top.length === 0
      ? 0
      : top.reduce((a, s) => a + s.value, 0) / top.length / 100
  );
  const avgCur = useRef(avgTarget.current);

  useEffect(() => {
    avgTarget.current =
      top.length === 0
        ? 0
        : top.reduce((a, s) => a + s.value, 0) / top.length / 100;
  }, [top]);

  useFrame((_, delta) => {
    if (!group.current) return;
    avgCur.current = damp(avgCur.current, avgTarget.current, delta);
    const avg = avgCur.current;
    group.current.rotation.z += delta * (0.045 + avg * 0.12);
    group.current.rotation.x += delta * (0.018 + avg * 0.045);
  });

  if (top.length === 0) {
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
      {top.map((s) => (
        <SubjectRing
          key={s.label}
          label={s.label}
          value={s.value}
          baseR={s.baseR}
          tilt={s.tilt}
        />
      ))}
    </group>
  );
}

function KnowledgeMesh({ mastery }: { mastery: MasterySnapshot }) {
  const coverage = useMemo(() => {
    const list = mastery.subjects;
    if (!list.length) return 0;
    return list.reduce((a, s) => a + s.value, 0) / list.length / 100;
  }, [mastery.subjects]);

  const cubeCount = Math.min(
    4,
    Math.max(
      0,
      Math.floor(mastery.streakDays / 10) + (mastery.streakDays > 0 ? 1 : 0)
    )
  );
  const orbitR = 2.2 + Math.min(0.35, mastery.totalQuestions / 2500);

  return (
    <Rig spin={0.09 + coverage * 0.07}>
      <KnowledgeCore accuracy={mastery.accuracy} coverage={coverage} />
      <MasteryRings subjects={mastery.subjects} />
      <OrbitingCubes
        count={cubeCount}
        radius={orbitR}
        speed={0.16 + coverage * 0.12}
      />
    </Rig>
  );
}

export default function BankHero3D({
  active = true,
  mastery = EMPTY_MASTERY,
}: {
  active?: boolean;
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
      <KnowledgeMesh mastery={mastery} />
    </Canvas>
  );
}
