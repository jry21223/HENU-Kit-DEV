"use client";

import { useRef } from "react";
import { Canvas, useFrame } from "@react-three/fiber";
import { Float } from "@react-three/drei";
import * as THREE from "three";

const INK = "#161513";
const ACCENT = "#ff4d00";

/** 主体：工程图纸质感线框 TorusKnot */
function WireKnot() {
  return (
    <Float speed={1.4} rotationIntensity={0.25} floatIntensity={0.8}>
      <mesh>
        <torusKnotGeometry args={[1.05, 0.3, 160, 20]} />
        <meshBasicMaterial wireframe color={INK} transparent opacity={0.45} />
      </mesh>
    </Float>
  );
}

/** 点缀：绕主体缓慢公转的安全橙小线框立方体 */
function OrbitingCubes() {
  const group = useRef<THREE.Group>(null);
  useFrame((_, delta) => {
    if (group.current) group.current.rotation.y += delta * 0.25;
  });
  const cubes: Array<{ angle: number; radius: number; y: number; size: number }> = [
    { angle: 0, radius: 2.3, y: 0.4, size: 0.22 },
    { angle: (Math.PI * 2) / 3, radius: 2.6, y: -0.5, size: 0.16 },
    { angle: (Math.PI * 4) / 3, radius: 2.1, y: 0.9, size: 0.12 },
  ];
  return (
    <group ref={group}>
      {cubes.map((c, i) => (
        <mesh
          key={i}
          position={[
            Math.cos(c.angle) * c.radius,
            c.y,
            Math.sin(c.angle) * c.radius,
          ]}
        >
          <boxGeometry args={[c.size, c.size, c.size]} />
          <meshBasicMaterial wireframe color={ACCENT} />
        </mesh>
      ))}
    </group>
  );
}

/** 整组：缓慢自转 + 鼠标偏转（lerp，幅度克制） */
function Rig() {
  const group = useRef<THREE.Group>(null);
  useFrame((state, delta) => {
    const g = group.current;
    if (!g) return;
    g.rotation.y += delta * 0.12;
    const targetX = state.pointer.y * -0.18;
    const targetZ = state.pointer.x * 0.08;
    g.rotation.x = THREE.MathUtils.lerp(g.rotation.x, targetX, 0.05);
    g.rotation.z = THREE.MathUtils.lerp(g.rotation.z, targetZ, 0.05);
  });
  return (
    <group ref={group}>
      <WireKnot />
      <OrbitingCubes />
    </group>
  );
}

export default function Hero3D({ active = true }: { active?: boolean }) {
  return (
    <Canvas
      frameloop={active ? "always" : "never"}
      camera={{ position: [0, 0, 5.2], fov: 42 }}
      dpr={[1, 1.8]}
      gl={{ antialias: true, alpha: true }}
      className="!pointer-events-none"
      style={{ background: "transparent" }}
    >
      <Rig />
    </Canvas>
  );
}
