<script setup lang="ts">
const boundaryStatus = "HC-03 application boundary";
</script>

<template>
  <main class="console-boundary">
    <p class="eyebrow">{{ boundaryStatus }}</p>
    <h1>HENUKit Console</h1>
    <p>独立 Console Bundle 已建立；产品导航、权限状态与六模块 Overview 将由 HC-04 交付。</p>
    <small>学生自主运营 · 非河南大学官方项目</small>
  </main>
</template>

<style>
:root {
  color: #172033;
  background: #f5f7fb;
  font-family: Inter, "PingFang SC", "Microsoft YaHei", sans-serif;
}

body {
  margin: 0;
}

.console-boundary {
  box-sizing: border-box;
  min-height: 100vh;
  display: grid;
  place-content: center;
  gap: 0.75rem;
  padding: 2rem;
  text-align: center;
}

.console-boundary > * {
  margin: 0;
}

.eyebrow,
small {
  color: #657089;
}
</style>
