<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch, nextTick } from 'vue'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'

const props = withDefaults(
  defineProps<{
    option: EChartsOption
    height?: string
    loading?: boolean
  }>(),
  { height: '240px', loading: false },
)

const el = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null

function render() {
  if (!el.value) return
  if (!chart) {
    chart = echarts.init(el.value, 'dark', { renderer: 'canvas' })
  }
  chart.setOption(props.option, { notMerge: false, lazyUpdate: true })
}

function resize() {
  chart?.resize()
}

onMounted(() => {
  render()
  if (el.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      chart?.resize()
    })
    resizeObserver.observe(el.value)
  }
  window.addEventListener('resize', resize)
})

onUnmounted(() => {
  window.removeEventListener('resize', resize)
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (chart) {
    chart.dispose()
    chart = null
  }
})

watch(
  () => props.option,
  () => {
    nextTick(() => {
      if (chart) {
        chart.setOption(props.option, true)
      } else {
        render()
      }
    })
  },
  { deep: true },
)

watch(
  () => props.loading,
  (val) => {
    if (!chart) return
    if (val) {
      chart.showLoading({
        text: '加载中...',
        color: '#22d3ee',
        textColor: '#94a3b8',
        maskColor: 'rgba(6, 10, 19, 0.45)',
      })
    } else {
      chart.hideLoading()
    }
  },
)
</script>

<template>
  <div ref="el" class="pg-chart-container" :style="{ width: '100%', height }" />
</template>

<style scoped>
.pg-chart-container {
  position: relative;
  min-width: 0;
  transition: opacity var(--pg-dur) var(--pg-ease);
}
</style>
