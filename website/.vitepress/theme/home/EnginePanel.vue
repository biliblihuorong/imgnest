<script setup lang="ts">
import MvIcon from "./MvIcon.vue";
import type { HomeCopy } from "./copy";

/** 后端三段叙事对应的三个面板：上传流水线、默认上限、失败补偿。 */
defineProps<{ view: number; copy: HomeCopy["engine"] }>();

const PATH = "2026/10/07/66ff1b2a41e07";
const OBJECTS = [
  { key: `${PATH}.jpg`, ok: true },
  { key: `${PATH}.webp`, ok: true },
  { key: `${PATH}_thumbs.webp`, ok: false },
];
</script>

<template>
  <div class="engine">
    <section v-if="view === 0" key="pipeline" class="engine__view">
      <h4>{{ copy.pipeline.caption }}</h4>
      <ol class="pipeline">
        <li v-for="(stage, index) in copy.pipeline.stages" :key="stage" :style="{ '--i': index }">
          <span class="pipeline__dot"><MvIcon name="check" /></span>
          {{ stage }}
        </li>
      </ol>
      <p class="pipeline__done" :style="{ '--i': copy.pipeline.stages.length }">
        {{ copy.pipeline.done }}
      </p>
    </section>

    <section v-else-if="view === 1" key="limits" class="engine__view">
      <h4>{{ copy.limits.caption }}</h4>
      <div class="slots">
        <div class="slots__group">
          <span>{{ copy.limits.working }}</span>
          <div><i class="is-busy" /><i class="is-busy" /></div>
        </div>
        <div class="slots__group">
          <span>{{ copy.limits.waiting }}</span>
          <div><i /><i /><i /><i /></div>
        </div>
      </div>
      <dl class="limits">
        <div v-for="[label, value] in copy.limits.rows" :key="label">
          <dt>{{ label }}</dt>
          <dd>{{ value }}</dd>
        </div>
      </dl>
      <p class="engine__foot">{{ copy.limits.foot }}</p>
    </section>

    <section v-else key="recovery" class="engine__view">
      <h4>{{ copy.recovery.caption }}</h4>
      <ul class="objects">
        <li v-for="object in OBJECTS" :key="object.key" :class="object.ok ? 'is-written' : 'is-failed'">
          <code>{{ object.key }}</code>
          <span v-if="object.ok" class="objects__state">
            <span class="objects__before"><MvIcon name="check" />{{ copy.recovery.written }}</span>
            <span class="objects__after"><MvIcon name="trash" />{{ copy.recovery.removed }}</span>
          </span>
          <span v-else class="objects__state"><MvIcon name="x" />{{ copy.recovery.failed }}</span>
        </li>
      </ul>
      <ul class="facts">
        <li v-for="fact in copy.recovery.facts" :key="fact"><MvIcon name="check" />{{ fact }}</li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.engine {
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-height: 468px;
  padding: clamp(20px, 3vw, 36px);
  background: var(--mv-card);
  border: 1px solid var(--mv-line);
  border-radius: 24px;
  box-shadow: var(--mv-shadow-side);
}

