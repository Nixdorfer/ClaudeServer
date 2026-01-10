<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { marked } from 'marked'
import { Events, Browser } from '@wailsio/runtime'
// @ts-ignore
import * as UpdateService from '../bindings/claudechat/services/updateservice.js'
import Sidebar from './components/Sidebar.vue'
import ChatWindow from './components/ChatWindow.vue'
import Modal from './components/Modal.vue'
import { useChat } from './composables/useChat'
import type { UpdateCheckResult } from './types'
import { noticeContent as staticNoticeContent } from '../../shared/notice'

const {
  conversations,
  currentConversationId,
  messages,
  isConnected,
  isConnecting,
  isLoading,
  error,
  usageStatus,
  usageBlocked,
  usageBlockMessage,
  isBanned,
  bannedReason,
  versionOutdated,
  versionOutdatedMessage,
  offlineMode,
  serverUnavailable,
  reconnectAttempts,
  initialize,
  cleanup,
  sendMessage,
  selectConversation,
  newConversation,
  reconnect,
  clearError,
  renameConversation,
  deleteConversation,
  reportError,
  updateDeviceNotice
} = useChat()

const showUpdateDialog = ref(false)
const updateInfo = ref<UpdateCheckResult | null>(null)
const currentVersion = ref('')
const showConnectionErrorDialog = ref(false)
const showBannedDialog = ref(true)
const showVersionOutdatedDialog = ref(true)
const showNoticeDialog = ref(false)
const noticeContent = ref(staticNoticeContent)
const showUsageBlockedDialog = ref(true)
const showServerUnavailableDialog = ref(true)
const showDeviceNoticeDialog = ref(false)
const deviceNoticeInput = ref('')
const deviceNoticeSending = ref(false)
let connectionErrorUnlisten: (() => void) | null = null
let deviceBannedUnlisten: (() => void) | null = null
let versionOutdatedUnlisten: (() => void) | null = null

const renderedNotice = computed(() => {
  if (!noticeContent.value) return ''
  return marked(noticeContent.value) as string
})
const renderedBannedReason = computed(() => {
  if (!bannedReason.value) return ''
  return marked(bannedReason.value) as string
})
const sendDisabled = computed(() => offlineMode.value || serverUnavailable.value || !isConnected.value)

async function openDownloadPage() {
  if (updateInfo.value?.download_url) {
    Browser.OpenURL(updateInfo.value.download_url)
  }
}

function checkNotice() {
  if (staticNoticeContent && staticNoticeContent.trim()) {
    showNoticeDialog.value = true
  }
}

function handleNoticeConfirm() {
  showNoticeDialog.value = false
}

