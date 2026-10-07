<script setup lang="ts">
import { withBase } from "vitepress";
import { computed, onBeforeUnmount, onMounted, shallowRef } from "vue";
import MvIcon from "./MvIcon.vue";
import { mockLabels, type Layout, type MockLang, type Mode } from "./copy";

/**
 * 用 HTML/CSS 画出来的 ImgNest 后台示意，不是截图：布局、深浅色、语言都是属性，
 * 切换时各部分各自过渡。内部按固定设计尺寸排版，再整体缩放到容器宽度。
 */
const props = defineProps<{ layout: Layout; mode: Mode; lang: MockLang }>();

const labels = computed(() => mockLabels[props.lang]);

const NAV = [
  { key: "upload", icon: "upload" },
  { key: "images", icon: "image", active: true },
  { key: "albums", icon: "folder" },
  { key: "gallery", icon: "grid" },
  { key: "dashboard", icon: "chart" },
  { key: "tokens", icon: "key" },
] as const;

const ADMIN = [
  { key: "users", icon: "users" },
  { key: "storages", icon: "database" },
  { key: "policies", icon: "sliders" },
] as const;

/* 缩略图只是色块加一座「山」，呼应 Logo；a/b 是天空的两端颜色。 */
const TILES = [
  { name: "66ff1b2a3c4d5.webp", size: "412 KB", a: "#ffd6a5", b: "#ff9f80" },
  { name: "66ff1b2a41e07.webp", size: "268 KB", a: "#bde0fe", b: "#7fb2ff" },
  { name: "66ff1b2a4a9c1.webp", size: "1.1 MB", a: "#d8f3dc", b: "#74c69d" },
  { name: "66ff1b2a52b3e.webp", size: "96 KB", a: "#e7d7ff", b: "#a78bfa" },
  { name: "66ff1b2a5c0f8.webp", size: "540 KB", a: "#ffe5ec", b: "#ff8fab" },
  { name: "66ff1b2a63d24.webp", size: "187 KB", a: "#fff3b0", b: "#f4a259" },
  { name: "66ff1b2a6b7a9.webp", size: "733 KB", a: "#caf0f8", b: "#48b5c4" },
  { name: "66ff1b2a7411c.webp", size: "305 KB", a: "#e2e8f0", b: "#94a3b8" },
] as const;

const SELECTED = 1;

const frame = shallowRef<HTMLElement>();
const compact = shallowRef(false);
const scale = shallowRef(1);
const design = computed(() => (compact.value ? { w: 640, h: 470 } : { w: 900, h: 560 }));

let observer: ResizeObserver | undefined;
onMounted(() => {
  observer = new ResizeObserver(([entry]) => {
    const width = entry.contentRect.width;
    compact.value = width < 560;
    scale.value = width / design.value.w;
  });
  if (frame.value) observer.observe(frame.value);
});
onBeforeUnmount(() => observer?.disconnect());
</script>

