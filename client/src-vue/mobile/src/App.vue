<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { marked } from 'marked'
import MobileHeader from './components/MobileHeader.vue'
import MobileDrawer from './components/MobileDrawer.vue'
import ChatWindow from './components/ChatWindow.vue'
import Modal from './components/Modal.vue'
import { useChat } from './composables/useChat'
import { noticeContent } from '../../shared/notice'

const {
  conversations,
  currentConversationId,
  currentConversation,
  messages,
  isConnected,
  isLoading,
  error,
  usageStatus,
  usageBlocked,
  usageBlockMessage,
  isBanned,
  bannedReason,
  serverUnavailable,
  versionOutdated,
  versionOutdatedMessage,
  updateInfo,
  initialize,
  cleanup,
  sendMessage,
  selectConversation,
  newConversation,
  reconnect,
  clearError,
  renameConversation,
  deleteConversation,
  checkForUpdates
} = useChat()
const showDrawer = ref(false)
const showConnectionErrorDialog = ref(false)
const showBannedDialog = ref(true)
const showUsageBlockedDialog = ref(true)
const showServerUnavailableDialog = ref(true)
const showNoticeDialog = ref(false)
const showUpdateDialog = ref(false)
const renderedNotice = computed(() => marked(noticeContent))
const renderedBannedReason = computed(() => marked(bannedReason.value))
const sendDisabled = computed(() => isBanned.value || usageBlocked.value || versionOutdated.value || serverUnavailable.value)
const headerTitle = computed(() => {
  if (currentConversation.value) {
    const name = currentConversation.value.name || currentConversation.value.first_message
    if (name && name.length > 15) {
      return name.slice(0, 15) + '...'
    }
    return name || 'Claude'
  }
  return 'Claude'
})
onMounted(async () => {
  initialize()
  if (noticeContent) {
    showNoticeDialog.value = true
  }
  const result = await checkForUpdates()
  if (result && result.has_update) {
    showUpdateDialog.value = true
  }
})
onUnmounted(() => {
  cleanup()
})
function handleRename(id: string, name: string) {
  renameConversation(id, name)
}
function handleDelete(id: string) {
  deleteConversation(id)
}
</script>

