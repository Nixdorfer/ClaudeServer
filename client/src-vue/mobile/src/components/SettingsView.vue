<script setup lang="ts">
import { ref } from 'vue'
import type { UsageStatus } from '../types'

defineProps<{
  isConnected: boolean
  isConnecting: boolean
  reconnectAttempts: number
  usageStatus: UsageStatus | null
  usageBlocked: boolean
  usageBlockMessage: string
  version: string
}>()
const emit = defineEmits<{
  reconnect: []
  updateDeviceNotice: [notice: string]
}>()
const deviceNoticeInput = ref('')
const deviceNoticeSending = ref(false)
const deviceNoticeSuccess = ref(false)
async function handleSendDeviceNotice() {
  if (!deviceNoticeInput.value.trim() || deviceNoticeSending.value) return
  deviceNoticeSending.value = true
  deviceNoticeSuccess.value = false
  emit('updateDeviceNotice', deviceNoticeInput.value.trim())
}
function onNoticeSent(success: boolean) {
  deviceNoticeSending.value = false
  if (success) {
    deviceNoticeSuccess.value = true
    deviceNoticeInput.value = ''
    setTimeout(() => {
      deviceNoticeSuccess.value = false
    }, 2000)
  }
}
defineExpose({ onNoticeSent })
</script>

<template>
  <div class="flex-1 overflow-y-auto p-4">
    <div class="space-y-4">
      <div class="bg-sidebar-bg rounded-xl p-4">
        <h3 class="text-sm font-medium text-zinc-300 mb-3">连接状态</h3>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span
              class="w-2.5 h-2.5 rounded-full"
              :class="[
                isConnected ? 'bg-green-500' :
                isConnecting ? 'bg-blue-500 animate-pulse' :
                'bg-red-500'
              ]"
            ></span>
            <span class="text-sm" :class="isConnected ? 'text-green-400' : isConnecting ? 'text-blue-400' : 'text-red-400'">
              <template v-if="isConnected">已连接</template>
              <template v-else-if="isConnecting">
                正在连接<span v-if="reconnectAttempts > 0"> ({{ reconnectAttempts }}/5)</span>
              </template>
              <template v-else>未连接</template>
            </span>
          </div>
          <button
            v-if="!isConnected && !isConnecting"
            @click="emit('reconnect')"
            class="px-4 py-1.5 bg-btn-primary active:bg-btn-hover text-white text-sm rounded-lg touch-manipulation"
          >
            重新连接
          </button>
        </div>
      </div>
      <div v-if="usageStatus" class="bg-sidebar-bg rounded-xl p-4">
        <h3 class="text-sm font-medium text-zinc-300 mb-3">用量统计</h3>
        <div v-if="usageBlocked" class="mb-4 p-3 bg-red-900/30 border border-red-700 rounded-lg">
          <p class="text-sm text-red-400 whitespace-pre-line">{{ usageBlockMessage }}</p>
        </div>
        <div class="space-y-4">
          <div>
            <div class="flex justify-between items-center mb-2">
              <span class="text-sm text-zinc-400">5小时用量</span>
              <span
                class="text-sm font-medium"
                :class="usageStatus.five_hour > usageStatus.limit_five_hour ? 'text-red-400' : 'text-zinc-200'"
              >{{ usageStatus.five_hour.toFixed(1) }}%</span>
            </div>
            <div class="w-full bg-zinc-700 rounded-full h-2">
              <div
                class="h-2 rounded-full transition-all"
                :class="usageStatus.five_hour > usageStatus.limit_five_hour ? 'bg-red-500' : 'bg-btn-primary'"
                :style="{ width: Math.min(usageStatus.five_hour, 100) + '%' }"
              ></div>
            </div>
          </div>
          <div>
            <div class="flex justify-between items-center mb-2">
              <span class="text-sm text-zinc-400">周用量</span>
              <span
                class="text-sm font-medium"
                :class="usageStatus.seven_day > usageStatus.limit_seven_day ? 'text-red-400' : 'text-zinc-200'"
              >{{ usageStatus.seven_day.toFixed(1) }}%</span>
            </div>
            <div class="w-full bg-zinc-700 rounded-full h-2">
              <div
                class="h-2 rounded-full transition-all"
                :class="usageStatus.seven_day > usageStatus.limit_seven_day ? 'bg-red-500' : 'bg-btn-primary'"
                :style="{ width: Math.min(usageStatus.seven_day, 100) + '%' }"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="bg-sidebar-bg rounded-xl p-4">
        <h3 class="text-sm font-medium text-zinc-300 mb-3">用量统计</h3>
        <p class="text-sm text-zinc-500">正在加载用量信息...</p>
      </div>
      <div class="bg-sidebar-bg rounded-xl p-4">
        <h3 class="text-sm font-medium text-zinc-300 mb-3">设备备注</h3>
        <div class="space-y-3">
          <p class="text-xs text-zinc-500">设置此设备的备注名称，方便管理员识别</p>
          <div class="flex gap-2">
            <input
              v-model="deviceNoticeInput"
              type="text"
              placeholder="例如：我的手机"
              class="flex-1 px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-zinc-200 text-sm placeholder-zinc-500 focus:outline-none focus:border-btn-primary"
            />
            <button
              @click="handleSendDeviceNotice"
              :disabled="deviceNoticeSending || !deviceNoticeInput.trim()"
              class="px-4 py-2 bg-btn-primary active:bg-btn-hover disabled:bg-zinc-700 disabled:text-zinc-500 text-white text-sm rounded-lg touch-manipulation"
            >
              <template v-if="deviceNoticeSending">...</template>
              <template v-else>发送</template>
            </button>
          </div>
          <p v-if="deviceNoticeSuccess" class="text-xs text-green-400">备注已更新</p>
        </div>
      </div>
      <div class="bg-sidebar-bg rounded-xl p-4">
        <h3 class="text-sm font-medium text-zinc-300 mb-3">关于</h3>
        <div class="space-y-2 text-sm text-zinc-400">
          <p>Claude Chat (移动版)</p>
          <p>版本: {{ version }}</p>
        </div>
      </div>
    </div>
  </div>
</template>