<template>
  <div ref="frame" class="mock-frame" :style="{ aspectRatio: `${design.w} / ${design.h}` }" aria-hidden="true">
    <div
      class="mock"
      :class="[`is-${layout}`, `is-${mode}`, { 'is-compact': compact }]"
      :style="{ width: `${design.w}px`, height: `${design.h}px`, transform: `scale(${scale})` }"
    >
      <aside class="m-side">
        <div class="m-brand">
          <img :src="withBase('/logo.svg')" alt="" />
          <b>ImgNest</b>
          <span class="m-badge">{{ labels.badge }}</span>
        </div>
        <div class="m-search">
          <MvIcon name="search" />
          <span>{{ labels.search }}</span>
          <kbd>Ctrl K</kbd>
        </div>
        <div v-for="item in NAV" :key="item.key" class="m-item" :class="{ 'is-active': 'active' in item }">
          <MvIcon :name="item.icon" />
          <span>{{ labels[item.key] }}</span>
        </div>
        <div class="m-label">{{ labels.admin }}</div>
        <div v-for="item in ADMIN" :key="item.key" class="m-item">
          <MvIcon :name="item.icon" />
          <span>{{ labels[item.key] }}</span>
        </div>
        <div class="m-foot">
          <i class="m-avatar" />
          <span>shell</span>
          <MvIcon name="gear" />
        </div>
      </aside>

      <div class="m-body">
        <div class="m-top">
          <span class="m-crumb">ImgNest <i>/</i> {{ labels.images }}</span>
          <span class="m-top__tools">
            <MvIcon name="search" />
            <MvIcon name="moon" />
            <MvIcon name="globe" />
            <i class="m-avatar" />
          </span>
        </div>
        <div class="m-tabs">
          <span>{{ labels.dashboard }}</span>
          <span class="is-active">{{ labels.images }}</span>
          <span>{{ labels.upload }}</span>
        </div>

        <div class="m-content">
          <div class="m-main">
            <div class="m-head">
              <b>{{ labels.images }}</b>
              <span>{{ labels.count }}</span>
              <span class="m-btn"><MvIcon name="upload" />{{ labels.upload }}</span>
            </div>
            <div class="m-grid">
              <figure v-for="(tile, index) in TILES" :key="tile.name" :class="{ 'is-selected': index === SELECTED }">
                <i class="m-thumb" :style="{ '--a': tile.a, '--b': tile.b }" />
                <figcaption>
                  <span>{{ tile.name }}</span>
                  <small>{{ tile.size }}</small>
                </figcaption>
              </figure>
            </div>
          </div>

          <aside class="m-detail">
            <div class="m-detail__inner">
              <i class="m-thumb" :style="{ '--a': TILES[SELECTED].a, '--b': TILES[SELECTED].b }" />
              <b>{{ TILES[SELECTED].name }}</b>
              <dl>
                <div>
                  <dt>{{ labels.dimensions }}</dt>
                  <dd>4032 × 3024</dd>
                </div>
                <div>
                  <dt>{{ labels.size }}</dt>
                  <dd>2.4 MB</dd>
                </div>
                <div>
                  <dt>{{ labels.webp }}</dt>
                  <dd>268 KB</dd>
                </div>
                <div>
                  <dt>{{ labels.album }}</dt>
                  <dd>{{ labels.albumName }}</dd>
                </div>
              </dl>
              <div class="m-formats"><span>URL</span><span class="is-active">Markdown</span><span>HTML</span></div>
              <span class="m-copy">{{ labels.copy }}</span>
            </div>
          </aside>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mock-frame {
  position: relative;
  width: 100%;
  overflow: hidden;
  border: 1px solid var(--mv-line);
  border-radius: 18px;
  box-shadow: var(--mv-shadow-lg);
}

.mock {
  --m-ease: cubic-bezier(0.22, 1, 0.36, 1);
  --m-t: 0.7s var(--m-ease);
  --m-deep: #f1f3f6;
  --m-card: #fff;
  --m-fg: #1b1b1e;
  --m-sub: #74747e;
  --m-line: #e8e8ec;
  --m-soft: #f3f3f6;
  --m-primary: hsl(212 100% 45%);
  --m-radius: 6px;

  position: absolute;
  top: 0;
  left: 0;
  display: flex;
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--m-fg);
  user-select: none;
  background: var(--m-deep);
  transform-origin: 0 0;
  transition: background var(--m-t), color var(--m-t);
}

.mock.is-marvis {
  --m-deep: #f6f6f8;
  --m-primary: hsl(230 100% 62%);
  --m-radius: 14px;
}

.mock.is-dark {
  --m-deep: #111114;
  --m-card: #1d1d22;
  --m-fg: #f3f3f6;
  --m-sub: #9d9da8;
  --m-line: #2e2e35;
  --m-soft: #29292f;
}

.mock.is-dark.is-marvis {
  --m-primary: hsl(230 100% 70%);
}

.mock * {
  box-sizing: border-box;
}

.mock b {
  font-weight: 650;
}

.mock i {
  font-style: normal;
}

/* 侧栏：经典贴边，新版浮起 ------------------------------------------- */
.m-side {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  width: 184px;
  padding: 14px 10px 10px;
  background: var(--m-card);
  border: 0 solid var(--m-line);
  border-right-width: 1px;
  transition:
    margin var(--m-t),
    border-radius var(--m-t),
    box-shadow var(--m-t),
    padding var(--m-t),
    background var(--m-t),
    border-color var(--m-t);
}

