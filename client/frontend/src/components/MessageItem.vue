<script setup lang="ts">
import type { Message } from '../types'

defineProps<{
  message: Message
}>()
</script>

<template>
  <div
    class="message-enter py-4"
    :class="message.role === 'user' ? 'bg-transparent' : 'bg-input-bg/30'"
  >
    <div class="max-w-3xl mx-auto px-4 flex gap-4">
      <div
        class="flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium"
        :class="message.role === 'user' ? 'bg-btn-primary' : 'bg-btn-primary'"
      >
        {{ message.role === 'user' ? 'U' : 'C' }}
      </div>

      <div class="flex-1 min-w-0">
        <div class="text-xs text-zinc-500 mb-1">
          {{ message.role === 'user' ? '你' : 'Claude' }}
        </div>
        <div class="text-zinc-200 whitespace-pre-wrap break-words">
          {{ message.content }}
          <span
            v-if="message.isStreaming && !message.content"
            class="inline-flex gap-1"
          >
            <span class="typing-dot w-2 h-2 bg-zinc-400 rounded-full"></span>
            <span class="typing-dot w-2 h-2 bg-zinc-400 rounded-full"></span>
            <span class="typing-dot w-2 h-2 bg-zinc-400 rounded-full"></span>
          </span>
          <span
            v-else-if="message.isStreaming"
            class="inline-block w-2 h-4 bg-zinc-400 animate-pulse ml-0.5"
          ></span>
        </div>
      </div>
    </div>
  </div>
</template>
