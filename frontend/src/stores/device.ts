import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { listDevices } from '@/api/device'
import type { Device } from '@/types'

const LAST_DEVICE_KEY = 'piguard_active_device_id'

export const useDeviceStore = defineStore('device', () => {
  const devices = ref<Device[]>([])
  const currentDeviceId = ref<string>(localStorage.getItem(LAST_DEVICE_KEY) || 'car-001')
  const deviceName = ref<string>('Demo Car (树莓派 4B)')
  const loading = ref(false)

  const currentDevice = computed(() => {
    return devices.value.find((d) => d.device_id === currentDeviceId.value) || null
  })

  function setDevices(list: Device[]) {
    devices.value = list
    const found = list.find((d) => d.device_id === currentDeviceId.value)
    if (found) {
      deviceName.value = found.name
    } else if (list.length > 0) {
      setDevice(list[0].device_id, list[0].name)
    }
  }

  function setDevice(id: string, name?: string) {
    currentDeviceId.value = id
    localStorage.setItem(LAST_DEVICE_KEY, id)
    if (name) {
      deviceName.value = name
    } else {
      const match = devices.value.find((d) => d.device_id === id)
      if (match) deviceName.value = match.name
    }
  }

  async function refreshDevices() {
    loading.value = true
    try {
      const list = await listDevices(true)
      if (Array.isArray(list) && list.length > 0) {
        setDevices(list)
      }
    } catch {
      /* 后端未就绪时保留已有设置 */
    } finally {
      loading.value = false
    }
  }

  return {
    devices,
    currentDeviceId,
    deviceName,
    currentDevice,
    loading,
    setDevices,
    setDevice,
    refreshDevices,
  }
})
