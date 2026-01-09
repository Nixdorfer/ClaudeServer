<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { marked } from 'marked'
import Sidebar from './components/Sidebar.vue'
import ChatWindow from './components/ChatWindow.vue'
import Modal from './components/Modal.vue'
import { useChat } from './composables/useChat'
import { CheckForUpdate, GetCurrentVersion, GetNotice } from '../wailsjs/go/main/App'
import { EventsOn, EventsOff, BrowserOpenURL } from '../wailsjs/runtime/runtime'
import type { UpdateCheckResult } from './types'

const {
  conversations,
  currentConversationId,
  messages,
  isConnected,
  isLoading,
  error,
  usageStatus,
  usageBlocked,
  usageBlockMessage,
  initialize,
  cleanup,
  sendMessage,
  selectConversation,
  newConversation,
  reconnect,
  clearError,
  renameConversation,
  deleteConversation
} = useChat()

// Update dialog state
const showUpdateDialog = ref(false)
const updateInfo = ref<UpdateCheckResult | null>(null)
const currentVersion = ref('')

// Connection error dialog state
const showConnectionErrorDialog = ref(false)

// Notice dialog state
const showNoticeDialog = ref(false)
const noticeContent = ref('')

// Computed property for rendered markdown
const renderedNotice = computed(() => {
  if (!noticeContent.value) return ''
  return marked(noticeContent.value) as string
})

// Open download URL in browser
function openDownloadPage() {
  if (updateInfo.value?.download_url) {
    BrowserOpenURL(updateInfo.value.download_url)
  }
}

async function checkNotice() {
  try {
    const notice = await GetNotice()
    if (notice && notice.trim()) {
      noticeContent.value = notice
      showNoticeDialog.value = true
    }
  } catch (e) {
    console.error('Failed to get notice:', e)
  }
}

function handleNoticeConfirm() {
  showNoticeDialog.value = false
}

async function checkForUpdates() {
  try {
    currentVersion.value = await GetCurrentVersion()
    const result = await CheckForUpdate() as UpdateCheckResult
    if (result.has_update) {
      updateInfo.value = result
      showUpdateDialog.value = true
    }
  } catch (e) {
    console.error('Failed to check for updates:', e)
  }
}

function handleUpdateConfirm() {
  showUpdateDialog.value = false
}

function handleConnectionErrorConfirm() {
  showConnectionErrorDialog.value = false
}

onMounted(() => {
  initialize()
  checkNotice()
  checkForUpdates()

  // Listen for connection errors
  EventsOn('connection_error', (err: string) => {
    // Show special dialog for server connection failure
    if (err.includes('claude.nixdorfer.com') || err.includes('连接失败') || err.includes('dial')) {
      showConnectionErrorDialog.value = true
    }
  })
})

onUnmounted(() => {
  cleanup()
  EventsOff('connection_error')
})

function handleSend(message: string) {
  sendMessage(message)
}

function handleSelectConversation(id: string) {
  selectConversation(id)
}

function handleRename(id: string, name: string) {
  renameConversation(id, name)
}

function handleDelete(id: string) {
  deleteConversation(id)
}
</script>

<template>
  <div class="flex h-screen bg-chat-bg text-white">
    <Sidebar
      :conversations="conversations"
      :current-id="currentConversationId"
      :is-connected="isConnected"
      :usage-status="usageStatus"
      :usage-blocked="usageBlocked"
      :usage-block-message="usageBlockMessage"
      :version="currentVersion"
      @select="handleSelectConversation"
      @new-chat="newConversation"
      @reconnect="reconnect"
      @rename="handleRename"
      @delete="handleDelete"
    />

    <ChatWindow
      :messages="messages"
      :is-loading="isLoading"
      :is-connected="isConnected"
      :error="error"
      @send="handleSend"
      @clear-error="clearError"
    />

    <!-- Update Available Dialog -->
    <Modal
      :show="showUpdateDialog"
      title="发现新版本"
      confirm-text="知道了"
      :show-cancel="false"
      type="info"
      @confirm="handleUpdateConfirm"
      @cancel="handleUpdateConfirm"
    >
      <div class="space-y-4">
        <div class="flex items-center gap-4 text-sm">
          <div class="text-zinc-400">
            当前版本: <span class="text-zinc-200">{{ updateInfo?.current_version }}</span>
          </div>
          <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" />
          </svg>
          <div class="text-zinc-400">
            最新版本: <span class="text-btn-primary font-medium">{{ updateInfo?.latest_version }}</span>
          </div>
        </div>

        <div class="bg-zinc-800/50 rounded-lg p-4">
          <h4 class="text-sm font-medium text-zinc-300 mb-2">更新内容:</h4>
          <ul class="space-y-1">
            <li
              v-for="(note, index) in updateInfo?.notes"
              :key="index"
              class="text-sm text-zinc-400 flex items-start gap-2"
            >
              <span class="text-btn-primary mt-1">-</span>
              <span>{{ note }}</span>
            </li>
          </ul>
        </div>

        <div v-if="updateInfo?.download_url" class="pt-2">
          <button
            @click="openDownloadPage"
            class="w-full py-2 px-4 bg-btn-primary hover:bg-btn-hover text-white rounded-lg text-sm font-medium transition-colors flex items-center justify-center gap-2"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
            前往下载页面
          </button>
        </div>
      </div>
    </Modal>

    <!-- Connection Error Dialog -->
    <Modal
      :show="showConnectionErrorDialog"
      title="连接失败"
      confirm-text="知道了"
      :show-cancel="false"
      type="error"
      @confirm="handleConnectionErrorConfirm"
      @cancel="handleConnectionErrorConfirm"
    >
      <div class="space-y-4">
        <p class="text-zinc-300">
          无法连接至服务器，服务器可能已经关闭。
        </p>
        <div class="bg-zinc-800/50 rounded-lg p-4">
          <p class="text-sm text-zinc-400 mb-2">请联系系统管理员:</p>
          <div class="space-y-1 text-sm">
            <p class="text-zinc-300">Discord / Telegram / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
          </div>
        </div>
      </div>
    </Modal>

    <!-- Notice Dialog -->
    <Modal
      :show="showNoticeDialog"
      title="公告"
      confirm-text="确定"
      :show-cancel="false"
      type="info"
      @confirm="handleNoticeConfirm"
      @cancel="handleNoticeConfirm"
    >
      <div class="prose prose-invert prose-sm max-w-none" v-html="renderedNotice"></div>
    </Modal>
  </div>
</template>
