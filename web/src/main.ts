import { createPinia } from "pinia";
import { createApp } from "vue";
import App from "./App.vue";
import { setUnauthorizedHandler } from "./api/client";
import { setupRouter } from "./router";
import { useAuthStore } from "./stores/auth";

const app = createApp(App);
app.use(createPinia());

const router = setupRouter();
app.use(router);

// 401/20001/20002：清空本地登录态并回到登录页
setUnauthorizedHandler(() => {
  useAuthStore().clear();
  void router.push("/login").catch(() => {
    // 重复导航等失败无需处理
  });
});

app.mount("#app");

// 挂载后异步恢复登录态；恢复失败会清空 token，后续首个 API 请求 401 兜底跳转
void useAuthStore().restore();