<template>
  <div class="h-screen h-dvh flex flex-col bg-chat-bg text-white overflow-hidden">
    <MobileHeader
      :title="headerTitle"
      :is-connected="isConnected"
      @open-drawer="showDrawer = true"
    />
    <ChatWindow
      class="flex-1 min-h-0"
      :messages="messages"
      :is-loading="isLoading"
      :is-connected="isConnected"
      :error="error"
      :send-disabled="sendDisabled"
      @send="sendMessage"
      @clear-error="clearError"
    />
    <MobileDrawer
      :show="showDrawer"
      :conversations="conversations"
      :current-id="currentConversationId"
      :is-connected="isConnected"
      :usage-status="usageStatus"
      :usage-blocked="usageBlocked"
      :usage-block-message="usageBlockMessage"
      @close="showDrawer = false"
      @select="selectConversation"
      @new-chat="newConversation"
      @reconnect="reconnect"
      @rename="handleRename"
      @delete="handleDelete"
    />
    <Modal
      :show="showConnectionErrorDialog"
      title="连接失败"
      confirm-text="知道了"
      :show-cancel="false"
      type="error"
      @confirm="showConnectionErrorDialog = false"
      @cancel="showConnectionErrorDialog = false"
    >
      <div class="space-y-3">
        <p class="text-zinc-300 text-sm">无法连接至服务器，服务器可能已经关闭。</p>
        <div class="bg-zinc-800/50 rounded-lg p-3">
          <p class="text-xs text-zinc-400 mb-1">请联系管理员:</p>
          <p class="text-sm text-zinc-300">Discord / Telegram / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
        </div>
      </div>
    </Modal>
    <div
      v-if="isBanned && showBannedDialog"
      class="fixed inset-0 z-[9999] bg-black/90 flex items-center justify-center safe-area-all"
      @click.self="showBannedDialog = false"
    >
      <div class="bg-zinc-900 border border-red-500/50 rounded-xl p-6 max-w-sm mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-4">
          <div class="w-10 h-10 bg-red-500/20 rounded-full flex items-center justify-center flex-shrink-0">
            <svg class="w-5 h-5 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
            </svg>
          </div>
          <h2 class="text-lg font-bold text-red-500">设备已被封禁</h2>
        </div>
        <div class="space-y-3">
          <div class="bg-zinc-800/50 rounded-lg p-3">
            <p class="text-xs text-zinc-400 mb-1">管理员留言:</p>
            <div class="text-sm text-zinc-300 prose prose-sm prose-invert max-w-none" v-html="renderedBannedReason"></div>
          </div>
          <div class="bg-zinc-800/50 rounded-lg p-3">
            <p class="text-xs text-zinc-400 mb-1">如有疑问请联系管理员:</p>
            <p class="text-sm text-zinc-300">Discord / Telegram / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
          </div>
          <button
            class="w-full py-2 bg-zinc-700 hover:bg-zinc-600 active:bg-zinc-600 rounded-lg text-sm text-zinc-300 touch-manipulation"
            @click="showBannedDialog = false"
          >
            关闭 (仅可查看历史对话)
          </button>
        </div>
      </div>
    </div>
    <div
      v-if="serverUnavailable && showServerUnavailableDialog"
      class="fixed inset-0 z-[9999] bg-black/90 flex items-center justify-center safe-area-all"
      @click.self="showServerUnavailableDialog = false"
    >
      <div class="bg-zinc-900 border border-yellow-500/50 rounded-xl p-6 max-w-sm mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-4">
          <div class="w-10 h-10 bg-yellow-500/20 rounded-full flex items-center justify-center flex-shrink-0">
            <svg class="w-5 h-5 text-yellow-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <h2 class="text-lg font-bold text-yellow-500">服务器不可用</h2>
        </div>
        <div class="space-y-3">
          <p class="text-zinc-300 text-sm">服务器暂时关闭或遇到异常</p>
          <div class="bg-zinc-800/50 rounded-lg p-3">
            <p class="text-xs text-zinc-400 mb-1">可以联系:</p>
            <p class="text-sm text-zinc-300">Telegram / Discord / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
          </div>
          <button
            class="w-full py-2 bg-zinc-700 hover:bg-zinc-600 active:bg-zinc-600 rounded-lg text-sm text-zinc-300 touch-manipulation"
            @click="showServerUnavailableDialog = false"
          >
            关闭 (仅可查看历史对话)
          </button>
        </div>
      </div>
    </div>
    <div
      v-if="usageBlocked && showUsageBlockedDialog"
      class="fixed inset-0 z-[9999] bg-black/90 flex items-center justify-center safe-area-all"
      @click.self="showUsageBlockedDialog = false"
    >
      <div class="bg-zinc-900 border border-orange-500/50 rounded-xl p-6 max-w-sm mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-4">
          <div class="w-10 h-10 bg-orange-500/20 rounded-full flex items-center justify-center flex-shrink-0">
            <svg class="w-5 h-5 text-orange-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <h2 class="text-lg font-bold text-orange-500">用量已超限</h2>
        </div>
        <div class="space-y-3">
          <p class="text-zinc-300 text-sm whitespace-pre-line">{{ usageBlockMessage }}</p>
          <div class="bg-zinc-800/50 rounded-lg p-3">
            <p class="text-xs text-zinc-400 mb-1">如有疑问请联系管理员:</p>
            <p class="text-sm text-zinc-300">Telegram / Discord / QQ: <span class="text-btn-primary">@Nixdorfer</span></p>
          </div>
          <button
            class="w-full py-2 bg-zinc-700 hover:bg-zinc-600 active:bg-zinc-600 rounded-lg text-sm text-zinc-300 touch-manipulation"
            @click="showUsageBlockedDialog = false"
          >
            关闭 (仅可查看历史对话)
          </button>
        </div>
      </div>
    </div>
    <div
      v-if="versionOutdated"
      class="fixed inset-0 z-[9999] bg-black/90 flex items-center justify-center safe-area-all"
    >
      <div class="bg-zinc-900 border border-red-500/50 rounded-xl p-6 max-w-sm mx-4 shadow-2xl">
        <div class="flex items-center gap-3 mb-4">
          <div class="w-10 h-10 bg-red-500/20 rounded-full flex items-center justify-center flex-shrink-0">
            <svg class="w-5 h-5 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <h2 class="text-lg font-bold text-red-500">版本过时</h2>
        </div>
        <div class="space-y-3">
          <p class="text-zinc-300 text-sm">{{ versionOutdatedMessage }}</p>
          <p class="text-xs text-zinc-500 text-center">此窗口无法关闭</p>
        </div>
      </div>
    </div>
    <Modal
      :show="showNoticeDialog"
      title="公告"
      confirm-text="知道了"
      :show-cancel="false"
      @confirm="showNoticeDialog = false"
      @cancel="showNoticeDialog = false"
    >
      <div class="prose prose-sm prose-invert max-w-none text-zinc-300" v-html="renderedNotice"></div>
    </Modal>
    <Modal
      v-if="updateInfo"
      :show="showUpdateDialog"
      title="发现新版本"
      confirm-text="前往下载"
      cancel-text="稍后"
      @confirm="showUpdateDialog = false"
      @cancel="showUpdateDialog = false"
    >
      <div class="space-y-3">
        <p class="text-zinc-300 text-sm">{{ updateInfo.current_version }} -> {{ updateInfo.latest_version }}</p>
        <div class="bg-zinc-800/50 rounded-lg p-3">
          <p class="text-xs text-zinc-400 mb-2">更新内容:</p>
          <ul class="text-sm text-zinc-300 list-disc list-inside space-y-1">
            <li v-for="(note, idx) in updateInfo.notes" :key="idx">{{ note }}</li>
          </ul>
        </div>
      </div>
    </Modal>
  </div>
</template>
