<script setup lang="ts">
import { computed } from 'vue'
import { ICONS, type IconName } from './icons'

const props = withDefaults(
  defineProps<{
    name: IconName
    size?: number | string
    stroke?: number
    spin?: boolean
  }>(),
  { size: 18, stroke: 1.9, spin: false },
)

const px = computed(() => (typeof props.size === 'number' ? `${props.size}px` : props.size))
</script>

<template>
  <svg
    class="pg-icon"
    :class="{ spin }"
    :width="px"
    :height="px"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    :stroke-width="stroke"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    v-html="ICONS[name]"
  />
</template>

<style scoped>
.pg-icon {
  flex-shrink: 0;
  display: inline-block;
  vertical-align: middle;
}

.spin {
  animation: pg-spin 0.9s linear infinite;
}
</style>
