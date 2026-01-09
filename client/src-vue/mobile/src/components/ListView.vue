<script setup lang="ts">
import { ref, nextTick } from 'vue'
import type { Conversation } from '../types'

const props = defineProps<{
  conversations: Conversation[]
  currentId: string
}>()
const emit = defineEmits<{
  select: [id: string]
  newChat: []
  rename: [id: string, name: string]
  delete: [id: string]
}>()
const editingId = ref<string | null>(null)
const editingName = ref('')
const editInputRef = ref<HTMLInputElement | null>(null)
const swipedId = ref<string | null>(null)
const touchStartX = ref(0)
const touchCurrentX = ref(0)
const isSwiping = ref(false)
const SWIPE_THRESHOLD = 60
const ACTION_WIDTH = 120
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
function truncateText(text: string, maxLength: number = 25): string {
  if (!text) return '新对话'
  return text.length > maxLength ? text.slice(0, maxLength) + '...' : text
}
function handleTouchStart(e: TouchEvent, convId: string) {
  if (editingId.value) return
  touchStartX.value = e.touches[0].clientX
  touchCurrentX.value = e.touches[0].clientX
  isSwiping.value = true
  if (swipedId.value && swipedId.value !== convId) {
    swipedId.value = null
  }
}
function handleTouchMove(e: TouchEvent, convId: string) {
  if (!isSwiping.value || editingId.value) return
  touchCurrentX.value = e.touches[0].clientX
  const diff = touchStartX.value - touchCurrentX.value
  if (diff > 10) {
    e.preventDefault()
  }
}
function handleTouchEnd(convId: string) {
  if (!isSwiping.value || editingId.value) return
  const diff = touchStartX.value - touchCurrentX.value
  if (diff > SWIPE_THRESHOLD) {
    swipedId.value = convId
  } else if (diff < -SWIPE_THRESHOLD) {
    swipedId.value = null
  }
  isSwiping.value = false
}
function getSwipeStyle(convId: string) {
  if (isSwiping.value && swipedId.value !== convId) {
    const diff = touchStartX.value - touchCurrentX.value
    if (diff > 0) {
      return { transform: `translateX(-${Math.min(diff, ACTION_WIDTH)}px)` }
    }
  }
  if (swipedId.value === convId) {
    return { transform: `translateX(-${ACTION_WIDTH}px)` }
  }
  return { transform: 'translateX(0)' }
}
function handleSelect(convId: string) {
  if (swipedId.value) {
    swipedId.value = null
    return
  }
  if (editingId.value !== convId) {
    emit('select', convId)
  }
}
async function startRename(conv: Conversation) {
  swipedId.value = null
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
  swipedId.value = null
  if (confirm('确定删除这个对话吗？')) {
    emit('delete', id)
  }
}
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0">
    <div class="p-4 border-b border-zinc-800">
      <button
        @click="emit('newChat')"
        class="w-full py-3 bg-btn-primary active:bg-btn-hover text-white rounded-lg font-medium flex items-center justify-center gap-2 touch-manipulation"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        新对话
      </button>
    </div>
    <div class="flex-1 overflow-y-auto">
      <div class="py-2">
        <div
          v-for="conv in conversations"
          :key="conv.conversation_id"
          class="relative overflow-hidden"
        >
          <div class="absolute right-0 top-0 bottom-0 flex items-center" :style="{ width: ACTION_WIDTH + 'px' }">
            <button
              @click="startRename(conv)"
              class="flex-1 h-full flex items-center justify-center bg-zinc-600 touch-manipulation"
            >
              <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
              </svg>
            </button>
            <button
              @click="handleDelete(conv.conversation_id)"
              class="flex-1 h-full flex items-center justify-center bg-red-600 touch-manipulation"
            >
              <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </button>
          </div>
          <div
            @click="handleSelect(conv.conversation_id)"
            @touchstart="handleTouchStart($event, conv.conversation_id)"
            @touchmove="handleTouchMove($event, conv.conversation_id)"
            @touchend="handleTouchEnd(conv.conversation_id)"
            class="relative bg-chat-bg px-4 py-3 touch-manipulation transition-transform duration-200"
            :class="conv.conversation_id === currentId ? 'bg-input-bg' : ''"
            :style="getSwipeStyle(conv.conversation_id)"
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
        </div>
        <div v-if="conversations.length === 0" class="text-center py-12 text-zinc-500 text-sm">
          暂无对话记录
        </div>
      </div>
    </div>
  </div>
</template>
