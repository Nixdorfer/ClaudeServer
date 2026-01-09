<script setup lang="ts">
defineProps<{
  activeTab: 'list' | 'chat' | 'settings'
}>()
const emit = defineEmits<{
  change: [tab: 'list' | 'chat' | 'settings']
}>()
</script>

<template>
  <div class="flex items-center justify-around bg-sidebar-bg border-t border-zinc-800 safe-area-bottom">
    <button
      class="flex-1 flex flex-col items-center py-3 touch-manipulation"
      :class="activeTab === 'list' ? 'text-btn-primary' : 'text-zinc-500'"
      @click="emit('change', 'list')"
    >
      <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 6h16M4 12h16M4 18h16" />
      </svg>
      <span class="text-xs mt-1">列表</span>
    </button>
    <button
      class="flex-1 flex flex-col items-center py-3 touch-manipulation"
      :class="activeTab === 'chat' ? 'text-btn-primary' : 'text-zinc-500'"
      @click="emit('change', 'chat')"
    >
      <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
      </svg>
      <span class="text-xs mt-1">对话</span>
    </button>
    <button
      class="flex-1 flex flex-col items-center py-3 touch-manipulation"
      :class="activeTab === 'settings' ? 'text-btn-primary' : 'text-zinc-500'"
      @click="emit('change', 'settings')"
    >
      <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      </svg>
      <span class="text-xs mt-1">设置</span>
    </button>
  </div>
</template>
