import request from './request'
import type { ApiResponse, CommandCreateRequest, CommandCreateResult, CommandView } from '@/types'

export function createCommand(deviceId: string, body: CommandCreateRequest) {
  return request
    .post<ApiResponse<CommandCreateResult>>(`/devices/${deviceId}/commands`, body)
    .then((r) => r.data.data!)
}

export function getCommand(commandId: string) {
  return request
    .get<ApiResponse<CommandView>>(`/commands/${commandId}`, { silent: true })
    .then((r) => r.data.data!)
}
