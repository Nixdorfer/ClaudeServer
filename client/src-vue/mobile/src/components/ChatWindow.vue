<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import MessageItem from './MessageItem.vue'
import InputArea from './InputArea.vue'
import type { Message } from '../types'
import favicon from '../assets/images/favicon.ico'
const props = defineProps<{
  messages: Message[]
  isLoading: boolean
  isConnected: boolean
  error: string | null
  sendDisabled?: boolean
  conversationId?: string
}>()
const emit = defineEmits<{
  send: [message: string]
  clearError: []
  reportError: [error: string, conversationId: string]
}>()
const messagesContainer = ref<HTMLElement | null>(null)
const lastUserMessage = ref('')
const inputInitialValue = ref('')
watch(() => props.error, (newError, oldError) => {
  if (newError && !oldError) {
    emit('reportError', newError, props.conversationId || '')
    inputInitialValue.value = lastUserMessage.value
  }
})
watch(
  () => props.messages.length,
  async () => {
    await nextTick()
    scrollToBottom()
  }
)
watch(
  () => props.messages[props.messages.length - 1]?.content,
  async () => {
    await nextTick()
    scrollToBottom()
  }
)
function scrollToBottom() {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}
function handleSend(message: string) {
  lastUserMessage.value = message
  inputInitialValue.value = ''
  emit('send', message)
}
</script>

<template>
  <div class="flex-1 flex flex-col bg-chat-bg min-h-0">
    <div v-if="error" class="flex justify-center pt-3 px-3 flex-shrink-0">
      <div class="bg-red-900/70 border border-red-700 rounded-lg px-3 py-2 text-red-200 text-sm flex items-center gap-2 max-w-full">
        <span class="flex-1 break-words whitespace-pre-wrap">{{ error }}</span>
        <button @click="emit('clearError')" class="text-red-400 active:text-red-300 p-1 touch-manipulation flex-shrink-0">
          <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
            <path
              fill-rule="evenodd"
              d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
      </div>
    </div>
    <div ref="messagesContainer" class="flex-1 overflow-y-auto overscroll-contain">
      <div v-if="messages.length === 0" class="h-full flex flex-col items-center justify-center text-zinc-400 px-6">
        <img :src="favicon" alt="Claude" class="w-16 h-16 rounded-full mb-4" />
        <h2 class="text-xl font-medium text-zinc-200 mb-2">Claude 对话</h2>
        <p class="text-center text-sm">在下方输入消息开始对话</p>
      </div>
      <div v-else class="pb-2">
        <MessageItem
          v-for="message in messages"
          :key="message.id"
          :message="message"
        />
      </div>
    </div>
    <InputArea
      :disabled="!isConnected"
      :is-loading="isLoading"
      :send-disabled="sendDisabled"
      :initial-value="inputInitialValue"
      @send="handleSend"
    />
  </div>
</template>
