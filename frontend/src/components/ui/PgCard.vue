<script setup lang="ts">
import PgIcon from './PgIcon.vue'
import type { IconName } from './icons'

withDefaults(
  defineProps<{
    title?: string
    subtitle?: string
    icon?: IconName
    /** 图标与描边强调色，任意 CSS 颜色 */
    accent?: string
    padding?: 'none' | 'sm' | 'md' | 'lg'
    interactive?: boolean
  }>(),
  { padding: 'md', interactive: false, accent: 'var(--pg-primary)' },
)
</script>

<template>
  <section class="pg-card" :class="[`pad-${padding}`, { interactive }]" :style="{ '--accent': accent }">
    <header v-if="title || $slots.header || $slots.extra" class="pg-card__head">
      <slot name="header">
        <div class="pg-card__title">
          <span v-if="icon" class="pg-card__icon"><PgIcon :name="icon" :size="16" /></span>
          <div class="pg-card__titles">
            <h3>{{ title }}</h3>
            <p v-if="subtitle">{{ subtitle }}</p>
          </div>
        </div>
      </slot>
      <div v-if="$slots.extra" class="pg-card__extra"><slot name="extra" /></div>
    </header>
    <div class="pg-card__body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.pg-card {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: var(--pg-surface);
  border: 1px solid var(--pg-border);
  border-radius: var(--pg-radius-lg);
  box-shadow: var(--pg-shadow), var(--pg-inset-highlight);
  backdrop-filter: blur(18px) saturate(140%);
  -webkit-backdrop-filter: blur(18px) saturate(140%);
  transition:
    border-color var(--pg-dur) var(--pg-ease),
    transform var(--pg-dur) var(--pg-ease),
    box-shadow var(--pg-dur) var(--pg-ease);
}

/* 顶部高光线 */
.pg-card::before {
  content: '';
  position: absolute;
  inset: 0 24px auto;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.09), transparent);
  pointer-events: none;
}

.pg-card.interactive:hover {
  border-color: var(--pg-border-strong);
  transform: translateY(-2px);
  box-shadow: var(--pg-shadow-lg), var(--pg-inset-highlight);
}

.pg-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px 0;
  min-height: 52px;
}

.pg-card__title {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.pg-card__icon {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent) 22%, transparent);
  flex-shrink: 0;
}

.pg-card__titles {
  min-width: 0;
}

.pg-card__titles h3 {
  font-size: 14.5px;
  font-weight: 640;
  color: var(--pg-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pg-card__titles p {
  margin-top: 1px;
  font-size: 12px;
  color: var(--pg-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pg-card__extra {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.pg-card__body {
  flex: 1;
  min-width: 0;
  min-height: 0;
}

.pad-none .pg-card__body { padding: 0; }
.pad-sm .pg-card__body { padding: 12px 14px; }
.pad-md .pg-card__body { padding: 16px 20px 20px; }
.pad-lg .pg-card__body { padding: 24px 28px 28px; }
</style>