async function checkForUpdates() {
  try {
    currentVersion.value = await UpdateService.GetCurrentVersion()
    const result = await UpdateService.CheckForUpdate() as unknown as UpdateCheckResult
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

onMounted(async () => {
  initialize()
  checkNotice()
  checkForUpdates()
  document.addEventListener('contextmenu', (e) => {
    e.preventDefault()
  })
  connectionErrorUnlisten = Events.On('connection_error', (event: any) => {
    const err = String(event?.data || event || '')
    if (err.includes('claude.nixdorfer.com') || err.includes('连接失败') || err.includes('dial')) {
      showConnectionErrorDialog.value = true
    }
  })
  deviceBannedUnlisten = Events.On('device_banned', () => {
    showBannedDialog.value = true
  })
  versionOutdatedUnlisten = Events.On('version_outdated', async () => {
    showVersionOutdatedDialog.value = true
    await checkForUpdates()
  })
})

onUnmounted(() => {
  cleanup()
  if (connectionErrorUnlisten) {
    connectionErrorUnlisten()
  }
  if (deviceBannedUnlisten) {
    deviceBannedUnlisten()
  }
  if (versionOutdatedUnlisten) {
    versionOutdatedUnlisten()
  }
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
async function handleReportError(errorMsg: string, conversationId: string) {
  await reportError(errorMsg, conversationId)
}
function handleOpenDeviceNotice() {
  deviceNoticeInput.value = ''
  showDeviceNoticeDialog.value = true
}
async function handleSendDeviceNotice() {
  if (!deviceNoticeInput.value.trim() || deviceNoticeSending.value) return
  deviceNoticeSending.value = true
  const success = await updateDeviceNotice(deviceNoticeInput.value.trim())
  deviceNoticeSending.value = false
  if (success) {
    showDeviceNoticeDialog.value = false
    deviceNoticeInput.value = ''
  }
}
</script>

<template>
  <div class="flex h-screen bg-chat-bg text-white">
    <Sidebar
      :conversations="conversations"
      :current-id="currentConversationId"
      :is-connected="isConnected"
      :is-connecting="isConnecting"
      :reconnect-attempts="reconnectAttempts"
      :usage-status="usageStatus"
      :usage-blocked="usageBlocked"
      :usage-block-message="usageBlockMessage"
      :offline-mode="offlineMode"
      :version="currentVersion"
      @select="handleSelectConversation"
      @new-chat="newConversation"
      @reconnect="reconnect"
      @rename="handleRename"
      @delete="handleDelete"
      @open-device-notice="handleOpenDeviceNotice"
    />

    <ChatWindow
      :messages="messages"
      :is-loading="isLoading"
      :is-connected="isConnected"
      :error="error"
      :send-disabled="sendDisabled"
      :conversation-id="currentConversationId"
      @send="handleSend"
      @clear-error="clearError"
      @report-error="handleReportError"
    />

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

    <div
      v-if="versionOutdated && showVersionOutdatedDialog"
      class="fixed inset-0 z-[10000] bg-black/90 flex items-center justify-center"
    >
      <div class="bg-zinc-900 border border-orange-500/50 rounded-xl p-8 max-w-md mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-6">
          <div class="w-12 h-12 bg-orange-500/20 rounded-full flex items-center justify-center">
            <svg class="w-6 h-6 text-orange-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
          </div>
          <h2 class="text-xl font-bold text-orange-500">版本已过时</h2>
        </div>
        <div class="space-y-4">
          <p class="text-zinc-300">
            {{ versionOutdatedMessage }}
          </p>
          <div class="bg-zinc-800/50 rounded-lg p-4">
            <p class="text-sm text-zinc-400">请下载最新版本后重新打开应用</p>
          </div>
          <div v-if="updateInfo?.download_url" class="pt-2">
            <button
              @click="openDownloadPage"
              class="w-full py-2 px-4 bg-orange-500 hover:bg-orange-600 text-white rounded-lg text-sm font-medium transition-colors flex items-center justify-center gap-2"
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              前往下载页面
            </button>
          </div>
          <p class="text-xs text-zinc-500 text-center">
            此窗口无法关闭
          </p>
        </div>
      </div>
    </div>

    <div
      v-if="isBanned && showBannedDialog"
      class="fixed inset-0 z-[9999] bg-black/90 flex items-center justify-center"
      @click.self="showBannedDialog = false"
    >
      <div class="bg-zinc-900 border border-red-500/50 rounded-xl p-8 max-w-md mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-6">
          <div class="w-12 h-12 bg-red-500/20 rounded-full flex items-center justify-center">
            <svg class="w-6 h-6 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
            </svg>
          </div>
          <h2 class="text-xl font-bold text-red-500">设备已被封禁</h2>
        </div>
        <div class="space-y-4">
          <div class="bg-zinc-800/50 rounded-lg p-4">
            <p class="text-sm text-zinc-400 mb-2">管理员留言:</p>
            <div class="text-zinc-300 prose prose-sm prose-invert max-w-none" v-html="renderedBannedReason"></div>
          </div>
          <div class="bg-zinc-800/50 rounded-lg p-4">
            <p class="text-sm text-zinc-400 mb-2">如有疑问请联系管理员:</p>
            <div class="space-y-1 text-sm">
              <p class="text-zinc-300">Discord / Telegram / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
            </div>
          </div>
          <button
            class="w-full py-2 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-sm text-zinc-300"
            @click="showBannedDialog = false"
          >
            关闭 (仅可查看历史对话)
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="usageBlocked && showUsageBlockedDialog"
      class="fixed inset-0 z-[9998] bg-black/90 flex items-center justify-center"
      @click.self="showUsageBlockedDialog = false"
    >
      <div class="bg-zinc-900 border border-orange-500/50 rounded-xl p-8 max-w-md mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-6">
          <div class="w-12 h-12 bg-orange-500/20 rounded-full flex items-center justify-center">
            <svg class="w-6 h-6 text-orange-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <h2 class="text-xl font-bold text-orange-500">已超过管理员设定用量</h2>
        </div>
        <div class="space-y-4">
          <p class="text-zinc-300 whitespace-pre-line">
            {{ usageBlockMessage }}
          </p>
          <div class="bg-zinc-800/50 rounded-lg p-4">
            <p class="text-sm text-zinc-400 mb-2">如有疑问请联系管理员:</p>
            <div class="space-y-1 text-sm">
              <p class="text-zinc-300">Discord / Telegram / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
            </div>
          </div>
          <button
            class="w-full py-2 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-sm text-zinc-300"
            @click="showUsageBlockedDialog = false"
          >
            关闭 (仅可查看历史对话)
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="serverUnavailable && showServerUnavailableDialog"
      class="fixed inset-0 z-[9997] bg-black/90 flex items-center justify-center"
      @click.self="showServerUnavailableDialog = false"
    >
      <div class="bg-zinc-900 border border-red-500/50 rounded-xl p-8 max-w-md mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-6">
          <div class="w-12 h-12 bg-red-500/20 rounded-full flex items-center justify-center">
            <svg class="w-6 h-6 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
            </svg>
          </div>
          <h2 class="text-xl font-bold text-red-500">服务器暂时不可用</h2>
        </div>
        <div class="space-y-4">
          <p class="text-zinc-300">
            服务器暂时关闭或遇到异常，请稀后再试。
          </p>
          <div class="bg-zinc-800/50 rounded-lg p-4">
            <p class="text-sm text-zinc-400 mb-2">如有疑问请联系管理员:</p>
            <div class="space-y-1 text-sm">
              <p class="text-zinc-300">Telegram / Discord / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
            </div>
          </div>
          <button
            class="w-full py-2 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-sm text-zinc-300"
            @click="showServerUnavailableDialog = false"
          >
            关闭 (仅可查看历史对话)
          </button>
        </div>
      </div>
    </div>
    <Modal
      :show="showDeviceNoticeDialog"
      title="设备备注"
      confirm-text="发送"
      cancel-text="取消"
      :show-cancel="true"
      type="info"
      @confirm="handleSendDeviceNotice"
      @cancel="showDeviceNoticeDialog = false"
    >
      <div class="space-y-4">
        <p class="text-sm text-zinc-400">输入备注信息，用于标识此设备</p>
        <input
          v-model="deviceNoticeInput"
          type="text"
          placeholder="例如：我的笔记本电脑"
          class="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-zinc-200 text-sm placeholder-zinc-500 focus:outline-none focus:border-btn-primary"
          @keydown.enter="handleSendDeviceNotice"
        />
        <p v-if="deviceNoticeSending" class="text-xs text-zinc-500">正在发送...</p>
      </div>
    </Modal>
  </div>
</template>
