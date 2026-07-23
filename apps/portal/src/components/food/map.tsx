"use client";

import { useEffect, useRef, useState } from "react";
import { MapContainer, TileLayer, Marker } from "react-leaflet";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import type { Shop } from "@/lib/food/mock";

/** 瓦片加载失败/超时兜底：结构线占位 + 店名坐标 */
function MapFallback({ shop }: { shop: Shop }) {
  return (
    <div className="bg-blueprint flex h-[280px] flex-col items-center justify-center border-t border-line">
      <span aria-hidden className="font-mono text-lg text-accent">+</span>
      <p className="mt-2 font-mono text-xs tracking-widest text-ink/60">{shop.name}</p>
      <p className="mt-1 font-mono text-[10px] tracking-widest text-ink/40">
        {shop.lat.toFixed(4)}, {shop.lng.toFixed(4)} · 地图加载失败，坐标仅供参考
      </p>
    </div>
  );
}

/**
 * 店家位置小地图：Leaflet + OSM，标记为自绘方块 divIcon（不引默认图标资源）。
 * 客户端加载（使用处 next/dynamic ssr:false）。
 */
export default function ShopMap({ shop }: { shop: Shop }) {
  const [failed, setFailed] = useState(false);
  const loadedRef = useRef(false);

  // 超时兜底：9s 内一张瓦片都没加载成功视为离线
  useEffect(() => {
    const t = setTimeout(() => {
      if (!loadedRef.current) setFailed(true);
    }, 9000);
    return () => clearTimeout(t);
  }, []);

  if (failed) return <MapFallback shop={shop} />;

  const icon = L.divIcon({
    className: "",
    html: '<div style="width:14px;height:14px;background:#ff4d00;border:2px solid #161513"></div>',
    iconSize: [14, 14],
    iconAnchor: [7, 7],
  });

  return (
    <MapContainer
      center={[shop.lat, shop.lng]}
      zoom={16}
      scrollWheelZoom={false}
      style={{ height: 280, width: "100%" }}
      className="border-t border-line"
    >
      <TileLayer
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
        eventHandlers={{
          load: () => {
            loadedRef.current = true;
          },
          tileerror: () => setFailed(true),
        }}
      />
      <Marker position={[shop.lat, shop.lng]} icon={icon} />
    </MapContainer>
  );
}
