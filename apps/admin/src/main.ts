import "element-plus/dist/index.css";
import "./styles/main.css";

import ElementPlus from "element-plus";
import { QueryClient, VueQueryPlugin } from "@tanstack/vue-query";
import { createPinia } from "pinia";
import { createApp } from "vue";

import App from "./App.vue";
import { router } from "./router";

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false } },
});

createApp(App).use(createPinia()).use(router).use(ElementPlus).use(VueQueryPlugin, { queryClient }).mount("#app");
