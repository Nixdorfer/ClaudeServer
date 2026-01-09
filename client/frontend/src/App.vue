<script lang="ts" setup>
import { onMounted, onUnmounted } from 'vue'
import Sidebar from './components/Sidebar.vue'
import ChatWindow from './components/ChatWindow.vue'
import { useChat } from './composables/useChat'

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

onMounted(() => {
  initialize()
})

onUnmounted(() => {
  cleanup()
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
  </div>
</template>