.is-marvis .m-side {
  margin: 12px 0 12px 12px;
  padding-top: 18px;
  border-width: 1px;
  border-radius: 20px;
  box-shadow:
    0 1px 2px rgb(0 0 0 / 4%),
    0 12px 32px rgb(0 0 0 / 7%);
}

.is-dark.is-marvis .m-side {
  box-shadow:
    0 1px 2px rgb(0 0 0 / 30%),
    0 12px 32px rgb(0 0 0 / 40%);
}

.m-brand {
  display: flex;
  gap: 7px;
  align-items: center;
  padding: 0 8px 12px;
  font-size: 14px;
}

.m-brand img {
  width: 22px;
  height: 22px;
}

.m-badge {
  padding: 0 6px;
  font-size: 10.5px;
  color: var(--m-sub);
  background: var(--m-soft);
  border-radius: 5px;
  transition: background var(--m-t);
}

.m-search {
  display: flex;
  gap: 7px;
  align-items: center;
  height: 0;
  padding: 0 9px;
  overflow: hidden;
  color: var(--m-sub);
  background: var(--m-soft);
  border-radius: 10px;
  opacity: 0;
  transition:
    height var(--m-t),
    margin var(--m-t),
    opacity var(--m-t),
    background var(--m-t);
}

.is-marvis .m-search {
  height: 32px;
  margin-bottom: 10px;
  opacity: 1;
}

.m-search span {
  flex: 1;
}

.m-search kbd {
  padding: 0 5px;
  font-family: inherit;
  font-size: 10px;
  background: var(--m-card);
  border: 1px solid var(--m-line);
  border-radius: 5px;
}

.m-item {
  display: flex;
  gap: 9px;
  align-items: center;
  height: 32px;
  padding: 0 10px;
  color: var(--m-sub);
  white-space: nowrap;
  border-radius: 8px;
  transition:
    background var(--m-t),
    color var(--m-t);
}

.m-item.is-active {
  color: var(--m-primary);
  background: color-mix(in srgb, var(--m-primary) 12%, transparent);
}

.is-marvis .m-item.is-active {
  font-weight: 650;
  color: var(--m-fg);
  background: var(--m-soft);
}

.m-label {
  padding: 12px 10px 4px;
  font-size: 11px;
  color: var(--m-sub);
}

.m-foot {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: auto;
  padding: 8px;
  color: var(--m-sub);
}

.m-foot span {
  flex: 1;
  color: var(--m-fg);
}

