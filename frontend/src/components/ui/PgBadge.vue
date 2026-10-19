<script setup lang="ts">
import PgIcon from './PgIcon.vue'
import type { IconName } from './icons'
import type { Tone } from '@/utils/meta'

withDefaults(
  defineProps<{
    tone?: Tone
    dot?: boolean
    pulse?: boolean
    icon?: IconName
    size?: 'sm' | 'md'
    variant?: 'soft' | 'outline' | 'solid'
  }>(),
  { tone: 'unknown', dot: false, pulse: false, size: 'md', variant: 'soft' },
)
</script>

<template>
  <span class="pg-badge" :class="[`tone-${tone}`, `size-${size}`, `v-${variant}`]">
    <span v-if="dot" class="pg-badge__dot" :class="{ pulse }" />
    <PgIcon v-if="icon" :name="icon" :size="size === 'sm' ? 12 : 13" :stroke="2.2" />
    <slot />
  </span>
</template>

<style scoped>
.pg-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: var(--pg-radius-pill);
  font-weight: 600;
  white-space: nowrap;
  line-height: 1;
  color: var(--tone);
}

.size-sm {
  font-size: 11px;
  padding: 4px 8px;
}

.size-md {
  font-size: 12px;
  padding: 5px 10px;
}

.v-soft {
  background: var(--tone-soft);
  border: 1px solid color-mix(in srgb, var(--tone) 22%, transparent);
}

.v-outline {
  background: transparent;
  border: 1px solid color-mix(in srgb, var(--tone) 45%, transparent);
}

.v-solid {
  background: var(--tone);
  color: #04121a;
  border: 1px solid transparent;
}

.pg-badge__dot {
  position: relative;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}

.v-solid .pg-badge__dot {
  background: #04121a;
}

.pg-badge__dot.pulse::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: 50%;
  background: currentColor;
  animation: pg-ping 1.6s var(--pg-ease) infinite;
}
</style>
