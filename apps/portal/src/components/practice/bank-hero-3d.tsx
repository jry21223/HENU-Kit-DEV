"use client";

/**
 * Practice bank hero 3D — A/B variants (no GLB).
 *
 * A  Knowledge mesh: icosahedron + rings + orbiting cubes (homepage language)
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
  count = 3,
  radius = 1.85,
  speed = 0.28,
}: {
  count?: number;
  radius?: number;
  speed?: number;
}) {
  const group = useRef<THREE.Group>(null);
  useFrame((_, delta) => {
    if (group.current) group.current.rotation.y += delta * speed;
  });
  const cubes = useMemo(
    () =>
      Array.from({ length: count }, (_, i) => {
        const angle = (Math.PI * 2 * i) / count;
        return {
          angle,
          y: (i % 2 === 0 ? 0.35 : -0.4) + (i - 1) * 0.12,
          size: 0.11 + (i % 3) * 0.04,
          r: radius + (i % 2) * 0.18,
        };
      }),
    [count, radius]
  );
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

/* ─── Variant A: Knowledge mesh ───────────────────────────────────────── */

function KnowledgeCore() {
  return (
    <Float speed={1.2} rotationIntensity={0.2} floatIntensity={0.55}>
      {/* Outer wireframe crystal — knowledge network */}
      <mesh>
        <icosahedronGeometry args={[1.15, 1]} />
        <meshBasicMaterial wireframe color={INK} transparent opacity={0.42} />
      </mesh>
      {/* Inner denser shell */}
      <mesh scale={0.62}>
        <icosahedronGeometry args={[1.15, 0]} />
        <meshBasicMaterial wireframe color={INK} transparent opacity={0.28} />
      </mesh>
      {/* Accent nucleus */}
      <mesh scale={0.22}>
        <octahedronGeometry args={[1, 0]} />
        <meshBasicMaterial wireframe color={ACCENT} transparent opacity={0.9} />
      </mesh>
    </Float>
  );
}

function MasteryRings() {
  const group = useRef<THREE.Group>(null);
  useFrame((_, delta) => {
    if (!group.current) return;
    group.current.rotation.z += delta * 0.08;
    group.current.rotation.x += delta * 0.03;
  });
  return (
    <group ref={group} rotation={[Math.PI / 2.6, 0.2, 0]}>
      {[1.45, 1.7, 1.95].map((r, i) => (
        <mesh key={r} rotation={[i * 0.18, i * 0.35, 0]}>
          <torusGeometry args={[r, 0.012, 8, 64]} />
          <meshBasicMaterial
            color={i === 1 ? ACCENT : INK}
            transparent
            opacity={i === 1 ? 0.75 : 0.22}
          />
        </mesh>
      ))}
    </group>
  );
}

function VariantA() {
  return (
    <Rig spin={0.12}>
      <KnowledgeCore />
      <MasteryRings />
      <OrbitingCubes count={3} radius={2.15} />
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
      {/* Card body — thin box, wireframe paper face */}
      <mesh>
        <boxGeometry args={[1.7, 2.15, 0.04]} />
        <meshBasicMaterial wireframe color={INK} transparent opacity={0.38} />
      </mesh>
      {/* Soft fill so stack reads as solid paper, not pure cage */}
      <mesh position={[0, 0, -0.005]}>
        <boxGeometry args={[1.62, 2.05, 0.01]} />
        <meshBasicMaterial color={PAPER} transparent opacity={0.55} />
      </mesh>
      {/* Header rule */}
      <mesh position={[0, 0.85, 0.03]}>
        <boxGeometry args={[1.35, 0.02, 0.01]} />
        <meshBasicMaterial color={accentEdge ? ACCENT : INK} />
      </mesh>
      {/* Answer lines */}
      {[-0.35, -0.05, 0.25, 0.55].map((ly, i) => (
        <mesh key={ly} position={[0.1, ly, 0.03]}>
          <boxGeometry args={[1.1 - i * 0.08, 0.012, 0.008]} />
          <meshBasicMaterial color={INK} transparent opacity={0.25} />
        </mesh>
      ))}
      {/* Checkbox column */}
      {[-0.35, -0.05, 0.25].map((cy, i) => (
        <group key={cy} position={[-0.62, cy, 0.04]}>
          <mesh>
            <boxGeometry args={[0.14, 0.14, 0.01]} />
            <meshBasicMaterial wireframe color={INK} transparent opacity={0.5} />
          </mesh>
          {i < 2 ? (
            // Check mark as two thin boxes
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
      {/* Shaft */}
      <mesh>
        <cylinderGeometry args={[0.045, 0.045, 1.35, 6]} />
        <meshBasicMaterial wireframe color={INK} transparent opacity={0.55} />
      </mesh>
      {/* Tip */}
      <mesh position={[0, -0.78, 0]}>
        <coneGeometry args={[0.045, 0.22, 6]} />
        <meshBasicMaterial wireframe color={ACCENT} transparent opacity={0.9} />
      </mesh>
      {/* Eraser cap */}
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
}: {
  active?: boolean;
  variant?: BankHeroVariant;
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
      {/* Soft paper/ink hemisphere keeps industrial light without GLB materials */}
      <hemisphereLight args={[PAPER, INK, 0.35]} />
      <ambientLight intensity={0.55} />
      {variant === "A" ? <VariantA /> : <VariantB />}
    </Canvas>
  );
}
