<script setup lang="ts">
import { NButton, NEmpty, NForm, NFormItem, NInput, NModal, NSpin, NSwitch, useMessage, type FormInst, type FormItemRule, type FormRules } from "naive-ui";
import { reactive, ref, watch } from "vue";
import { createAlbum, updateAlbum, type AlbumInput, type AlbumView } from "@/api/albums";
import { listImages, type ImageView } from "@/api/images";

const props = defineProps<{
  show: boolean;
  /** null = 新建；非 null = 编辑并回填。 */
  album: AlbumView | null;
}>();

const emit = defineEmits<{
  "update:show": [show: boolean];
  saved: [album: AlbumView];
}>();

const message = useMessage();

const formRef = ref<FormInst | null>(null);
const saving = ref(false);
const model = reactive({
  name: "",
  intro: "",
  is_public: false,
  cover_image_id: 0,
});

const rules: FormRules = {
  name: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        value.trim().length > 0 ? true : new Error("请输入相册名称"),
    },
  ],
};

/** 封面选择：从最近图片里单选（简化实现，不翻页不搜索）。 */
const pickerShow = ref(false);
const pickerLoading = ref(false);
const pickerLoaded = ref(false);
const pickerImages = ref<ImageView[]>([]);

watch(
  () => props.show,
  (show) => {
    if (!show) {
      return;
    }
    if (props.album) {
      model.name = props.album.name;
      model.intro = props.album.intro;
      model.is_public = props.album.is_public;
      model.cover_image_id = props.album.cover_image_id;
    } else {
      model.name = "";
      model.intro = "";
      model.is_public = false;
      model.cover_image_id = 0;
    }
    pickerShow.value = false;
    pickerImages.value = [];
    pickerLoaded.value = false;
  },
  { immediate: true },
);

async function togglePicker(): Promise<void> {
  pickerShow.value = !pickerShow.value;
  if (pickerShow.value && !pickerLoaded.value) {
    pickerLoading.value = true;
    try {
      const data = await listImages({ page: 1, size: 12 });
      pickerImages.value = data.items;
      pickerLoaded.value = true;
    } catch (error) {
      message.error(`图片列表加载失败：${error instanceof Error ? error.message : "未知错误"}`);
      pickerShow.value = false;
    } finally {
      pickerLoading.value = false;
    }
  }
}

function close(): void {
  emit("update:show", false);
}

async function submit(): Promise<void> {
  if (saving.value) {
    return;
  }
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示，不发请求
    return;
  }
  saving.value = true;
  try {
    const body: AlbumInput = {
      name: model.name.trim(),
      intro: model.intro.trim(),
      is_public: model.is_public,
      cover_image_id: model.cover_image_id,
    };
    const saved = props.album
      ? await updateAlbum(props.album.id, body)
      : await createAlbum(body);
    message.success(props.album ? "相册已更新" : "相册已创建");
    emit("saved", saved);
    emit("update:show", false);
  } catch (error) {
    message.error(error instanceof Error && error.message ? error.message : "保存相册失败");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="album ? '编辑相册' : '新建相册'"
    class="album-form-modal"
    @update:show="close"
  >
    <NForm
      ref="formRef"
      :model="model"
      :rules="rules"
      label-placement="top"
      @submit.prevent="submit"
    >
      <NFormItem label="名称" path="name">
        <NInput
          v-model:value="model.name"
          placeholder="不超过 100 个字符"
          :maxlength="100"
          :disabled="saving"
        />
      </NFormItem>
      <NFormItem label="简介" path="intro">
        <NInput
          v-model:value="model.intro"
          type="textarea"
          placeholder="可选，不超过 500 个字符"
          :maxlength="500"
          :rows="3"
          :disabled="saving"
        />
      </NFormItem>
      <NFormItem label="公开性" path="is_public">
        <NSwitch v-model:value="model.is_public" :disabled="saving">
          <template #checked>公开</template>
          <template #unchecked>私有</template>
        </NSwitch>
      </NFormItem>
      <NFormItem label="封面" path="cover_image_id">
        <div class="album-form__cover">
          <span class="album-form__cover-value">
            {{ model.cover_image_id > 0 ? `图片 #${model.cover_image_id}` : "未设置" }}
          </span>
          <NButton size="tiny" :disabled="saving" @click="togglePicker">从我的图片选择</NButton>
          <NButton
            v-if="model.cover_image_id > 0"
            size="tiny"
            quaternary
            :disabled="saving"
            @click="model.cover_image_id = 0"
          >
            清除封面
          </NButton>
        </div>
      </NFormItem>
      <div v-if="pickerShow" class="album-form__picker">
        <NSpin :show="pickerLoading" size="small">
          <div v-if="pickerImages.length > 0" class="album-form__picker-grid">
            <button
              v-for="image in pickerImages"
              :key="image.id"
              type="button"
              class="album-form__picker-item"
              :class="{ 'album-form__picker-item--active': model.cover_image_id === image.id }"
              :title="image.name"
              @click="model.cover_image_id = image.id"
            >
              <img v-if="image.local_thumb_url" :src="image.local_thumb_url" :alt="image.name" />
              <span v-else class="album-form__picker-fallback">{{ image.name }}</span>
            </button>
          </div>
          <NEmpty v-else-if="!pickerLoading" description="没有图片，先去上传几张吧" size="small" />
        </NSpin>
      </div>
    </NForm>
    <template #footer>
      <div class="album-form__footer">
        <NButton :disabled="saving" @click="close">取消</NButton>
        <NButton type="primary" :loading="saving" @click="submit">
          {{ album ? "保存" : "创建" }}
        </NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.album-form__cover {
  display: flex;
  align-items: center;
  gap: 8px;
}

.album-form__cover-value {
  font-size: 13px;
  color: rgba(128, 128, 128, 1);
}

.album-form__picker {
  margin-top: 4px;
}

.album-form__picker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 8px;
}

.album-form__picker-item {
  height: 64px;
  padding: 0;
  border: 2px solid transparent;
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  background: rgba(128, 128, 128, 0.08);
  display: flex;
  align-items: center;
  justify-content: center;
}

.album-form__picker-item--active {
  border-color: var(--n-color-target, #18a058);
}

.album-form__picker-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.album-form__picker-fallback {
  font-size: 12px;
  color: rgba(128, 128, 128, 0.9);
  padding: 0 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.album-form__footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