.m-avatar {
  width: 22px;
  height: 22px;
  background: linear-gradient(135deg, #ffb199, #ff6f91);
  border-radius: 50%;
}

/* 顶栏与标签页：只有经典布局有 --------------------------------------- */
.m-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.m-top,
.m-tabs {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  overflow: hidden;
  background: var(--m-card);
  border-bottom: 1px solid var(--m-line);
  transition:
    height var(--m-t),
    opacity var(--m-t),
    background var(--m-t),
    border-color var(--m-t);
}

.m-top {
  justify-content: space-between;
  height: 44px;
  padding: 0 16px;
  color: var(--m-sub);
}

.m-crumb i {
  margin: 0 5px;
  opacity: 0.5;
}

.m-top__tools {
  display: flex;
  gap: 13px;
  align-items: center;
  font-size: 13px;
}

.m-tabs {
  gap: 4px;
  height: 36px;
  padding: 0 12px;
  font-size: 12px;
  color: var(--m-sub);
}

.m-tabs span {
  padding: 4px 12px;
  border-radius: 5px;
}

.m-tabs .is-active {
  color: var(--m-primary);
  background: color-mix(in srgb, var(--m-primary) 12%, transparent);
}

.is-marvis .m-top,
.is-marvis .m-tabs {
  height: 0;
  border-bottom-width: 0;
  opacity: 0;
}

/* 内容区 ------------------------------------------------------------- */
.m-content {
  display: flex;
  flex: 1;
  min-height: 0;
  padding: 16px;
  transition: padding var(--m-t);
}

.is-marvis .m-content {
  padding: 22px 14px 14px 22px;
}

.m-main {
  flex: 1;
  min-width: 0;
}

.m-head {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 14px;
  color: var(--m-sub);
}

.m-head b {
  font-size: 17px;
  color: var(--m-fg);
  letter-spacing: -0.01em;
}

.m-btn {
  display: flex;
  gap: 6px;
  align-items: center;
  height: 30px;
  margin-left: auto;
  padding: 0 12px;
  font-weight: 600;
  color: #fff;
  background: var(--m-primary);
  border-radius: calc(var(--m-radius) * 0.7 + 2px);
  transition:
    background var(--m-t),
    border-radius var(--m-t);
}

.is-dark .m-btn {
  color: #0d0d12;
}

.m-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.m-grid figure {
  margin: 0;
  overflow: hidden;
  background: var(--m-card);
  border: 1px solid var(--m-line);
  border-radius: var(--m-radius);
  outline: 2px solid transparent;
  outline-offset: 1px;
  transition:
    border-radius var(--m-t),
    background var(--m-t),
    border-color var(--m-t),
    outline-color var(--m-t);
}

.is-marvis .m-grid figure.is-selected {
  outline-color: var(--m-primary);
}

.m-thumb {
  position: relative;
  display: block;
  aspect-ratio: 4 / 3;
  overflow: hidden;
  background: linear-gradient(165deg, var(--a), var(--b));
}

.m-thumb::before {
  position: absolute;
  top: 18%;
  left: 20%;
  width: 14%;
  aspect-ratio: 1;
  content: "";
  background: rgb(255 255 255 / 80%);
  border-radius: 50%;
}

.m-thumb::after {
  position: absolute;
  inset: 58% -10% -40% -10%;
  content: "";
  background: rgb(0 0 0 / 16%);
  border-radius: 50% 60% 0 0;
  transform: rotate(-4deg);
}

.is-dark .m-thumb {
  filter: saturate(0.9) brightness(0.86);
}

.m-grid figcaption {
  display: flex;
  flex-direction: column;
  padding: 7px 9px 8px;
  font-size: 11px;
}

.m-grid figcaption span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.m-grid small {
  font-size: 10.5px;
  color: var(--m-sub);
}

/* 右侧详情面板：只有新版布局有 --------------------------------------- */
.m-detail {
  flex-shrink: 0;
  width: 0;
  margin-left: 0;
  overflow: hidden;
  background: var(--m-card);
  border: 0 solid var(--m-line);
  border-radius: 18px;
  opacity: 0;
  transition:
    width var(--m-t),
    margin var(--m-t),
    opacity var(--m-t),
    background var(--m-t),
    border-color var(--m-t);
}

.is-marvis .m-detail {
  width: 206px;
  margin-left: 14px;
  border-width: 1px;
  opacity: 1;
}

.is-compact .m-detail {
  display: none;
}

.is-compact .m-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.is-compact .m-grid figure:nth-child(n + 7) {
  display: none;
}

.m-detail__inner {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 204px;
  padding: 12px;
}

.m-detail .m-thumb {
  border-radius: 10px;
}

.m-detail b {
  overflow: hidden;
  text-overflow: ellipsis;
}

.m-detail dl {
  display: grid;
  gap: 6px;
  margin: 0;
}

.m-detail dl div {
  display: flex;
  justify-content: space-between;
}

.m-detail dt {
  color: var(--m-sub);
}

.m-detail dd {
  margin: 0;
}

.m-formats {
  display: flex;
  padding: 3px;
  font-size: 11px;
  color: var(--m-sub);
  background: var(--m-soft);
  border-radius: 9px;
}

.m-formats span {
  flex: 1;
  padding: 3px 0;
  text-align: center;
  border-radius: 7px;
}

.m-formats .is-active {
  color: var(--m-fg);
  background: var(--m-card);
  box-shadow: 0 1px 2px rgb(0 0 0 / 10%);
}

.m-copy {
  padding: 7px 0;
  font-weight: 600;
  text-align: center;
  background: var(--m-soft);
  border-radius: 10px;
}
</style>
