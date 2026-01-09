<script setup lang="ts">
defineProps<{
  show: boolean
  title: string
  confirmText?: string
  cancelText?: string
  showCancel?: boolean
  type?: 'info' | 'warning' | 'error'
}>()

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="fixed inset-0 z-50 flex items-center justify-center"
        @click.self="emit('cancel')"
      >
        <div class="absolute inset-0 bg-black/60 backdrop-blur-sm"></div>

        <div class="relative bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl max-w-md w-full mx-4 animate-fadeIn">
          <div class="p-5 border-b border-zinc-800">
            <div class="flex items-center gap-3">
              <div
                v-if="type === 'warning'"
                class="w-10 h-10 rounded-full bg-yellow-500/20 flex items-center justify-center"
              >
                <svg class="w-5 h-5 text-yellow-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div
                v-else-if="type === 'error'"
                class="w-10 h-10 rounded-full bg-red-500/20 flex items-center justify-center"
              >
                <svg class="w-5 h-5 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div
                v-else
                class="w-10 h-10 rounded-full bg-btn-primary/20 flex items-center justify-center"
              >
                <svg class="w-5 h-5 text-btn-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <h3 class="text-lg font-semibold text-white">{{ title }}</h3>
            </div>
          </div>

          <div class="p-5">
            <slot></slot>
          </div>

          <div class="p-4 border-t border-zinc-800 flex justify-end gap-3">
            <button
              v-if="showCancel !== false"
              @click="emit('cancel')"
              class="px-4 py-2 text-sm text-zinc-400 hover:text-white hover:bg-zinc-800 rounded-lg transition-colors"
            >
              {{ cancelText || '取消' }}
            </button>
            <button
              @click="emit('confirm')"
              class="px-4 py-2 text-sm bg-btn-primary hover:bg-btn-hover text-white rounded-lg transition-colors"
            >
              {{ confirmText || '确定' }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.animate-fadeIn {
  animation: fadeIn 0.2s ease-out;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: scale(0.95) translateY(-10px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}
</style>
