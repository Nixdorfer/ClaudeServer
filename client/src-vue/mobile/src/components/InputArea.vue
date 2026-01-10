<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{
  disabled: boolean
  isLoading: boolean
  sendDisabled?: boolean
  initialValue?: string
}>()
const emit = defineEmits<{
  send: [message: string]
}>()
const inputText = ref('')
watch(() => props.initialValue, (val) => {
  if (val) {
    inputText.value = val
  }
})
const textareaRef = ref<HTMLTextAreaElement | null>(null)
function handleSend() {
  if (inputText.value.trim() && !props.disabled && !props.isLoading && !props.sendDisabled) {
    emit('send', inputText.value)
    inputText.value = ''
    if (textareaRef.value) {
      textareaRef.value.style.height = 'auto'
    }
  }
}
function autoResize() {
  if (textareaRef.value) {
    textareaRef.value.style.height = 'auto'
    if (inputText.value.trim()) {
      textareaRef.value.style.height = Math.min(textareaRef.value.scrollHeight, 120) + 'px'
    }
  }
}
watch(inputText, autoResize)
</script>

<template>
  <div class="border-t border-zinc-800 bg-chat-bg p-3 safe-area-bottom flex-shrink-0">
    <div class="flex items-end gap-2 bg-input-bg rounded-2xl p-2">
      <textarea
        ref="textareaRef"
        v-model="inputText"
        :disabled="disabled"
        placeholder="输入消息..."
        rows="1"
        class="flex-1 bg-transparent text-zinc-200 placeholder-zinc-500 resize-none outline-none px-3 py-2 max-h-[120px] text-base leading-normal"
      ></textarea>
      <button
        @click="handleSend"
        :disabled="disabled || isLoading || !inputText.trim() || sendDisabled"
        class="flex-shrink-0 w-10 h-10 rounded-full transition-colors flex items-center justify-center touch-manipulation"
        :class="[
          inputText.trim() && !disabled && !isLoading && !sendDisabled
            ? 'bg-btn-primary active:bg-btn-hover text-white'
            : 'bg-zinc-700 text-zinc-500'
        ]"
      >
        <svg v-if="!isLoading" class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
        </svg>
        <svg v-else class="w-5 h-5 animate-spin" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </button>
    </div>
  </div>
</template>
