<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "@vben/locales";
import { bytesMB, type Field, type ParseResult } from "@/domain/search/query";
const props = defineProps<{
  parsed: ParseResult;
  authorizedAlbums: { id: string; name: string }[];
}>();
const emit = defineEmits<{ edit: [field: Field]; remove: [field: Field] }>();
const { t } = useI18n();
const chips = computed(() => {
  if (!props.parsed.ok) return [];
  const f = props.parsed.ast.filters;
  const values: Partial<Record<Field, string>> = {};
  if (f.formats.length) values.format = f.formats.map((x) => x.toUpperCase()).join(" / ");
  if (f.albums.length)
    values.album = f.albums
      .map((album) =>
        album.kind === "unfiled"
          ? t("search.unfiled")
          : album.kind === "name"
            ? album.value
            : `${props.authorizedAlbums.find((x) => x.id === album.value)?.name ?? t("search.album")} (#${album.value})`,
      )
      .join(" / ");
  if (f.camera) values.camera = f.camera;
  if (f.minBytes !== null) values.minsize = bytesMB(f.minBytes);
  if (f.maxBytes !== null) values.maxsize = bytesMB(f.maxBytes);
  if (f.after) values.after = f.after;
  if (f.before) values.before = f.before;
  if (f.visibility !== "all" || props.parsed.tokens.some((token) => token.field === "visibility"))
    values.visibility = t(`search.values.${f.visibility}`);
  if (
    props.parsed.ast.sort !== "newest" ||
    props.parsed.tokens.some((token) => token.field === "sort")
  )
    values.sort = t(`search.values.${props.parsed.ast.sort}`);
  return Object.entries(values).map(([field, value]) => ({
    field: field as Field,
    label: `${t(`search.fields.${field}`)}: ${value}`,
  }));
});
</script>
<template>
  <div v-if="chips.length" class="search-chips" :aria-label="t('search.conditions')">
    <span v-for="chip in chips" :key="chip.field" class="search-chip">
      <button
        type="button"
        class="search-chip__edit"
        :title="t('search.editCondition', { condition: chip.label })"
        @click="emit('edit', chip.field)"
        @keydown.delete.prevent="emit('remove', chip.field)"
      >
        {{ chip.label }}
      </button>
      <button
        type="button"
        class="search-chip__remove"
        :data-remove="chip.field"
        :aria-label="t('search.removeCondition', { condition: chip.label })"
        @click="emit('remove', chip.field)"
      >
        ×
      </button>
    </span>
  </div>
</template>
<style scoped>
.search-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.search-chip {
  display: inline-flex;
  align-items: stretch;
  max-width: 100%;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  background: hsl(var(--muted) / 0.6);
}
.search-chip__edit {
  min-width: 0;
  overflow-wrap: anywhere;
  text-align: start;
  padding: 4px 8px;
}
.search-chip__remove {
  min-width: 32px;
  border-inline-start: 1px solid hsl(var(--border));
}
.search-chip button:focus-visible {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}
@media (max-width: 600px) {
  .search-chip button {
    min-height: 44px;
  }
  .search-chip__remove {
    min-width: 44px;
  }
}
</style>
