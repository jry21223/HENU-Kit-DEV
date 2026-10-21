"use client";

import { useEffect, useRef, useSyncExternalStore, type ReactNode } from "react";
import { Swiper, SwiperSlide } from "swiper/react";
import type { Swiper as SwiperInstance } from "swiper";
import "swiper/css";

const reducedQuery = "(prefers-reduced-motion: reduce)";
function subscribeMotion(notify: () => void) {
  const query = window.matchMedia(reducedQuery);
  query.addEventListener("change", notify);
  return () => query.removeEventListener("change", notify);
}
const getMotion = () => window.matchMedia(reducedQuery).matches;
const getServerMotion = () => false;

export default function QuestionPager({ count, index, disabled, onChange, renderQuestion }: {
  count: number;
  index: number;
  disabled: boolean;
  onChange: (index: number) => void;
  renderQuestion: (index: number, active: boolean) => ReactNode;
}) {
  const swiperRef = useRef<SwiperInstance | null>(null);
  const gestureRef = useRef<{ x: number; y: number; cancelled: boolean } | null>(null);
  const multiTouchRef = useRef(false);
  const reducedMotion = useSyncExternalStore(subscribeMotion, getMotion, getServerMotion);

  useEffect(() => {
    function cancelMultitouch(event: TouchEvent) {
      const swiper = swiperRef.current;
      if (event.touches.length < 2 || !gestureRef.current || !swiper || swiper.destroyed) return;
      // Capture before Swiper's document move listener, including a second
      // contact outside the track. Native zoom keeps its default behavior.
      multiTouchRef.current = true;
      gestureRef.current.cancelled = true;
      swiper.params.longSwipes = false;
      swiper.params.shortSwipes = false;
      swiper.allowTouchMove = false;
      swiper.slideTo(index, 0);
    }
    function finishMultitouch(event: TouchEvent) {
      const swiper = swiperRef.current;
      if (event.touches.length || !multiTouchRef.current || !swiper || swiper.destroyed) return;
      multiTouchRef.current = false;
      swiper.allowTouchMove = !disabled && swiper.params.allowTouchMove !== false;
    }
    const options = { capture: true, passive: true };
    document.addEventListener("touchstart", cancelMultitouch, options);
    document.addEventListener("touchmove", cancelMultitouch, options);
    document.addEventListener("touchend", finishMultitouch, options);
    document.addEventListener("touchcancel", finishMultitouch, options);
    return () => {
      document.removeEventListener("touchstart", cancelMultitouch, options);
      document.removeEventListener("touchmove", cancelMultitouch, options);
      document.removeEventListener("touchend", finishMultitouch, options);
      document.removeEventListener("touchcancel", finishMultitouch, options);
    };
  }, [index, disabled]);

  useEffect(() => {
    const swiper = swiperRef.current;
    if (!swiper || swiper.destroyed) return;
    const speed = reducedMotion ? 0 : 240;
    swiper.params.speed = speed;
    // Leaving a breakpoint restores originalParams, including gesture speed.
    swiper.originalParams.speed = speed;
    if (swiper.activeIndex !== index) swiper.slideTo(index, speed);
    // Explanation, errors and correction forms change height independently of
    // navigation. Observe the active card so autoHeight does not clip them.
    swiper.updateAutoHeight(0);
    const card = swiper.slides[index]?.firstElementChild;
    if (!card) return;
    const observer = new ResizeObserver(() => {
      if (!swiper.destroyed) swiper.updateAutoHeight(0);
    });
    observer.observe(card);
    return () => observer.disconnect();
  }, [index, count, reducedMotion]);

  return (
    <Swiper
      className="practice-question-track min-w-0 max-w-full"
      initialSlide={index}
      slidesPerView={1}
      spaceBetween={16}
      autoHeight
      speed={reducedMotion ? 0 : 240}
      allowTouchMove={!disabled}
      simulateTouch={false}
      breakpoints={{ 769: { allowTouchMove: false } }}
      threshold={8}
      touchAngle={35}
      longSwipesRatio={0.18}
      resistanceRatio={0.4}
      edgeSwipeDetection
      edgeSwipeThreshold={24}
      touchStartPreventDefault={false}
      noSwipingSelector="button, input, textarea, select, a, [contenteditable='true'], [data-no-question-swipe]"
      onSwiper={(swiper) => { swiperRef.current = swiper; }}
      onTouchStart={(swiper, event) => {
        if (multiTouchRef.current) return;
        const point = "touches" in event ? event.touches[0] : event;
        gestureRef.current = point ? { x: point.pageX, y: point.pageY, cancelled: false } : null;
        swiper.params.longSwipes = true;
        swiper.params.shortSwipes = true;
      }}
      onTouchMove={(_swiper, event) => {
        const start = gestureRef.current;
        const point = "touches" in event ? event.touches[0] : event;
        if (start && point && Math.abs(point.pageY - start.y) > 12 && Math.abs(point.pageY - start.y) * 1.25 >= Math.abs(point.pageX - start.x)) start.cancelled = true;
      }}
      onTouchEnd={(swiper) => {
        // Swiper calls this before settling. A drag that turns into vertical
        // reading must return to the active question, even after axis lock.
        if (gestureRef.current?.cancelled) {
          swiper.params.longSwipes = false;
          swiper.params.shortSwipes = false;
        }
        gestureRef.current = null;
      }}
      onSlideChange={(swiper) => onChange(swiper.activeIndex)}
    >
      {Array.from({ length: count }, (_, position) => (
        <SwiperSlide key={position} inert={position !== index} aria-hidden={position !== index}>
          {Math.abs(position - index) <= 1 ? renderQuestion(position, position === index) : null}
        </SwiperSlide>
      ))}
    </Swiper>
  );
}
