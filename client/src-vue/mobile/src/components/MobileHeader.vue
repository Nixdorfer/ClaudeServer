<script setup lang="ts">
defineProps<{
  title: string
  isConnected: boolean
}>()
const emit = defineEmits<{
  openDrawer: []
}>()
</script>

<template>
  <header class="h-14 bg-sidebar-bg border-b border-zinc-800 flex items-center px-4 flex-shrink-0 safe-area-top">
    <button @click="emit('openDrawer')" class="p-2 -ml-2 touch-manipulation">
      <svg class="w-6 h-6 text-zinc-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
      </svg>
    </button>
    <h1 class="flex-1 text-center text-zinc-200 font-medium truncate px-4">{{ title }}</h1>
    <span class="w-2.5 h-2.5 rounded-full flex-shrink-0" :class="isConnected ? 'bg-green-500' : 'bg-red-500'"></span>
  </header>
</template>
