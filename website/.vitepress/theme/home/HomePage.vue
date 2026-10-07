<script setup lang="ts">
import { useData, withBase } from "vitepress";
import { computed, reactive, shallowRef, watch } from "vue";
import AdminMock from "./AdminMock.vue";
import EnginePanel from "./EnginePanel.vue";
import ScrollStage from "./ScrollStage.vue";
import { copy, type Layout, type MockLang, type Mode } from "./copy";

/** 首页。文案全部在 copy.ts；这里只管结构和滚动联动。 */
const { lang } = useData();
const base = computed<MockLang>(() => (lang.value.startsWith("zh") ? "zh" : "en"));
const t = computed(() => copy[base.value]);
const guide = computed(() => (base.value === "zh" ? "/guide/" : "/en/guide/"));

/* 界面演示：滚动决定状态；点下面的开关可以临时改，滚到下一段时让回给滚动。 */
const tourStep = shallowRef(0);
const mock = reactive<{ layout: Layout; mode: Mode; lang: MockLang }>({
  layout: "classic",
  mode: "light",
  lang: base.value,
});

watch(
  [tourStep, base],
  () => {
    const state = t.value.tour.steps[tourStep.value].state;
    const other: MockLang = base.value === "zh" ? "en" : "zh";
    mock.layout = state.layout;
    mock.mode = state.mode;
    mock.lang = state.lang === "alt" ? other : base.value;
  },
  { immediate: true },
);

const engineStep = shallowRef(0);

