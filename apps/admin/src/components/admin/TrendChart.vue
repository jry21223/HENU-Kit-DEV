<template>
  <div ref="chartElement" class="h-[300px] w-full" role="img" :aria-label="ariaLabel" />
</template>

<script setup lang="ts">
import { LineChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { graphic, init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

type Point = { time: string; value: number };
use([LineChart, GridComponent, TooltipComponent, CanvasRenderer]);
const props = defineProps<{ points: Point[]; label: string; color?: string }>();
const chartElement = ref<HTMLElement | null>(null);
const ariaLabel = computed(() => `${props.label}最近 ${props.points.length} 天趋势图`);
let chart: EChartsType | null = null;
let resizeObserver: ResizeObserver | null = null;

function render() {
  if (!chartElement.value) return;
  chart ??= init(chartElement.value);
  const color = props.color ?? "#0f6b4f";
  chart.setOption({
    animationDuration: 450,
    grid: { left: 12, right: 14, top: 22, bottom: 8, containLabel: true },
    tooltip: {
      trigger: "axis",
      backgroundColor: "rgba(15, 23, 42, .94)",
      borderWidth: 0,
      textStyle: { color: "#fff", fontSize: 12 },
      valueFormatter: (value: unknown) => new Intl.NumberFormat("zh-CN").format(Number(value)),
    },
    xAxis: {
      type: "category",
      boundaryGap: false,
      data: props.points.map((point) => point.time.slice(5)),
      axisLine: { lineStyle: { color: "#e2e8f0" } },
      axisTick: { show: false },
      axisLabel: { color: "#64748b", fontSize: 11, interval: 2 },
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      axisLabel: { color: "#64748b", fontSize: 11 },
      splitLine: { lineStyle: { color: "#eef2f6", type: "dashed" } },
    },
    series: [{
      name: props.label,
      type: "line",
      data: props.points.map((point) => point.value),
      smooth: 0.32,
      showSymbol: false,
      symbolSize: 7,
      lineStyle: { color, width: 3 },
      itemStyle: { color },
      areaStyle: {
        color: new graphic.LinearGradient(0, 0, 0, 1, [
          { offset: 0, color: `${color}38` },
          { offset: 1, color: `${color}03` },
        ]),
      },
    }],
  }, true);
}

onMounted(async () => {
  await nextTick();
  render();
  if (chartElement.value) {
    resizeObserver = new ResizeObserver(() => chart?.resize());
    resizeObserver.observe(chartElement.value);
  }
});
watch(() => [props.points, props.label, props.color], render, { deep: true });
onBeforeUnmount(() => { resizeObserver?.disconnect(); chart?.dispose(); });
</script>
