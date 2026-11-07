<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from "vue";

import { Button, Card, PageHeader } from "@/components/ui";
import { consoleLoginHref, fetchLibraryReadiness, type LibraryReadiness } from "@/lib/console-gateway";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; permissions: string[] }>();
const readiness = ref<LibraryReadiness>();
const state = ref<"loading" | "confirmed" | "signed_out" | "denied" | "unavailable">("loading");
const retrying = ref(false);
const publicDirectoryLink = ref<HTMLAnchorElement>();
let generation = 0;

async function refresh(retryTrigger?: HTMLElement) {
  const requestGeneration = ++generation;
  retrying.value = !!retryTrigger;
  readiness.value = undefined;
  if (props.authState !== "authenticated") { state.value = props.authState; return; }
  if (!props.permissions.includes("library.read")) { state.value = "denied"; return; }
  state.value = "loading";
  const result = await fetchLibraryReadiness();
  if (requestGeneration !== generation) return;
  const returnFocus = retryTrigger && document.activeElement === retryTrigger;
  if (result.state === "authenticated") { readiness.value = result.readiness; state.value = "confirmed"; }
  else state.value = result.state;
  retrying.value = false;
  if (returnFocus && state.value === "confirmed") {
    await nextTick();
    if (requestGeneration === generation) publicDirectoryLink.value?.focus();
  }
}

function retry(event: MouseEvent) {
  if (state.value === "loading") return;
  void refresh(event.currentTarget as HTMLElement);
}

function confirmationTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", dateStyle: "medium", timeStyle: "medium" }).format(new Date(value));
}

watch(() => [props.authState, props.permissions.join("\n")], () => { void refresh(); }, { immediate: true });
onBeforeUnmount(() => { generation += 1; });
</script>

<template>
  <section aria-labelledby="library-heading">
    <PageHeader title="资料库运营" title-id="library-heading" description="查看资料管理状态，或进入公开资料目录。" />

    <div v-if="state === 'loading' || state === 'unavailable'" class="operation-state">
      <p role="status" aria-live="polite" aria-atomic="true">{{ state === 'loading' ? '正在确认资料管理能力…' : '暂时无法确认资料管理能力，请稍后重新加载。' }}</p>
      <Button v-if="state === 'unavailable' || retrying" class="mt-3" :aria-disabled="state === 'loading'" :aria-busy="state === 'loading'" @click="retry">重新加载</Button>
    </div>
    <div v-else-if="state === 'signed_out'" class="operation-state" role="status"><p>登录状态已过期，请重新登录后查看。</p><Button as="a" :href="consoleLoginHref()" class="mt-3">重新登录</Button></div>
    <div v-else-if="state === 'denied'" class="operation-state" role="status">当前账户没有资料库查看权限，请联系管理员。</div>
    <Card v-else-if="readiness" class="mt-6 p-5" role="status" aria-labelledby="library-readiness-heading">
      <template v-if="readiness.management === 'not_ready'">
        <h2 id="library-readiness-heading" class="text-lg font-bold">资料管理尚未就绪</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">当前不能在此维护课程和资料，也不能处理投稿审核与纠错。</p>
      </template>
      <template v-else>
        <h2 id="library-readiness-heading" class="text-lg font-bold">当前版本暂不支持资料管理</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">服务已报告管理能力可用，当前后台版本尚未接入管理操作，暂时无法在此维护资料。</p>
      </template>
      <p class="mt-3 text-sm leading-6">你可以打开公开资料目录，搜索和查看已公开的资料。</p>
      <div class="mt-4"><Button as-child><a ref="publicDirectoryLink" href="https://henukit.cn/library" target="_blank" rel="noopener noreferrer">打开公开资料目录</a></Button></div>
      <p class="mt-5 text-sm text-muted-foreground">状态确认于 <time :datetime="readiness.checked_at" :title="`来源时间：${readiness.checked_at}`">{{ confirmationTime(readiness.checked_at) }}</time>（北京时间）。</p>
    </Card>
  </section>
</template>
