<script setup lang="ts">
import { NModal } from "naive-ui";
import { computed } from "vue";

/**
 * 居中的表单弹窗：新增/编辑一类操作统一用它，不再用右侧抽屉。
 *
 * - 内容区按宽度自动排成 cols 列（默认 2），弹窗变窄（手机）时自动变成 1 列。
 *   把 NForm 加上 class="form-grid"（样式在 src/assets/kit.css），NFormItem 就会按列排；需要占满一行的项加 class="form-grid__full"。
 * - busy 为 true（保存中）时不能通过遮罩、Esc 或右上角关闭，避免保存结果落到已关闭/已切换的弹窗上。
 */
const show = defineModel<boolean>("show", { default: false });
const props = withDefaults(
  defineProps<{
    title?: string;
    busy?: boolean;
    /** 桌面宽度（px，默认 560）；窄屏自动收缩到视口宽度减去两侧留白。 */
    width?: number;
    /** 宽屏时的列数。 */
    cols?: number;
  }>(),
  { title: "", busy: false, width: 560, cols: 2 },
);

const style = computed(() => ({
  width: `min(${props.width}px, calc(100vw - 32px))`,
  "--form-cols": String(props.cols),
}));

function onUpdateShow(value: boolean): void {
  if (!value && props.busy) return;
  show.value = value;
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    class="form-modal"
    :style="style"
    :title="title"
    :bordered="false"
    :mask-closable="!busy"
    :close-on-esc="!busy"
    :closable="!busy"
    :segmented="{ content: true, footer: 'soft' }"
    @update:show="onUpdateShow"
  >
    <div class="form-modal__body">
      <slot />
    </div>
    <template v-if="$slots.footer" #footer>
      <div class="form-modal__footer">
        <slot name="footer" />
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.form-modal__body {
  /* 列数按弹窗自身宽度决定，而不是视口宽度：同一个弹窗在侧栏展开/收起时表现一致。 */
  container-type: inline-size;
  max-height: calc(100dvh - 220px);
  overflow-y: auto;
}
.form-modal__footer {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  justify-content: flex-end;
}
</style>
