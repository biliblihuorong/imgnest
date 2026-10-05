<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NButton,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSwitch,
  useMessage,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import {
  computed,
  nextTick,
  onScopeDispose,
  reactive,
  shallowRef,
  useTemplateRef,
  watch,
} from "vue";
import { createAlbum, updateAlbum, type AlbumInput, type AlbumView } from "@/api/albums";
import { formatApiError } from "@/locales/errors";
import AlbumCoverPicker from "./AlbumCoverPicker.vue";

const props = defineProps<{ show: boolean; album: AlbumView | null }>();
const emit = defineEmits<{ "update:show": [show: boolean]; saved: [album: AlbumView] }>();
const { t, locale } = useI18n();
const message = useMessage();
const formRef = useTemplateRef<FormInst>("formRef");
const saving = shallowRef(false);
const pickerShow = shallowRef(false);
const model = reactive({ name: "", intro: "", is_public: false, cover_image_id: 0 });
let session = 0;
let validated = false;

const rules = computed<FormRules>(() => ({
  name: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error => {
        validated = true;
        if (!value.trim()) return new Error(t("albums.nameRequired"));
        return [...value.trim()].length <= 100 ? true : new Error(t("albums.nameTooLong"));
      },
    },
  ],
  intro: [
    {
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error => {
        validated = true;
        return [...value].length <= 500 ? true : new Error(t("albums.introTooLong"));
      },
    },
  ],
}));

watch(
  () => [props.show, props.album] as const,
  ([show, album]) => {
    session += 1;
    saving.value = false;
    validated = false;
    pickerShow.value = false;
    formRef.value?.restoreValidation();
    if (!show) return;
    Object.assign(model, {
      name: album?.name ?? "",
      intro: album?.intro ?? "",
      is_public: album?.is_public ?? false,
      cover_image_id: album?.cover_image_id ?? 0,
    });
  },
  { immediate: true },
);

watch(locale, async () => {
  if (!props.show || !validated) return;
  await nextTick();
  await formRef.value?.validate().catch(() => undefined);
});
onScopeDispose(() => {
  session += 1;
});

function close(): void {
  if (saving.value) return;
  session += 1;
  pickerShow.value = false;
  emit("update:show", false);
}

async function submit(): Promise<void> {
  if (saving.value || !props.show) return;
  saving.value = true;
  validated = true;
  const current = session;
  const albumId = props.album?.id;
  try {
    await formRef.value?.validate();
  } catch {
    if (current === session) saving.value = false;
    return;
  }
  if (current !== session || !props.show) return;
  try {
    const body: AlbumInput = {
      name: model.name.trim(),
      intro: model.intro.trim(),
      is_public: model.is_public,
      cover_image_id: model.cover_image_id,
    };
    const saved =
      albumId === undefined ? await createAlbum(body) : await updateAlbum(albumId, body);
    if (current !== session || !props.show) return;
    message.success(() => t(albumId === undefined ? "albums.created" : "albums.updated"));
    emit("saved", saved);
    emit("update:show", false);
  } catch (error) {
    if (current === session && props.show)
      message.error(() => formatApiError(error, "albums.saveFailed"));
  } finally {
    if (current === session) saving.value = false;
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="t(album ? 'albums.editTitle' : 'albums.create')"
    class="album-form-modal"
    :style="{ width: 'min(560px, calc(100vw - 32px))' }"
    :closable="!saving"
    :mask-closable="!saving"
    :close-on-esc="!saving"
    @update:show="close"
  >
    <NForm
      ref="formRef"
      :model="model"
      :rules="rules"
      label-placement="top"
      @submit.prevent="submit"
    >
      <NFormItem :label="t('albums.name')" path="name">
        <NInput
          v-model:value="model.name"
          :placeholder="t('albums.namePlaceholder')"
          :disabled="saving"
        />
      </NFormItem>
      <NFormItem :label="t('albums.intro')" path="intro">
        <NInput
          v-model:value="model.intro"
          type="textarea"
          :placeholder="t('albums.introPlaceholder')"
          :rows="3"
          :disabled="saving"
        />
      </NFormItem>
      <NFormItem :label="t('albums.visibility')" path="is_public">
        <NSwitch v-model:value="model.is_public" :disabled="saving">
          <template #checked>{{ t("albums.public") }}</template>
          <template #unchecked>{{ t("albums.private") }}</template>
        </NSwitch>
      </NFormItem>
      <NFormItem :label="t('albums.cover')" path="cover_image_id">
        <div class="album-form__cover">
          <span class="text-sm text-muted-foreground">{{
            model.cover_image_id > 0
              ? t("albums.coverImage", { id: model.cover_image_id })
              : t("albums.coverUnset")
          }}</span>
          <NButton size="small" :disabled="saving" @click="pickerShow = !pickerShow">{{
            t("albums.chooseCover")
          }}</NButton>
          <NButton
            v-if="model.cover_image_id > 0"
            size="small"
            quaternary
            :disabled="saving"
            @click="model.cover_image_id = 0"
            >{{ t("albums.clearCover") }}</NButton
          >
        </div>
      </NFormItem>
      <AlbumCoverPicker
        v-if="pickerShow && show"
        :selected="model.cover_image_id"
        :disabled="saving"
        @select="model.cover_image_id = $event"
      />
    </NForm>
    <template #footer>
      <div class="album-form__footer">
        <NButton :disabled="saving" @click="close">{{ t("albums.cancel") }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="saving" @click="submit">{{
          t(album ? "albums.save" : "albums.submitCreate")
        }}</NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.album-form__cover {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.album-form__footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
