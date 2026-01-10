<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { marked } from 'marked'
import hljs from 'highlight.js'
import type { Message } from '../types'
import favicon from '../assets/images/favicon.ico'
const props = defineProps<{
  message: Message
}>()
marked.setOptions({
  breaks: true,
  gfm: true
})
const fullscreenIndex = ref<number | null>(null)
interface CodeBlock {
  code: string
  lang: string
  isHtml: boolean
  showPreview: boolean
  previewMode: 'desktop' | 'mobile'
  highlighted: string
}
const codeBlocks = ref<CodeBlock[]>([])
function highlightCode(code: string, lang: string): string {
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(code, { language: lang }).value
    }
    return hljs.highlightAuto(code).value
  } catch {
    return code.replace(/</g, '&lt;').replace(/>/g, '&gt;')
  }
}
const renderedContent = computed(() => {
  if (!props.message.content) return ''
  codeBlocks.value = []
  let blockIndex = 0
  const content = props.message.content.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => {
    const isHtml = lang.toLowerCase() === 'html'
    const trimmedCode = code.trim()
    codeBlocks.value.push({
      code: trimmedCode,
      lang: lang || 'text',
      isHtml,
      showPreview: isHtml,
      previewMode: 'mobile',
      highlighted: highlightCode(trimmedCode, lang || 'text')
    })
    const placeholder = `<!--CODEBLOCK${blockIndex}-->`
    blockIndex++
    return placeholder
  })
  return marked.parse(content) as string
})
function copyCode(code: string) {
  navigator.clipboard.writeText(code)
}
function togglePreview(index: number) {
  codeBlocks.value[index].showPreview = !codeBlocks.value[index].showPreview
}
function setPreviewMode(index: number, mode: 'desktop' | 'mobile') {
  codeBlocks.value[index].previewMode = mode
}
function openFullscreen(index: number) {
  fullscreenIndex.value = index
}
function closeFullscreen() {
  fullscreenIndex.value = null
}
function handleBackButton() {
  if (fullscreenIndex.value !== null) {
    closeFullscreen()
  }
}
onMounted(() => {
  window.addEventListener('popstate', handleBackButton)
})
onUnmounted(() => {
  window.removeEventListener('popstate', handleBackButton)
})
</script>

