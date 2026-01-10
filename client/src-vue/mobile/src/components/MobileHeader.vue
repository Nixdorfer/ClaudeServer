<script setup lang="ts">
defineProps<{
  title: string
  isConnected: boolean
  isConnecting: boolean
  offlineMode: boolean
  showMenu: boolean
}>()
</script>

<template>
  <header class="h-16 bg-sidebar-bg border-b border-zinc-800 flex items-center px-4 flex-shrink-0 safe-area-top">
    <div class="w-10"></div>
    <h1 class="flex-1 text-center text-zinc-200 font-medium truncate px-4">{{ title }}</h1>
    <div v-if="offlineMode" class="flex items-center gap-1.5 flex-shrink-0">
      <span class="w-2.5 h-2.5 rounded-full bg-zinc-500"></span>
      <span class="text-xs text-zinc-500">离线</span>
    </div>
    <span
      v-else
      class="w-2.5 h-2.5 rounded-full flex-shrink-0"
      :class="[
        isConnected ? 'bg-green-500' :
        isConnecting ? 'bg-blue-500 animate-pulse' :
        'bg-red-500'
      ]"
    ></span>
  </header>
</template>
