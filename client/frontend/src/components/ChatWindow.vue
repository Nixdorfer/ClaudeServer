<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import MessageItem from './MessageItem.vue'
import InputArea from './InputArea.vue'
import type { Message } from '../types'

const props = defineProps<{
  messages: Message[]
  isLoading: boolean
  isConnected: boolean
  error: string | null
}>()

const emit = defineEmits<{
  send: [message: string]
  clearError: []
}>()

const messagesContainer = ref<HTMLElement | null>(null)

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
  emit('send', message)
}
</script>

<template>
  <div class="flex-1 flex flex-col bg-chat-bg h-full">
    <div
      v-if="error"
      class="bg-red-900/50 border-b border-red-800 px-4 py-2 text-red-200 text-sm flex items-center justify-between"
    >
      <span>{{ error }}</span>
      <button
        @click="emit('clearError')"
        class="text-red-400 hover:text-red-300"
      >
        <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
          <path
            fill-rule="evenodd"
            d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
            clip-rule="evenodd"
          />
        </svg>
      </button>
    </div>

    <div
      ref="messagesContainer"
      class="flex-1 overflow-y-auto"
    >
      <div
        v-if="messages.length === 0"
        class="h-full flex flex-col items-center justify-center text-zinc-400 px-4"
      >
        <div class="w-16 h-16 rounded-full bg-btn-primary flex items-center justify-center text-white text-2xl font-bold mb-4">
          C
        </div>
        <h2 class="text-xl font-medium text-zinc-200 mb-2">Claude 对话</h2>
        <p class="text-center max-w-md">
          在下方输入消息开始对话
        </p>
        <div class="mt-8 grid grid-cols-2 gap-3 max-w-lg">
          <div class="p-3 bg-input-bg rounded-lg text-sm text-zinc-300 hover:bg-input-bg/70 cursor-pointer transition-colors">
            "解释量子计算"
          </div>
          <div class="p-3 bg-input-bg rounded-lg text-sm text-zinc-300 hover:bg-input-bg/70 cursor-pointer transition-colors">
            "写一个 Python 函数"
          </div>
          <div class="p-3 bg-input-bg rounded-lg text-sm text-zinc-300 hover:bg-input-bg/70 cursor-pointer transition-colors">
            "总结这篇文章"
          </div>
          <div class="p-3 bg-input-bg rounded-lg text-sm text-zinc-300 hover:bg-input-bg/70 cursor-pointer transition-colors">
            "帮我头脑风暴"
          </div>
        </div>
      </div>

      <div v-else>
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
      @send="handleSend"
    />
  </div>
</template>
