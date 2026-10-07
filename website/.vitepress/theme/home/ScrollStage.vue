<script setup lang="ts">
import { onBeforeUnmount, onMounted, shallowRef } from "vue";
import type { Step } from "./copy";

/**
 * 滚动叙事：左边是一段段文字，右边的舞台粘在视口里。
 * 哪一段经过视口中线，就把它的序号交给父组件，由父组件决定舞台显示什么。
 */
defineProps<{ steps: Step[] }>();
const active = defineModel<number>({ required: true });

const list = shallowRef<HTMLElement>();

/** 判定线在视口里的位置：宽屏取中线；窄屏时舞台占着上半屏，线下移到文字实际可见的区域。 */
function line(): number {
  const narrow = window.matchMedia("(max-width: 959px)").matches;
  return window.innerHeight * (narrow ? 0.72 : 0.5);
}

/** 最后一个顶边已经越过判定线的段落就是当前段落。 */
function sync(): void {
  const steps = list.value?.children;
  if (!steps) return;
  const at = line();
  let current = 0;
  for (let index = 0; index < steps.length; index += 1)
    if (steps[index].getBoundingClientRect().top <= at) current = index;
  if (current !== active.value) active.value = current;
}

onMounted(() => {
  sync();
  window.addEventListener("scroll", sync, { passive: true });
  window.addEventListener("resize", sync);
});
onBeforeUnmount(() => {
  window.removeEventListener("scroll", sync);
  window.removeEventListener("resize", sync);
});
</script>

<template>
  <div class="stage">
    <div ref="list" class="stage__steps">
      <article
        v-for="(step, index) in steps"
        :key="step.title"
        class="stage__step"
        :class="{ 'is-active': index === active }"
        :data-index="index"
      >
        <h3>{{ step.title }}</h3>
        <p>{{ step.body }}</p>
        <p v-if="step.note" class="stage__note">{{ step.note }}</p>
      </article>
    </div>
    <div class="stage__sticky">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.stage {
  display: grid;
  grid-template-columns: minmax(0, 0.72fr) minmax(0, 1.5fr);
  gap: clamp(28px, 5vw, 72px);
  align-items: start;
}

.stage__step {
  display: flex;
  flex-direction: column;
  gap: 14px;
  justify-content: center;
  min-height: 82vh;
  opacity: 0.32;
  transition: opacity 0.5s var(--mv-ease);
}

.stage__step.is-active {
  opacity: 1;
}

.stage__step h3 {
  font-size: clamp(24px, 2.5vw, 34px);
  font-weight: 700;
  line-height: 1.25;
  letter-spacing: -0.02em;
  text-wrap: balance;
}

.stage__step p {
  max-width: 30em;
  font-size: 16.5px;
  line-height: 1.8;
  color: var(--mv-sub);
}

.stage__step .stage__note {
  padding-left: 14px;
  font-size: 14.5px;
  border-left: 2px solid var(--mv-line-strong);
}

.stage__sticky {
  position: sticky;
  top: calc(var(--vp-nav-height) + 20px);
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-height: calc(100vh - var(--vp-nav-height) - 40px);
}

@media (max-width: 959px) {
  .stage {
    display: flex;
    flex-direction: column-reverse;
    gap: 0;
  }

  /* 窄屏下 VitePress 的顶栏不固定，舞台直接贴到视口顶部。 */
  .stage__sticky {
    top: 0;
    z-index: 5;
    width: calc(100% + 32px);
    min-height: 0;
    margin-inline: -16px;
    padding: 12px 16px 14px;
    background: var(--mv-bg-deep);
    border-bottom: 1px solid var(--mv-line);
  }

  .stage__steps {
    width: 100%;
  }

  .stage__step {
    justify-content: flex-start;
    min-height: 62vh;
    padding-top: 36px;
  }

  .stage__step:last-child {
    min-height: 46vh;
  }
}
</style>
