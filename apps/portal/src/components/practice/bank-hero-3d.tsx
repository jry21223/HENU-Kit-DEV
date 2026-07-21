"use client";

import { Suspense, useMemo, useRef } from "react";
import { Canvas, useFrame } from "@react-three/fiber";
import { useGLTF } from "@react-three/drei";
import * as THREE from "three";

const PLASTER = "#e9e4da"; // 哑光石膏白（微暖）

/** Poly Haven marble_bust_01（CC0）古典大理石胸像：材质覆盖为哑光石膏，归一化尺寸并居中 */
function PlasterHead() {
  const { scene } = useGLTF("/models/plaster-bust.glb");
  const normalized = useMemo(() => {
    const mat = new THREE.MeshStandardMaterial({
      color: PLASTER,
      roughness: 0.9,
      metalness: 0,
    });
    scene.traverse((o) => {
      if ((o as THREE.Mesh).isMesh) (o as THREE.Mesh).material = mat;
    });
    // useGLTF 按 URL 全局缓存 scene：二次挂载时对象带着上次设置的缩放。
    // 必须先复位再测量，否则归一化系数叠加错误，模型会变得巨大。
    scene.scale.setScalar(1);
    scene.position.set(0, 0, 0);
    scene.updateMatrixWorld(true);
    const box = new THREE.Box3().setFromObject(scene);
    const size = box.getSize(new THREE.Vector3());
    const center = box.getCenter(new THREE.Vector3());
    // 归一化到 2.4 个单位高：视口可见高约 2.89（cam z=4.2, fov=38），占约 83%
    const s = 2.4 / size.y;
    scene.scale.setScalar(s);
    scene.position.set(-center.x * s, -center.y * s - 0.15, -center.z * s);
    return scene;
  }, [scene]);
  return <primitive object={normalized} />;
}

/** 缓慢自转 + 鼠标 lerp 微偏转（沿用首页 hero-3d 的 Rig 思路） */
function Rig({ children }: { children: React.ReactNode }) {
  const group = useRef<THREE.Group>(null);
  useFrame((state, delta) => {
    const g = group.current;
    if (!g) return;
    g.rotation.y += delta * 0.22;
    g.rotation.x = THREE.MathUtils.lerp(g.rotation.x, state.pointer.y * -0.1, 0.05);
    g.rotation.z = THREE.MathUtils.lerp(g.rotation.z, state.pointer.x * 0.05, 0.05);
  });
  return <group ref={group}>{children}</group>;
}

export default function BankHero3D({ active = true }: { active?: boolean }) {
  return (
    <Canvas
      frameloop={active ? "always" : "never"}
      camera={{ position: [0, 0.15, 4.2], fov: 38 }}
      dpr={[1, 1.8]}
      gl={{ antialias: true, alpha: true }}
      className="!pointer-events-none"
      style={{ background: "transparent" }}
    >
      {/* 素描石膏的明暗：环境底光 + 主光 + 暖色轮廓光 */}
      <hemisphereLight args={["#f2f0ea", "#161513", 0.5]} />
      <directionalLight position={[3.5, 4, 5]} intensity={1.2} />
      <directionalLight position={[-4, 1.5, -3]} intensity={0.35} color="#ffd9c7" />
      <Suspense fallback={null}>
        <Rig>
          <PlasterHead />
        </Rig>
      </Suspense>
    </Canvas>
  );
}
