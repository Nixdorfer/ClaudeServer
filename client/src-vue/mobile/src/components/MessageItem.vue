<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'
import type { Message } from '../types'

const props = defineProps<{
  message: Message
}>()
marked.setOptions({
  breaks: true,
  gfm: true
})
const renderedContent = computed(() => {
  if (!props.message.content) return ''
  return marked.parse(props.message.content) as string
})
</script>

<template>
  <div
    class="message-enter py-3"
    :class="message.role === 'user' ? 'bg-transparent' : 'bg-input-bg/30'"
  >
    <div class="px-4 flex gap-3">
      <div
        class="flex-shrink-0 w-7 h-7 rounded-full flex items-center justify-center text-xs font-medium"
        :class="message.role === 'user' ? 'bg-btn-primary' : 'bg-btn-primary'"
      >
        {{ message.role === 'user' ? 'U' : 'C' }}
      </div>
      <div class="flex-1 min-w-0">
        <div class="text-xs text-zinc-500 mb-1">
          {{ message.role === 'user' ? '你' : 'Claude' }}
        </div>
        <div class="message-content text-zinc-200 text-sm break-words" v-html="renderedContent"></div>
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
</template>

<style scoped>
.message-content :deep(p) {
  margin-bottom: 0.5rem;
}
.message-content :deep(p:last-child) {
  margin-bottom: 0;
}
.message-content :deep(pre) {
  background-color: #1a1a1a;
  border-radius: 0.5rem;
  padding: 0.75rem;
  overflow-x: auto;
  margin: 0.5rem 0;
  font-size: 0.8rem;
}
.message-content :deep(code) {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 0.8rem;
}
.message-content :deep(:not(pre) > code) {
  background-color: #2a2a2a;
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  font-size: 0.85em;
}
.message-content :deep(ul),
.message-content :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.25rem;
}
.message-content :deep(ul) {
  list-style-type: disc;
}
.message-content :deep(ol) {
  list-style-type: decimal;
}
.message-content :deep(li) {
  margin: 0.25rem 0;
}
.message-content :deep(h1),
.message-content :deep(h2),
.message-content :deep(h3),
.message-content :deep(h4) {
  font-weight: 600;
  margin: 0.75rem 0 0.5rem 0;
}
.message-content :deep(h1) {
  font-size: 1.25rem;
}
.message-content :deep(h2) {
  font-size: 1.125rem;
}
.message-content :deep(h3) {
  font-size: 1rem;
}
.message-content :deep(blockquote) {
  border-left: 3px solid #4a4a4a;
  padding-left: 0.75rem;
  margin: 0.5rem 0;
  color: #a1a1aa;
}
.message-content :deep(a) {
  color: #60a5fa;
  text-decoration: underline;
}
.message-content :deep(table) {
  border-collapse: collapse;
  margin: 0.5rem 0;
  width: 100%;
  font-size: 0.85rem;
}
.message-content :deep(th),
.message-content :deep(td) {
  border: 1px solid #3f3f46;
  padding: 0.375rem;
  text-align: left;
}
.message-content :deep(th) {
  background-color: #27272a;
  font-weight: 600;
}
.message-content :deep(hr) {
  border: none;
  border-top: 1px solid #3f3f46;
  margin: 0.75rem 0;
}
.message-content :deep(img) {
  max-width: 100%;
  border-radius: 0.5rem;
}
</style>
