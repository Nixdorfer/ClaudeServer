import { ref, computed } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  SendMessage,
  Connect,
  IsConnected,
  GetLocalConversations,
  GetLocalMessages,
  SaveLocalMessage,
  UpdateLocalMessage,
  RenameConversation,
  DeleteLocalConversation,
  GenerateConversationID,
  CreateLocalConversation,
  GetUsageStatus,
  FormatResetTime
} from '../../wailsjs/go/main/App'
import type { Message, Conversation, WSContentData, WSConversationData, WSDoneData, WSErrorData, UsageStatus } from '../types'

interface LocalMessage {
  id: number
  conversation_id: string
  role: string
  content: string
  timestamp: string
}

interface LocalConversation {
  conversation_id: string
  name: string
  first_message: string
  message_count: number
  last_used_time: string
  is_generating: boolean
}

const conversations = ref<Conversation[]>([])
const currentConversationId = ref<string>('')
const messages = ref<Message[]>([])
const isConnected = ref(false)
const isLoading = ref(false)
const streamingContent = ref('')
const error = ref<string | null>(null)
const usageStatus = ref<UsageStatus | null>(null)
const usageBlocked = ref(false)
const usageBlockMessage = ref('')

export function useChat() {
  const currentConversation = computed(() =>
    conversations.value.find(c => c.conversation_id === currentConversationId.value)
  )

  const hasMessages = computed(() => messages.value.length > 0)

  function generateId(): string {
    return Date.now().toString(36) + Math.random().toString(36).substr(2)
  }

  async function fetchUsageStatus() {
    try {
      const status = await GetUsageStatus() as UsageStatus
      usageStatus.value = status

      if (status.is_blocked) {
        usageBlocked.value = true
        const resetTime = await FormatResetTime(status.block_reset_time)
        usageBlockMessage.value = `${status.block_reason}\n${resetTime} 重置用量`
      } else {
        usageBlocked.value = false
        usageBlockMessage.value = ''
      }
    } catch (e) {
      console.error('获取用量状态失败:', e)
    }
  }

  async function initialize() {
    EventsOn('connected', () => {
      isConnected.value = true
      error.value = null
      fetchUsageStatus()
    })

    EventsOn('disconnected', () => {
      isConnected.value = false
    })

    EventsOn('connection_error', (err: string) => {
      error.value = err
      isConnected.value = false
    })

    EventsOn('conversation_id', async (data: WSConversationData) => {
      if (data.conversation_id) {
        const oldId = currentConversationId.value
        currentConversationId.value = data.conversation_id

        if (!oldId || oldId.startsWith('local_')) {
          try {
            const firstMsg = pendingUserMessage || ''
            await CreateLocalConversation(data.conversation_id, firstMsg)

            if (pendingUserMessage) {
              await SaveLocalMessage(data.conversation_id, 'user', pendingUserMessage)
              pendingUserMessage = null
            }
          } catch (e) {
            console.error('创建对话失败:', e)
          }
        }
      }
    })

    EventsOn('content', (data: WSContentData) => {
      streamingContent.value = data.text || data.delta || ''

      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        lastMsg.content = streamingContent.value
      }
    })

    EventsOn('done', async (data: WSDoneData) => {
      isLoading.value = false
      streamingContent.value = ''

      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant') {
        lastMsg.content = data.response
        lastMsg.isStreaming = false

        if (currentConversationId.value) {
          try {
            await SaveLocalMessage(currentConversationId.value, 'assistant', data.response)
          } catch (e) {
            console.error('保存助手消息失败:', e)
          }
        }
      }

      if (data.conversation_id && !currentConversationId.value) {
        currentConversationId.value = data.conversation_id
      }

      await loadDialogues()
      await fetchUsageStatus()
    })

    EventsOn('ws_error', (data: WSErrorData) => {
      error.value = data.error
      isLoading.value = false

      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        messages.value.pop()
      }
    })

    await loadDialogues()

    try {
      isConnected.value = await IsConnected()
      if (!isConnected.value) {
        await Connect()
      }
    } catch (e) {
      console.error('Connection check failed:', e)
    }

    await fetchUsageStatus()
  }

  function cleanup() {
    EventsOff('connected')
    EventsOff('disconnected')
    EventsOff('connection_error')
    EventsOff('conversation_id')
    EventsOff('content')
    EventsOff('done')
    EventsOff('ws_error')
  }

  async function loadDialogues() {
    try {
      const localConvs = await GetLocalConversations() as LocalConversation[]
      conversations.value = localConvs.map(conv => ({
        conversation_id: conv.conversation_id,
        name: conv.name,
        first_message: conv.first_message,
        message_count: conv.message_count,
        last_used_time: conv.last_used_time,
        is_generating: conv.is_generating || false
      }))
    } catch (e) {
      console.error('Failed to load local dialogues:', e)
    }
  }

  async function loadHistory(conversationId: string) {
    try {
      const localMsgs = await GetLocalMessages(conversationId) as LocalMessage[]
      messages.value = localMsgs.map(msg => ({
        id: String(msg.id),
        role: msg.role as 'user' | 'assistant',
        content: msg.content,
        timestamp: new Date(msg.timestamp)
      }))
    } catch (e) {
      console.error('Failed to load local history:', e)
    }
  }

  let pendingUserMessage: string | null = null

  async function sendMessage(content: string) {
    if (!content.trim() || isLoading.value) return

    await fetchUsageStatus()
    if (usageBlocked.value) {
      error.value = usageBlockMessage.value
      return
    }

    error.value = null

    const isNewConversation = !currentConversationId.value || currentConversationId.value.startsWith('local_')

    if (isNewConversation) {
      pendingUserMessage = content.trim()
      if (!currentConversationId.value) {
        currentConversationId.value = await GenerateConversationID()
      }
    }

    const userMessage: Message = {
      id: generateId(),
      role: 'user',
      content: content.trim(),
      timestamp: new Date()
    }
    messages.value.push(userMessage)

    if (!isNewConversation) {
      try {
        await SaveLocalMessage(currentConversationId.value, 'user', content.trim())
      } catch (e) {
        console.error('保存用户消息失败:', e)
      }
    }

    const assistantMessage: Message = {
      id: generateId(),
      role: 'assistant',
      content: '',
      timestamp: new Date(),
      isStreaming: true
    }
    messages.value.push(assistantMessage)

    isLoading.value = true
    streamingContent.value = ''

    try {
      const serverConvId = isNewConversation ? '' : currentConversationId.value
      await SendMessage(serverConvId, content.trim())
    } catch (e) {
      error.value = String(e)
      isLoading.value = false
      messages.value.pop()
      pendingUserMessage = null
    }
  }

  async function selectConversation(conversationId: string) {
    if (conversationId === currentConversationId.value) return

    currentConversationId.value = conversationId
    messages.value = []

    if (conversationId) {
      await loadHistory(conversationId)
    }
  }

  function newConversation() {
    currentConversationId.value = ''
    messages.value = []
    error.value = null
  }

  async function reconnect() {
    try {
      await Connect()
    } catch (e) {
      error.value = String(e)
    }
  }

  function clearError() {
    error.value = null
  }

  async function renameConversation(id: string, name: string) {
    try {
      await RenameConversation(id, name)
      const conv = conversations.value.find(c => c.conversation_id === id)
      if (conv) {
        conv.name = name
      }
    } catch (e) {
      console.error('重命名对话失败:', e)
    }
  }

  async function deleteConversation(id: string) {
    try {
      await DeleteLocalConversation(id)
      conversations.value = conversations.value.filter(c => c.conversation_id !== id)
      if (currentConversationId.value === id) {
        newConversation()
      }
    } catch (e) {
      console.error('删除对话失败:', e)
    }
  }

  return {
    conversations,
    currentConversationId,
    currentConversation,
    messages,
    isConnected,
    isLoading,
    streamingContent,
    error,
    hasMessages,
    usageStatus,
    usageBlocked,
    usageBlockMessage,
    initialize,
    cleanup,
    sendMessage,
    selectConversation,
    newConversation,
    reconnect,
    clearError,
    loadDialogues,
    renameConversation,
    deleteConversation,
    fetchUsageStatus
  }
}