.engine__view {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.engine h4 {
  font-size: 14px;
  font-weight: 500;
  color: var(--mv-sub);
}

/* 只做进入动画、不做离开动画：面板任何时刻都有内容。 */
.engine__view {
  animation: view-in 0.36s var(--mv-ease) both;
}

@keyframes view-in {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
}

.engine__foot {
  font-size: 13.5px;
  color: var(--mv-sub);
}

/* 流水线：真实的先后顺序，所以用有序列表 ------------------------------ */
.pipeline {
  display: grid;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.pipeline li {
  position: relative;
  display: flex;
  gap: 14px;
  align-items: center;
  padding: 7px 0;
  font-size: 16px;
  animation: stage-in 0.5s var(--mv-ease) both;
  animation-delay: calc(var(--i) * 0.2s + 0.15s);
}

.pipeline li + li::before {
  position: absolute;
  top: -11px;
  left: 10px;
  width: 2px;
  height: 18px;
  content: "";
  background: var(--mv-line-strong);
}

.pipeline__dot {
  z-index: 1;
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 22px;
  height: 22px;
  font-size: 12px;
  color: #fff;
  background: var(--mv-primary);
  border-radius: 50%;
}

.pipeline__done {
  padding: 12px 16px;
  font-size: 14.5px;
  font-weight: 600;
  color: var(--mv-ok);
  background: color-mix(in srgb, var(--mv-ok) 10%, transparent);
  border-radius: 12px;
  animation: stage-in 0.5s var(--mv-ease) both;
  animation-delay: calc(var(--i) * 0.2s + 0.3s);
}

@keyframes stage-in {
  from {
    opacity: 0;
    transform: translateX(-8px);
  }
}

/* 上限：两个处理位，其余排队 ------------------------------------------ */
.slots {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 28px;
  padding: 16px;
  background: var(--mv-soft);
  border-radius: 16px;
}

.slots__group {
  display: grid;
  gap: 8px;
  font-size: 13px;
  color: var(--mv-sub);
}

.slots__group div {
  display: flex;
  gap: 8px;
}

.slots i {
  position: relative;
  width: 46px;
  height: 36px;
  overflow: hidden;
  background: var(--mv-card);
  border: 1px dashed var(--mv-line-strong);
  border-radius: 9px;
}

.slots i.is-busy {
  border: 1px solid var(--mv-primary);
}

.slots i.is-busy::after {
  position: absolute;
  inset: auto 0 0;
  height: 4px;
  content: "";
  background: var(--mv-primary);
  transform-origin: left;
  animation: busy 2.4s ease-in-out infinite;
}

.slots i.is-busy + i.is-busy::after {
  animation-delay: -0.9s;
}

@keyframes busy {
  from {
    transform: scaleX(0);
  }

  to {
    transform: scaleX(1);
  }
}

.limits {
  display: grid;
  margin: 0;
}

.limits div {
  display: flex;
  gap: 16px;
  align-items: baseline;
  justify-content: space-between;
  padding: 10px 0;
  border-bottom: 1px solid var(--mv-line);
}

.limits dt {
  color: var(--mv-sub);
}

.limits dd {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

/* 补偿：写入失败后，已写入的对象被清理 -------------------------------- */
.objects,
.facts {
  display: grid;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.objects li {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  background: var(--mv-soft);
  border-radius: 12px;
}

.objects code {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  overflow-wrap: anywhere;
}

.objects__state {
  position: relative;
  display: inline-flex;
  gap: 5px;
  align-items: center;
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
}

.is-failed .objects__state {
  color: var(--mv-bad);
}

.objects__before,
.objects__after {
  display: inline-flex;
  gap: 5px;
  align-items: center;
}

.objects__before {
  color: var(--mv-ok);
  animation: fade-out 0.4s 1.5s forwards;
}

.objects__after {
  position: absolute;
  right: 0;
  color: var(--mv-sub);
  opacity: 0;
  animation: fade-in 0.4s 1.7s forwards;
}

.is-written code {
  animation: strike 0.4s 1.7s forwards;
}

@keyframes fade-out {
  to {
    opacity: 0;
  }
}

@keyframes fade-in {
  to {
    opacity: 1;
  }
}

@keyframes strike {
  to {
    color: var(--mv-sub);
    text-decoration: line-through;
  }
}

.facts {
  margin-top: 6px;
}

.facts li {
  display: flex;
  gap: 10px;
  align-items: center;
  font-size: 15px;
}

.facts .mv-icon {
  color: var(--mv-primary);
}

/* 窄屏时面板粘在顶部，高度固定，免得切换时把下面的文字顶来顶去。 */
@media (max-width: 959px) {
  .engine {
    justify-content: flex-start;
    min-height: 384px;
    padding: 16px;
    border-radius: 18px;
  }

  .engine__view {
    gap: 12px;
  }

  .pipeline li {
    padding: 4px 0;
    font-size: 14.5px;
  }

  .pipeline li + li::before {
    top: -9px;
    height: 12px;
  }

  .pipeline__done {
    padding: 9px 12px;
    font-size: 13.5px;
  }

  .slots {
    padding: 12px;
  }

  .limits div {
    padding: 7px 0;
    font-size: 14.5px;
  }

  .limits dd {
    font-size: 15px;
  }

  .objects li {
    padding: 9px 12px;
  }

  .objects code {
    font-size: 12px;
  }

  .facts li {
    font-size: 14px;
  }
}
</style>
