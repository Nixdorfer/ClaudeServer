<script setup lang="ts">
import { ref, nextTick } from 'vue'
import type { Conversation, UsageStatus } from '../types'

defineProps<{
  conversations: Conversation[]
  currentId: string
  isConnected: boolean
  isConnecting: boolean
  reconnectAttempts: number
  usageStatus: UsageStatus | null
  usageBlocked: boolean
  usageBlockMessage: string
  version: string
}>()

const emit = defineEmits<{
  select: [id: string]
  newChat: []
  reconnect: []
  rename: [id: string, name: string]
  delete: [id: string]
  openDeviceNotice: []
}>()

const editingId = ref<string | null>(null)
const editingName = ref('')
const editInputRef = ref<HTMLInputElement | null>(null)
const contextMenuId = ref<string | null>(null)
const contextMenuPos = ref({ x: 0, y: 0 })

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

function truncateText(text: string, maxLength: number = 30): string {
  if (!text) return '新对话'
  return text.length > maxLength ? text.slice(0, maxLength) + '...' : text
}

function handleContextMenu(e: MouseEvent, conv: Conversation) {
  e.preventDefault()
  contextMenuId.value = conv.conversation_id
  contextMenuPos.value = { x: e.clientX, y: e.clientY }
}

function closeContextMenu() {
  contextMenuId.value = null
}

async function startRename(conv: Conversation) {
  editingId.value = conv.conversation_id
  editingName.value = conv.name || conv.first_message || ''
  closeContextMenu()
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

function handleRenameKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault()
    confirmRename()
  } else if (e.key === 'Escape') {
    cancelRename()
  }
}

function handleDelete(id: string) {
  emit('delete', id)
  closeContextMenu()
}

function handleClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.context-menu')) {
    closeContextMenu()
  }
}
</script>

