<template>
  <main class="admin-login">
    <el-card class="login-card" shadow="never">
      <template #header>
        <strong>资料运营工作台登录</strong>
      </template>
      <p class="muted">管理员登录后可以维护课程、上传 PDF 资料，并管理资料保障相关状态。</p>
      <el-form class="form-stack" label-position="top" @submit.prevent>
        <el-form-item label="管理员邮箱">
          <el-input v-model="email" placeholder="admin@example.com" />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="name" placeholder="可选" />
        </el-form-item>
        <el-form-item label="验证码">
          <div class="inline-row">
            <el-input v-model="code" placeholder="123456" />
            <el-button :disabled="loading || !email" @click="handleSendCode">发送验证码</el-button>
          </div>
        </el-form-item>
        <el-button class="full-width" type="primary" :loading="loading" :disabled="!email || !code" @click="handleLogin">登录</el-button>
      </el-form>
      <el-alert v-if="devCode" class="notice" type="info" :closable="false" :title="`开发环境验证码：${devCode}`" />
      <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
      <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
    </el-card>
  </main>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { login, sendCode } from "../lib/api";
import { useAuthStore } from "../stores/auth";

const router = useRouter();
const auth = useAuthStore();
const email = ref("admin@example.com");
const name = ref("管理员");
const code = ref("");
const devCode = ref("");
const message = ref("");
const error = ref("");
const loading = ref(false);

async function handleSendCode() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const response = await sendCode(email.value);
    devCode.value = response.data?.devCode ?? "";
    if (devCode.value) code.value = devCode.value;
    message.value = "验证码已发送。";
  } catch (err) {
    error.value = err instanceof Error ? err.message : "发送验证码失败";
  } finally {
    loading.value = false;
  }
}

async function handleLogin() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const response = await login(email.value, code.value, name.value);
    auth.setUser(response.data?.user ?? null);
    if (!auth.canAccessAdminConsole) {
      error.value = "当前账号没有后台访问权限。";
      return;
    }
    await router.push(auth.isAdmin ? "/dashboard" : "/ai/drafts");
  } catch (err) {
    error.value = err instanceof Error ? err.message : "登录失败";
  } finally {
    loading.value = false;
  }
}
</script>
