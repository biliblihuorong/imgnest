<script setup lang="ts">
import { NButton, NResult } from "naive-ui";
import { useRouter } from "vue-router";

const router = useRouter();

async function backToUpload(): Promise<void> {
  await router.push("/upload");
}
</script>

<template>
  <NResult
    status="403"
    title="403"
    description="你没有权限访问管理后台"
    style="margin: 48px auto"
  >
    <template #footer>
      <NButton type="primary" @click="backToUpload">返回上传页</NButton>
    </template>
  </NResult>
</template>
