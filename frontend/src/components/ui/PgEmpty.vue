<script setup lang="ts">
import PgIcon from './PgIcon.vue'
import type { IconName } from './icons'

withDefaults(
  defineProps<{
    icon?: IconName
    title?: string
    description?: string
    compact?: boolean
  }>(),
  { icon: 'layers', title: '暂无数据', compact: false },
)
</script>

<template>
  <div class="pg-empty" :class="{ compact }">
    <div class="pg-empty__icon">
      <PgIcon :name="icon" :size="compact ? 20 : 26" :stroke="1.6" />
    </div>
    <div class="pg-empty__title">{{ title }}</div>
    <div v-if="description" class="pg-empty__desc">{{ description }}</div>
    <div v-if="$slots.default" class="pg-empty__actions"><slot /></div>
  </div>
</template>

<style scoped>
.pg-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  gap: 6px;
  padding: 36px 16px;
  color: var(--pg-text-muted);
}

.pg-empty.compact {
  padding: 20px 12px;
}

.pg-empty__icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  margin-bottom: 6px;
  border-radius: 18px;
  color: var(--pg-text-faint);
  background: radial-gradient(circle at 30% 20%, rgba(148, 163, 184, 0.14), rgba(148, 163, 184, 0.04));
  border: 1px dashed var(--pg-border-strong);
}

.compact .pg-empty__icon {
  width: 42px;
  height: 42px;
  border-radius: 14px;
}

.pg-empty__title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--pg-text-2);
}

.pg-empty__desc {
  font-size: 12px;
  color: var(--pg-text-faint);
  max-width: 280px;
}

.pg-empty__actions {
  margin-top: 10px;
}
</style>