<template>
  <div
    class="message-enter py-3"
    :class="message.role === 'user' ? 'bg-transparent' : 'bg-input-bg/30'"
  >
    <div class="px-4 flex gap-3">
      <img v-if="message.role === 'assistant'" :src="favicon" alt="Claude" class="flex-shrink-0 w-7 h-7 rounded-full" />
      <div v-else class="flex-shrink-0 w-7 h-7 rounded-full flex items-center justify-center text-xs font-medium bg-btn-primary">
        U
      </div>
      <div class="flex-1 min-w-0">
        <div class="text-xs text-zinc-500 mb-1">
          {{ message.role === 'user' ? '你' : 'Claude' }}
        </div>
        <div class="message-content text-zinc-200 text-sm break-words">
          <template v-for="(part, idx) in renderedContent.split(/<!--CODEBLOCK(\d+)-->/)" :key="idx">
            <template v-if="idx % 2 === 0">
              <span v-html="part"></span>
            </template>
            <template v-else>
              <div v-if="codeBlocks[parseInt(part)]" class="code-block-wrapper my-2">
                <div class="code-block-header flex items-center justify-between px-3 py-2 bg-zinc-800 rounded-t-lg border-b border-zinc-700">
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-zinc-400">{{ codeBlocks[parseInt(part)].lang }}</span>
                    <template v-if="codeBlocks[parseInt(part)].isHtml">
                      <button
                        @click="togglePreview(parseInt(part))"
                        class="px-2 py-0.5 text-xs rounded touch-manipulation"
                        :class="codeBlocks[parseInt(part)].showPreview ? 'bg-btn-primary text-white' : 'bg-zinc-700 text-zinc-300'"
                      >
                        预览
                      </button>
                      <button
                        @click="togglePreview(parseInt(part))"
                        class="px-2 py-0.5 text-xs rounded touch-manipulation"
                        :class="!codeBlocks[parseInt(part)].showPreview ? 'bg-btn-primary text-white' : 'bg-zinc-700 text-zinc-300'"
                      >
                        代码
                      </button>
                    </template>
                  </div>
                  <button
                    @click="copyCode(codeBlocks[parseInt(part)].code)"
                    class="flex items-center gap-1 px-2 py-1 text-xs text-zinc-400 active:text-zinc-200 active:bg-zinc-700 rounded transition-colors touch-manipulation"
                  >
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                    </svg>
                    复制
                  </button>
                </div>
                <template v-if="codeBlocks[parseInt(part)].isHtml && codeBlocks[parseInt(part)].showPreview">
                  <div class="html-preview bg-zinc-900 rounded-b-lg overflow-hidden">
                    <div class="flex items-center justify-between py-2 px-3 bg-zinc-800/50 border-b border-zinc-700">
                      <div class="flex items-center justify-center gap-2 flex-1">
                        <button
                          @click="setPreviewMode(parseInt(part), 'desktop')"
                          class="p-1.5 rounded transition-colors touch-manipulation"
                          :class="codeBlocks[parseInt(part)].previewMode === 'desktop' ? 'bg-btn-primary text-white' : 'text-zinc-400 active:text-zinc-200 active:bg-zinc-700'"
                        >
                          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                          </svg>
                        </button>
                        <button
                          @click="setPreviewMode(parseInt(part), 'mobile')"
                          class="p-1.5 rounded transition-colors touch-manipulation"
                          :class="codeBlocks[parseInt(part)].previewMode === 'mobile' ? 'bg-btn-primary text-white' : 'text-zinc-400 active:text-zinc-200 active:bg-zinc-700'"
                        >
                          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
                          </svg>
                        </button>
                      </div>
                      <button
                        @click="openFullscreen(parseInt(part))"
                        class="p-1.5 rounded transition-colors text-zinc-400 active:text-zinc-200 active:bg-zinc-700 touch-manipulation"
                      >
                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
                        </svg>
                      </button>
                    </div>
                    <div class="flex justify-center p-3">
                      <div
                        class="bg-white overflow-hidden"
                        :class="codeBlocks[parseInt(part)].previewMode === 'mobile' ? 'rounded-2xl border-4 border-zinc-600' : 'rounded-lg'"
                        :style="codeBlocks[parseInt(part)].previewMode === 'mobile' ? 'width: 50%; aspect-ratio: 9/20;' : 'width: 100%; aspect-ratio: 16/9;'"
                      >
                        <iframe
                          :srcdoc="codeBlocks[parseInt(part)].code"
                          class="w-full h-full border-0"
                          sandbox="allow-scripts"
                        ></iframe>
                      </div>
                    </div>
                  </div>
                </template>
                <template v-else>
                  <pre class="code-pre"><code v-html="codeBlocks[parseInt(part)].highlighted"></code></pre>
                </template>
              </div>
            </template>
          </template>
        </div>
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
    <Teleport to="body">
      <div
        v-if="fullscreenIndex !== null && codeBlocks[fullscreenIndex]"
        class="fixed inset-0 z-[10000] bg-black flex flex-col safe-area-inset"
      >
        <div class="flex items-center justify-between px-4 py-3 bg-zinc-900 border-b border-zinc-800 safe-area-top">
          <div class="flex items-center gap-2">
            <button
              @click="setPreviewMode(fullscreenIndex, 'desktop')"
              class="p-2 rounded transition-colors touch-manipulation"
              :class="codeBlocks[fullscreenIndex].previewMode === 'desktop' ? 'bg-btn-primary text-white' : 'text-zinc-400 active:text-zinc-200 active:bg-zinc-700'"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
            </button>
            <button
              @click="setPreviewMode(fullscreenIndex, 'mobile')"
              class="p-2 rounded transition-colors touch-manipulation"
              :class="codeBlocks[fullscreenIndex].previewMode === 'mobile' ? 'bg-btn-primary text-white' : 'text-zinc-400 active:text-zinc-200 active:bg-zinc-700'"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
              </svg>
            </button>
          </div>
          <button
            @click="closeFullscreen"
            class="p-2 rounded-lg text-zinc-400 active:text-zinc-200 active:bg-zinc-700 transition-colors touch-manipulation"
          >
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="flex-1 flex items-center justify-center p-4 safe-area-bottom">
          <div
            class="bg-white overflow-hidden"
            :class="codeBlocks[fullscreenIndex].previewMode === 'mobile' ? 'rounded-3xl border-8 border-zinc-700' : 'rounded-lg'"
            :style="codeBlocks[fullscreenIndex].previewMode === 'mobile' ? 'height: 90%; aspect-ratio: 9/20;' : 'width: 95%; aspect-ratio: 16/9;'"
          >
            <iframe
              :srcdoc="codeBlocks[fullscreenIndex].code"
              class="w-full h-full border-0"
              sandbox="allow-scripts"
            ></iframe>
          </div>
        </div>
      </div>
    </Teleport>
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
.code-block-wrapper .code-pre {
  margin-top: 0 !important;
  border-top-left-radius: 0 !important;
  border-top-right-radius: 0 !important;
  background-color: #1e1e1e;
  border-radius: 0 0 0.5rem 0.5rem;
  padding: 0.75rem;
  overflow-x: auto;
  margin: 0;
}
.code-block-wrapper .code-pre code {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 0.8rem;
}
.code-block-wrapper .code-pre :deep(.hljs-keyword) {
  color: #c586c0;
}
.code-block-wrapper .code-pre :deep(.hljs-string) {
  color: #ce9178;
}
.code-block-wrapper .code-pre :deep(.hljs-number) {
  color: #b5cea8;
}
.code-block-wrapper .code-pre :deep(.hljs-function) {
  color: #dcdcaa;
}
.code-block-wrapper .code-pre :deep(.hljs-title) {
  color: #dcdcaa;
}
.code-block-wrapper .code-pre :deep(.hljs-params) {
  color: #9cdcfe;
}
.code-block-wrapper .code-pre :deep(.hljs-comment) {
  color: #6a9955;
}
.code-block-wrapper .code-pre :deep(.hljs-tag) {
  color: #569cd6;
}
.code-block-wrapper .code-pre :deep(.hljs-attr) {
  color: #9cdcfe;
}
.code-block-wrapper .code-pre :deep(.hljs-attribute) {
  color: #9cdcfe;
}
.code-block-wrapper .code-pre :deep(.hljs-name) {
  color: #569cd6;
}
.code-block-wrapper .code-pre :deep(.hljs-variable) {
  color: #9cdcfe;
}
.code-block-wrapper .code-pre :deep(.hljs-property) {
  color: #9cdcfe;
}
.code-block-wrapper .code-pre :deep(.hljs-operator) {
  color: #d4d4d4;
}
.code-block-wrapper .code-pre :deep(.hljs-punctuation) {
  color: #d4d4d4;
}
.code-block-wrapper .code-pre :deep(.hljs-built_in) {
  color: #4ec9b0;
}
.code-block-wrapper .code-pre :deep(.hljs-type) {
  color: #4ec9b0;
}
.code-block-wrapper .code-pre :deep(.hljs-class) {
  color: #4ec9b0;
}
.code-block-wrapper .code-pre :deep(.hljs-literal) {
  color: #569cd6;
}
.code-block-wrapper .code-pre :deep(.hljs-meta) {
  color: #569cd6;
}
.code-block-wrapper .code-pre :deep(.hljs-selector-tag) {
  color: #d7ba7d;
}
.code-block-wrapper .code-pre :deep(.hljs-selector-class) {
  color: #d7ba7d;
}
.code-block-wrapper .code-pre :deep(.hljs-selector-id) {
  color: #d7ba7d;
}
</style>