<template>
  <div
    class="w-64 bg-sidebar-bg h-full flex flex-col border-r border-zinc-800"
    @click="handleClickOutside"
  >
    <div class="p-4 border-b border-zinc-800">
      <button
        @click="emit('newChat')"
        class="w-full py-2.5 px-4 bg-btn-primary hover:bg-btn-hover text-white rounded-lg font-medium transition-colors flex items-center justify-center gap-2"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        新对话
      </button>
    </div>

    <div class="px-4 py-2 text-xs flex items-center gap-2">
      <span
        class="w-2 h-2 rounded-full"
        :class="[
          isConnected ? 'bg-green-500' :
          isConnecting ? 'bg-blue-500 animate-pulse' :
          'bg-red-500'
        ]"
      ></span>
      <span class="text-zinc-400">
        <template v-if="isConnected">已连接</template>
        <template v-else-if="isConnecting">
          正在连接<span v-if="reconnectAttempts > 0"> ({{ reconnectAttempts }}/5)</span>
        </template>
        <template v-else>连接失败</template>
      </span>
      <button
        v-if="!isConnected && !isConnecting"
        @click="emit('reconnect')"
        class="ml-auto text-btn-primary hover:text-btn-hover text-xs"
      >
        重新连接
      </button>
    </div>

    <div v-if="usageStatus" class="px-4 py-3 border-b border-zinc-800">
      <div v-if="usageBlocked" class="mb-2 p-2 bg-red-900/30 border border-red-700 rounded-lg">
        <p class="text-xs text-red-400 whitespace-pre-line">{{ usageBlockMessage }}</p>
      </div>

      <div class="space-y-2">
        <div class="flex justify-between items-center">
          <span class="text-xs text-zinc-500">5小时用量</span>
          <span
            class="text-xs font-medium"
            :class="usageStatus.five_hour > usageStatus.limit_five_hour ? 'text-red-400' : 'text-zinc-300'"
          >
            {{ usageStatus.five_hour.toFixed(1) }}%
          </span>
        </div>
        <div class="w-full bg-zinc-700 rounded-full h-1.5">
          <div
            class="h-1.5 rounded-full transition-all duration-300"
            :class="usageStatus.five_hour > usageStatus.limit_five_hour ? 'bg-red-500' : 'bg-btn-primary'"
            :style="{ width: Math.min(usageStatus.five_hour, 100) + '%' }"
          ></div>
        </div>

        <div class="flex justify-between items-center mt-2">
          <span class="text-xs text-zinc-500">周总用量</span>
          <span
            class="text-xs font-medium"
            :class="usageStatus.seven_day > usageStatus.limit_seven_day ? 'text-red-400' : 'text-zinc-300'"
          >
            {{ usageStatus.seven_day.toFixed(1) }}%
          </span>
        </div>
        <div class="w-full bg-zinc-700 rounded-full h-1.5">
          <div
            class="h-1.5 rounded-full transition-all duration-300"
            :class="usageStatus.seven_day > usageStatus.limit_seven_day ? 'bg-red-500' : 'bg-btn-primary'"
            :style="{ width: Math.min(usageStatus.seven_day, 100) + '%' }"
          ></div>
        </div>

        <div class="flex justify-between items-center mt-2">
          <span class="text-xs text-zinc-500">周Sonnet用量</span>
          <span class="text-xs font-medium text-zinc-300">
            {{ usageStatus.seven_day_sonnet.toFixed(1) }}%
          </span>
        </div>
        <div class="w-full bg-zinc-700 rounded-full h-1.5">
          <div
            class="h-1.5 rounded-full bg-blue-500 transition-all duration-300"
            :style="{ width: Math.min(usageStatus.seven_day_sonnet, 100) + '%' }"
          ></div>
        </div>
      </div>
    </div>

    <div class="flex-1 overflow-y-auto">
      <div class="p-2 space-y-1">
        <div
          v-for="conv in conversations"
          :key="conv.conversation_id"
          @click="editingId !== conv.conversation_id && emit('select', conv.conversation_id)"
          @contextmenu="handleContextMenu($event, conv)"
          class="group p-3 rounded-lg cursor-pointer transition-colors"
          :class="[
            conv.conversation_id === currentId
              ? 'bg-input-bg'
              : 'hover:bg-input-bg/50'
          ]"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="flex-1 min-w-0">
              <input
                v-if="editingId === conv.conversation_id"
                ref="editInputRef"
                v-model="editingName"
                @blur="confirmRename"
                @keydown="handleRenameKeydown"
                @click.stop
                class="w-full text-sm text-zinc-200 bg-zinc-700 border border-zinc-600 rounded px-1.5 py-0.5 outline-none focus:border-btn-primary"
              />
              <p v-else class="text-sm text-zinc-200 truncate">
                {{ conv.name || truncateText(conv.first_message || '') }}
              </p>
              <div class="flex items-center gap-2 mt-1">
                <span class="text-xs text-zinc-500">
                  {{ conv.message_count }} 条消息
                </span>
                <span class="text-xs text-zinc-600">
                  {{ formatTime(conv.last_used_time) }}
                </span>
              </div>
            </div>
            <span
              v-if="conv.is_generating"
              class="flex-shrink-0 w-2 h-2 bg-btn-primary rounded-full animate-pulse"
            ></span>
          </div>
        </div>

        <div
          v-if="conversations.length === 0"
          class="text-center py-8 text-zinc-500 text-sm"
        >
          暂无对话记录
        </div>
      </div>
    </div>

    <div class="p-4 border-t border-zinc-800 text-xs text-zinc-500 text-center">
      <div>Claude 对话客户端</div>
      <div
        v-if="version"
        class="mt-1 text-zinc-600 cursor-pointer hover:text-zinc-400 transition-colors"
        @click="emit('openDeviceNotice')"
      >v{{ version }}</div>
    </div>

    <Teleport to="body">
      <div
        v-if="contextMenuId"
        class="context-menu fixed bg-zinc-800 border border-zinc-700 rounded-lg shadow-xl py-1 z-50 min-w-[120px]"
        :style="{ left: contextMenuPos.x + 'px', top: contextMenuPos.y + 'px' }"
      >
        <button
          @click="startRename(conversations.find(c => c.conversation_id === contextMenuId)!)"
          class="w-full px-3 py-2 text-left text-sm text-zinc-200 hover:bg-zinc-700 flex items-center gap-2"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
          </svg>
          重命名
        </button>
        <button
          @click="handleDelete(contextMenuId!)"
          class="w-full px-3 py-2 text-left text-sm text-red-400 hover:bg-zinc-700 flex items-center gap-2"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
          删除
        </button>
      </div>
    </Teleport>
  </div>
</template>
