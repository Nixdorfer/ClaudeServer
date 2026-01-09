<script setup lang="ts">
import { ref, nextTick } from 'vue'
import type { Conversation, UsageStatus } from '../types'

const props = defineProps<{
  show: boolean
  conversations: Conversation[]
  currentId: string
  isConnected: boolean
  usageStatus: UsageStatus | null
  usageBlocked: boolean
  usageBlockMessage: string
}>()
const emit = defineEmits<{
  close: []
  select: [id: string]
  newChat: []
  reconnect: []
  rename: [id: string, name: string]
  delete: [id: string]
}>()
const startX = ref(0)
const currentX = ref(0)
const isDragging = ref(false)
const editingId = ref<string | null>(null)
const editingName = ref('')
const editInputRef = ref<HTMLInputElement | null>(null)
function handleTouchStart(e: TouchEvent) {
  startX.value = e.touches[0].clientX
  isDragging.value = true
}
function handleTouchMove(e: TouchEvent) {
  if (!isDragging.value) return
  currentX.value = e.touches[0].clientX - startX.value
  if (currentX.value > 0) currentX.value = 0
}
function handleTouchEnd() {
  if (currentX.value < -80) {
    emit('close')
  }
  currentX.value = 0
  isDragging.value = false
}
function formatTime(timeStr: string): string {
  const date = new Date(timeStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  const days = Math.floor(diff / (1000 * 60 * 60 * 24))
  if (days === 0) {
    return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  } else if (days === 1) {
    return '昨天'
  } else if (days < 7) {
    return `${days}天前`
  } else {
    return date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
  }
}
function truncateText(text: string, maxLength: number = 20): string {
  if (!text) return '新对话'
  return text.length > maxLength ? text.slice(0, maxLength) + '...' : text
}
async function startRename(conv: Conversation) {
  editingId.value = conv.conversation_id
  editingName.value = conv.name || conv.first_message || ''
  await nextTick()
  editInputRef.value?.focus()
  editInputRef.value?.select()
}
function confirmRename() {
  if (editingId.value && editingName.value.trim()) {
    emit('rename', editingId.value, editingName.value.trim())
  }
  cancelRename()
}
function cancelRename() {
  editingId.value = null
  editingName.value = ''
}
function handleDelete(id: string) {
  if (confirm('确定删除这个对话吗？')) {
    emit('delete', id)
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer">
      <div v-if="show" class="fixed inset-0 z-50">
        <div class="absolute inset-0 bg-black/60" @click="emit('close')"></div>
        <div
          class="absolute left-0 top-0 h-full w-72 bg-sidebar-bg flex flex-col safe-area-left"
          :style="{ transform: `translateX(${currentX}px)` }"
          @touchstart="handleTouchStart"
          @touchmove="handleTouchMove"
          @touchend="handleTouchEnd"
        >
          <div class="p-4 border-b border-zinc-800 safe-area-top">
            <button
              @click="emit('newChat'); emit('close')"
              class="w-full py-3 bg-btn-primary active:bg-btn-hover text-white rounded-lg font-medium flex items-center justify-center gap-2 touch-manipulation"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
              </svg>
              新对话
            </button>
          </div>
          <div class="px-4 py-3 flex items-center gap-2 border-b border-zinc-800">
            <span class="w-2 h-2 rounded-full" :class="isConnected ? 'bg-green-500' : 'bg-red-500'"></span>
            <span class="text-sm text-zinc-400">{{ isConnected ? '已连接' : '未连接' }}</span>
            <button
              v-if="!isConnected"
              @click="emit('reconnect')"
              class="ml-auto text-btn-primary active:text-btn-hover text-sm touch-manipulation"
            >
              重连
            </button>
          </div>
          <div v-if="usageStatus" class="px-4 py-3 border-b border-zinc-800">
            <div v-if="usageBlocked" class="mb-2 p-2 bg-red-900/30 border border-red-700 rounded-lg">
              <p class="text-xs text-red-400 whitespace-pre-line">{{ usageBlockMessage }}</p>
            </div>
            <div class="space-y-2">
              <div class="flex justify-between items-center">
                <span class="text-xs text-zinc-500">5小时</span>
                <span
                  class="text-xs font-medium"
                  :class="usageStatus.five_hour > usageStatus.limit_five_hour ? 'text-red-400' : 'text-zinc-300'"
                >{{ usageStatus.five_hour.toFixed(1) }}%</span>
              </div>
              <div class="w-full bg-zinc-700 rounded-full h-1.5">
                <div
                  class="h-1.5 rounded-full transition-all"
                  :class="usageStatus.five_hour > usageStatus.limit_five_hour ? 'bg-red-500' : 'bg-btn-primary'"
                  :style="{ width: Math.min(usageStatus.five_hour, 100) + '%' }"
                ></div>
              </div>
              <div class="flex justify-between items-center mt-2">
                <span class="text-xs text-zinc-500">周用量</span>
                <span
                  class="text-xs font-medium"
                  :class="usageStatus.seven_day > usageStatus.limit_seven_day ? 'text-red-400' : 'text-zinc-300'"
                >{{ usageStatus.seven_day.toFixed(1) }}%</span>
              </div>
              <div class="w-full bg-zinc-700 rounded-full h-1.5">
                <div
                  class="h-1.5 rounded-full transition-all"
                  :class="usageStatus.seven_day > usageStatus.limit_seven_day ? 'bg-red-500' : 'bg-btn-primary'"
                  :style="{ width: Math.min(usageStatus.seven_day, 100) + '%' }"
                ></div>
              </div>
            </div>
          </div>
          <div class="flex-1 overflow-y-auto">
            <div class="py-2">
              <div
                v-for="conv in conversations"
                :key="conv.conversation_id"
                class="group relative"
              >
                <div
                  @click="editingId !== conv.conversation_id && (emit('select', conv.conversation_id), emit('close'))"
                  class="px-4 py-3 active:bg-zinc-800 touch-manipulation"
                  :class="conv.conversation_id === currentId ? 'bg-input-bg' : ''"
                >
                  <div class="flex items-center gap-2">
                    <div class="flex-1 min-w-0">
                      <input
                        v-if="editingId === conv.conversation_id"
                        ref="editInputRef"
                        v-model="editingName"
                        @blur="confirmRename"
                        @keydown.enter.prevent="confirmRename"
                        @keydown.escape="cancelRename"
                        @click.stop
                        class="w-full text-sm text-zinc-200 bg-zinc-700 border border-zinc-600 rounded px-2 py-1 outline-none focus:border-btn-primary"
                      />
                      <p v-else class="text-sm text-zinc-200 truncate">
                        {{ conv.name || truncateText(conv.first_message || '') }}
                      </p>
                      <div class="flex items-center gap-2 mt-1">
                        <span class="text-xs text-zinc-500">{{ conv.message_count }} 条</span>
                        <span class="text-xs text-zinc-600">{{ formatTime(conv.last_used_time) }}</span>
                      </div>
                    </div>
                    <span v-if="conv.is_generating" class="w-2 h-2 bg-btn-primary rounded-full animate-pulse"></span>
                  </div>
                </div>
                <div class="absolute right-2 top-1/2 -translate-y-1/2 flex gap-1 opacity-0 group-active:opacity-100">
                  <button
                    @click.stop="startRename(conv)"
                    class="p-1.5 bg-zinc-700 rounded touch-manipulation"
                  >
                    <svg class="w-4 h-4 text-zinc-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                    </svg>
                  </button>
                  <button
                    @click.stop="handleDelete(conv.conversation_id)"
                    class="p-1.5 bg-zinc-700 rounded touch-manipulation"
                  >
                    <svg class="w-4 h-4 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                  </button>
                </div>
              </div>
              <div v-if="conversations.length === 0" class="text-center py-8 text-zinc-500 text-sm">
                暂无对话记录
              </div>
            </div>
          </div>
          <div class="p-4 border-t border-zinc-800 text-xs text-zinc-500 text-center safe-area-bottom">
            Claude 对话 (移动版)
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.drawer-enter-active {
  transition: opacity 0.3s ease;
}
.drawer-enter-active > div:last-child {
  transition: transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.drawer-leave-active {
  transition: opacity 0.25s ease;
}
.drawer-leave-active > div:last-child {
  transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from > div:last-child,
.drawer-leave-to > div:last-child {
  transform: translateX(-100%);
}
</style>
