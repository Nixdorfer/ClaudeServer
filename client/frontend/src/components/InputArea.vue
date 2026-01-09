<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{
  disabled: boolean
  isLoading: boolean
}>()

const emit = defineEmits<{
  send: [message: string]
}>()

const inputText = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)

function handleSend() {
  if (inputText.value.trim() && !props.disabled && !props.isLoading) {
    emit('send', inputText.value)
    inputText.value = ''
    if (textareaRef.value) {
      textareaRef.value.style.height = 'auto'
    }
  }
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

function autoResize() {
  if (textareaRef.value) {
    textareaRef.value.style.height = 'auto'
    textareaRef.value.style.height = Math.min(textareaRef.value.scrollHeight, 200) + 'px'
  }
}

watch(inputText, autoResize)
</script>

<template>
  <div class="border-t border-zinc-800 bg-chat-bg p-4">
    <div class="max-w-3xl mx-auto">
      <div class="relative flex items-end gap-2 bg-input-bg rounded-xl p-2">
        <textarea
          ref="textareaRef"
          v-model="inputText"
          @keydown="handleKeydown"
          :disabled="disabled"
          placeholder="输入消息..."
          rows="1"
          class="flex-1 bg-transparent text-zinc-200 placeholder-zinc-500 resize-none outline-none px-2 py-1.5 max-h-[200px]"
        ></textarea>
        <button
          @click="handleSend"
          :disabled="disabled || isLoading || !inputText.trim()"
          class="flex-shrink-0 p-2 rounded-lg transition-colors"
          :class="[
            inputText.trim() && !disabled && !isLoading
              ? 'bg-btn-primary hover:bg-btn-hover text-white'
              : 'bg-zinc-700 text-zinc-500 cursor-not-allowed'
          ]"
        >
          <svg
            v-if="!isLoading"
            class="w-5 h-5"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"
            />
          </svg>
          <svg
            v-else
            class="w-5 h-5 animate-spin"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
        </button>
      </div>
      <p class="text-xs text-zinc-600 text-center mt-2">
        按 Enter 发送，Shift+Enter 换行
      </p>
    </div>
  </div>
</template>
