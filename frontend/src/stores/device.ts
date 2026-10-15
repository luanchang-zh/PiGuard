import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Device } from '@/types'

// 前端默认演示设备为 car-001，同时支持从后端拉取设备列表切换。
export const useDeviceStore = defineStore('device', () => {
  const devices = ref<Device[]>([])
  const currentDeviceId = ref<string>('car-001')
  const deviceName = ref<string>('Demo Car')

  function setDevices(list: Device[]) {
    devices.value = list
  }

  function setDevice(id: string, name?: string) {
    currentDeviceId.value = id
    if (name) deviceName.value = name
  }

  return { devices, currentDeviceId, deviceName, setDevices, setDevice }
})
