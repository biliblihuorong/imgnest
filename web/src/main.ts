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

// 业务码 20001（凭证失效）：清空本地登录态并回到登录页；
// 20002 是凭证内容错误，由调用方就地展示，不清会话
setUnauthorizedHandler(() => {
  useAuthStore().clear();
  void router.push("/login").catch(() => {
    // 重复导航等失败无需处理
  });
});

app.mount("#app");

// 挂载后异步恢复登录态；恢复失败会清空 token，后续首个 API 请求 401 兜底跳转
void useAuthStore().restore();