/** 文案里的 `代码` 转成 <code>，其余原样转义。 */
function rich(text: string): string {
  const escaped = text.replace(/[&<>"]/g, (char) => `&#${char.charCodeAt(0)};`);
  return escaped.replace(/`([^`]+)`/g, "<code>$1</code>");
}

const REQUEST = `curl -X POST https://img.example.com/api/v1/upload \\
  -H "Authorization: Bearer 1|9f2c…" \\
  -H "Accept: application/json" \\
  -F file=@photo.png`;

const RESPONSE = `{
  "status": true,
  "message": "上传成功",
  "data": {
    "key": "aZ3kP0qL",
    "name": "66ff1b2a41e07.png",
    "pathname": "2026/10/07/66ff1b2a41e07.png",
    "origin_name": "photo.png",
    "size": 2381.42,
    "links": {
      "url": "https://img.example.com/2026/10/07/66ff1b2a41e07.webp",
      "thumbnail_url": "https://img.example.com/2026/10/07/66ff1b2a41e07_thumbs.webp",
      "webp_url": "…/66ff1b2a41e07.webp",
      "origin_url": "…/66ff1b2a41e07.png"
    }
  }
}`;

const COMPOSE = "docker compose -f deploy/compose.dev.yaml";
const commands = computed(() => {
  const [build, migrate, serve] = t.value.start.comments;
  return `# ${build}
${COMPOSE} build dev

# ${migrate}
${COMPOSE} run --rm dev go run ./cmd/imgnest migrate

# ${serve}
${COMPOSE} run --rm --service-ports dev go run ./cmd/imgnest serve`;
});
</script>

<template>
  <div class="home">
    <header class="hero">
      <h1>{{ t.hero.title }}</h1>
      <p>{{ t.hero.lead }}</p>
      <div class="hero__actions">
        <a class="mv-btn mv-btn--primary" :href="withBase(t.hero.docs)">{{ t.hero.primary }}</a>
        <a class="mv-btn" href="#interface">{{ t.hero.secondary }}</a>
      </div>
    </header>

    <section id="interface" class="band">
      <ScrollStage v-model="tourStep" :steps="t.tour.steps">
        <AdminMock v-bind="mock" />
        <div class="switches">
          <div class="switch" role="group" :aria-label="t.tour.controls.layout">
            <span>{{ t.tour.controls.layout }}</span>
            <button
              v-for="option in ['classic', 'marvis'] as const"
              :key="option"
              type="button"
              :aria-pressed="mock.layout === option"
              @click="mock.layout = option"
            >
              {{ t.tour.options[option] }}
            </button>
          </div>
          <div class="switch" role="group" :aria-label="t.tour.controls.mode">
            <span>{{ t.tour.controls.mode }}</span>
            <button
              v-for="option in ['light', 'dark'] as const"
              :key="option"
              type="button"
              :aria-pressed="mock.mode === option"
              @click="mock.mode = option"
            >
              {{ t.tour.options[option] }}
            </button>
          </div>
          <div class="switch" role="group" :aria-label="t.tour.controls.lang">
            <span>{{ t.tour.controls.lang }}</span>
            <button type="button" :aria-pressed="mock.lang === 'zh'" @click="mock.lang = 'zh'">中文</button>
            <button type="button" :aria-pressed="mock.lang === 'en'" @click="mock.lang = 'en'">English</button>
          </div>
        </div>
      </ScrollStage>
    </section>

    <section class="band">
      <div class="band__head">
        <h2>{{ t.engine.title }}</h2>
        <p>{{ t.engine.lead }}</p>
      </div>
      <ScrollStage v-model="engineStep" :steps="t.engine.steps">
        <EnginePanel :view="engineStep" :copy="t.engine" />
      </ScrollStage>
    </section>

    <section class="band split">
      <div class="split__text">
        <h2>{{ t.compat.title }}</h2>
        <!-- eslint-disable-next-line vue/no-v-html -->
        <p v-html="rich(t.compat.body)" />
        <ul>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <li v-for="point in t.compat.points" :key="point" v-html="rich(point)" />
        </ul>
        <a class="mv-btn" :href="withBase(`${guide}lsky-api`)">{{ t.compat.link }}</a>
      </div>
      <div class="split__code">
        <figure class="snippet">
          <figcaption>{{ t.compat.request }}</figcaption>
          <pre tabindex="0"><code>{{ REQUEST }}</code></pre>
        </figure>
        <figure class="snippet">
          <figcaption>{{ t.compat.response }}</figcaption>
          <pre tabindex="0"><code>{{ RESPONSE }}</code></pre>
        </figure>
      </div>
    </section>

    <section class="band split">
      <div class="split__text">
        <h2>{{ t.start.title }}</h2>
        <p>{{ t.start.body }}</p>
        <a class="mv-btn mv-btn--primary" :href="withBase(t.hero.docs)">{{ t.start.action }}</a>
      </div>
      <div class="split__code">
        <figure class="snippet">
          <pre tabindex="0"><code>{{ commands }}</code></pre>
        </figure>
      </div>
    </section>

    <footer class="home__foot">{{ t.footer }}</footer>
  </div>
</template>

<style scoped>
.home {
  width: min(1200px, 100% - 32px);
  margin-inline: auto;
  color: var(--mv-fg);
}

.home :is(h1, h2, h3, h4, p, ul, figure) {
  margin: 0;
}

/* 首屏 --------------------------------------------------------------- */
.hero {
  display: grid;
  gap: 24px;
  max-width: 880px;
  padding-block: clamp(56px, 11vh, 120px) clamp(24px, 5vh, 48px);
}

.hero h1 {
  font-size: clamp(38px, 6.2vw, 78px);
  font-weight: 750;
  line-height: 1.12;
  letter-spacing: -0.035em;
  text-wrap: balance;
}

.hero p {
  max-width: 36em;
  font-size: clamp(16.5px, 1.5vw, 19px);
  line-height: 1.8;
  color: var(--mv-sub);
}

.hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 8px;
}

/* 分段 --------------------------------------------------------------- */
.band {
  padding-block: clamp(24px, 6vh, 64px);
}

.band__head {
  display: grid;
  gap: 16px;
  max-width: 760px;
  padding-top: clamp(40px, 10vh, 110px);
}

.band h2 {
  font-size: clamp(30px, 4.2vw, 54px);
  font-weight: 750;
  line-height: 1.15;
  letter-spacing: -0.03em;
  text-wrap: balance;
}

.band__head p,
.split__text p {
  font-size: 17px;
  line-height: 1.8;
  color: var(--mv-sub);
}

/* 演示下方的开关 ----------------------------------------------------- */
.switches {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 22px;
  margin-top: 18px;
}

.switch {
  display: flex;
  gap: 2px;
  align-items: center;
  padding: 3px;
  background: var(--mv-card);
  border: 1px solid var(--mv-line);
  border-radius: 12px;
  box-shadow: var(--mv-shadow-sm);
}

.switch span {
  padding: 0 8px 0 9px;
  font-size: 13px;
  color: var(--mv-sub);
}

.switch button {
  min-height: 30px;
  padding: 0 12px;
  font-size: 13.5px;
  color: var(--mv-sub);
  border-radius: 9px;
  transition:
    background 0.25s,
    color 0.25s;
}

.switch button:hover {
  color: var(--mv-fg);
}

.switch button[aria-pressed="true"] {
  font-weight: 600;
  color: var(--mv-fg);
  background: var(--mv-soft-strong);
}

/* 左文右码 ----------------------------------------------------------- */
.split {
  display: grid;
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr);
  gap: clamp(28px, 5vw, 72px);
  align-items: start;
  padding-top: clamp(56px, 12vh, 130px);
}

.split__text {
  display: grid;
  gap: 18px;
  justify-items: start;
}

.split__text h2 {
  font-size: clamp(28px, 3.4vw, 42px);
}

.split__text ul {
  display: grid;
  gap: 8px;
  padding: 0;
  list-style: none;
}

.split__text li {
  position: relative;
  padding-left: 20px;
  line-height: 1.7;
}

.split__text li::before {
  position: absolute;
  top: 0.72em;
  left: 2px;
  width: 7px;
  height: 7px;
  content: "";
  background: var(--mv-primary);
  border-radius: 50%;
}

.split__text :deep(code) {
  padding: 2px 6px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.88em;
  background: var(--mv-soft-strong);
  border-radius: 6px;
}

.split__code {
  display: grid;
  gap: 14px;
  min-width: 0;
}

.snippet {
  overflow: hidden;
  background: var(--mv-card);
  border: 1px solid var(--mv-line);
  border-radius: 18px;
  box-shadow: var(--mv-shadow-sm);
}

.snippet figcaption {
  padding: 10px 18px;
  font-size: 13px;
  color: var(--mv-sub);
  border-bottom: 1px solid var(--mv-line);
}

.snippet pre {
  margin: 0;
  padding: 16px 18px;
  overflow-x: auto;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  line-height: 1.75;
}

.home__foot {
  margin-top: clamp(48px, 10vh, 110px);
  padding-block: 28px 44px;
  font-size: 13.5px;
  color: var(--mv-sub);
  border-top: 1px solid var(--mv-line);
}

@media (max-width: 959px) {
  .split {
    grid-template-columns: minmax(0, 1fr);
  }

  .switches {
    gap: 8px;
    margin-top: 10px;
  }

  .switch span {
    display: none;
  }

  .switch button {
    padding: 0 9px;
  }
}
</style>
