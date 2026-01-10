<script setup lang="ts">
defineProps<{
  title: string
  isConnected: boolean
  isConnecting: boolean
  showMenu: boolean
}>()
</script>

<template>
  <header class="h-16 bg-sidebar-bg border-b border-zinc-800 flex items-center px-4 flex-shrink-0 safe-area-top">
    <div class="w-10"></div>
    <h1 class="flex-1 text-center text-zinc-200 font-medium truncate px-4">{{ title }}</h1>
    <span
      class="w-2.5 h-2.5 rounded-full flex-shrink-0"
      :class="[
        isConnected ? 'bg-green-500' :
        isConnecting ? 'bg-blue-500 animate-pulse' :
        'bg-red-500'
      ]"
    ></span>
  </header>
</template>
