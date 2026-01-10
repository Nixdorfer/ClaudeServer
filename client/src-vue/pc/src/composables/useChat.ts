import { ref, computed } from 'vue'
import { Events } from '@wailsio/runtime'
// @ts-ignore
import * as ChatService from '../../bindings/claudechat/services/chatservice.js'
// @ts-ignore
import * as UsageService from '../../bindings/claudechat/services/usageservice.js'
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
const isConnecting = ref(false)
const isLoading = ref(false)
const streamingContent = ref('')
const error = ref<string | null>(null)
const usageStatus = ref<UsageStatus | null>(null)
const usageBlocked = ref(false)
const usageBlockMessage = ref('')
const serverUnavailable = ref(false)
const reconnectAttempts = ref(0)
const maxReconnectAttempts = 5
const reconnectDelay = 3000
const unlisteners: (() => void)[] = []
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
export function useChat() {
  const currentConversation = computed(() =>
    conversations.value.find(c => c.conversation_id === currentConversationId.value)
  )
  const hasMessages = computed(() => messages.value.length > 0)
  function generateId(): string {
    return Date.now().toString(36) + Math.random().toString(36).substr(2)
  }
  function formatRelativeTime(isoTime: string): string {
    try {
      const resetDate = new Date(isoTime)
      const now = new Date()
      const diffMs = resetDate.getTime() - now.getTime()
      if (diffMs <= 0) {
        return '即将重置用量'
      }
      const diffMinutes = Math.ceil(diffMs / (1000 * 60))
      const diffHours = Math.floor(diffMinutes / 60)
      const remainingMinutes = diffMinutes % 60
      if (diffHours >= 1) {
        if (remainingMinutes > 0) {
          return `${diffHours}小时${remainingMinutes}分钟后重置用量`
        }
        return `${diffHours}小时后重置用量`
      }
      return `${diffMinutes}分钟后重置用量`
    } catch {
      return '稀后重置用量'
    }
  }
  function clearReconnectTimer() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }
  async function attemptAutoReconnect() {
    if (serverUnavailable.value) return
    if (reconnectAttempts.value >= maxReconnectAttempts) {
      console.log('已达最大重连次数')
      return
    }
    reconnectAttempts.value++
    console.log(`自动重连尝试 ${reconnectAttempts.value}/${maxReconnectAttempts}`)
    isConnecting.value = true
    try {
      await ChatService.Connect()
    } catch (e) {
      const errStr = String(e)
      console.error('自动重连失败:', e)
      isConnecting.value = false
      if (errStr.includes('502') || errStr.includes('Bad Gateway')) {
        serverUnavailable.value = true
        return
      }
      if (reconnectAttempts.value < maxReconnectAttempts) {
        reconnectTimer = setTimeout(() => {
          attemptAutoReconnect()
        }, reconnectDelay)
      }
    }
  }
  async function fetchUsageStatus() {
    try {
      const status = await UsageService.GetUsageStatus() as unknown as UsageStatus
      usageStatus.value = status
      serverUnavailable.value = false
      if (status.is_blocked) {
        usageBlocked.value = true
        const relativeTime = formatRelativeTime(status.block_reset_time)
        usageBlockMessage.value = `${status.block_reason}\n${relativeTime}`
      } else {
        usageBlocked.value = false
        usageBlockMessage.value = ''
      }
    } catch (e) {
      const errStr = String(e)
      console.error('获取用量状态失败:', e)
      if (errStr.includes('502') || errStr.includes('Bad Gateway') || errStr.includes('decoding') || errStr.includes('expected value')) {
        serverUnavailable.value = true
      }
    }
  }
  async function initialize() {
    unlisteners.push(Events.On('connected', () => {
      isConnected.value = true
      isConnecting.value = false
      error.value = null
      reconnectAttempts.value = 0
      clearReconnectTimer()
      fetchUsageStatus()
    }))
    unlisteners.push(Events.On('disconnected', () => {
      isConnected.value = false
      isConnecting.value = false
      reconnectTimer = setTimeout(() => {
        attemptAutoReconnect()
      }, reconnectDelay)
    }))
    unlisteners.push(Events.On('connection_error', (event: any) => {
      error.value = event?.data || event
      isConnected.value = false
      isConnecting.value = false
      reconnectTimer = setTimeout(() => {
        attemptAutoReconnect()
      }, reconnectDelay)
    }))
    unlisteners.push(Events.On('conversation_id', async (event: any) => {
      const data = (event?.data || event) as WSConversationData
      if (data.conversation_id) {
        const oldId = currentConversationId.value
        currentConversationId.value = data.conversation_id
        if (!oldId || oldId.startsWith('local_')) {
          try {
            const firstMsg = pendingUserMessage || ''
            await ChatService.CreateLocalConversation(data.conversation_id, firstMsg)
            if (pendingUserMessage) {
              await ChatService.SaveLocalMessage(data.conversation_id, 'user', pendingUserMessage)
              pendingUserMessage = null
            }
          } catch (e) {
            console.error('创建对话失败:', e)
          }
        }
      }
    }))
    unlisteners.push(Events.On('content', (event: any) => {
      const data = (event?.data || event) as WSContentData
      streamingContent.value = data.text || data.delta || ''
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        lastMsg.content = streamingContent.value
      }
    }))
    unlisteners.push(Events.On('done', async (event: any) => {
      const data = (event?.data || event) as WSDoneData
      isLoading.value = false
      streamingContent.value = ''
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant') {
        lastMsg.content = data.response
        lastMsg.isStreaming = false
        if (currentConversationId.value) {
          try {
            await ChatService.SaveLocalMessage(currentConversationId.value, 'assistant', data.response)
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
    }))
    unlisteners.push(Events.On('ws_error', (event: any) => {
      const data = (event?.data || event) as WSErrorData
      error.value = data.error
      isLoading.value = false
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        messages.value.pop()
      }
    }))
    unlisteners.push(Events.On('usage_blocked', (event: any) => {
      const data = (event?.data || event) as { block_reason: string; block_reset_time: string }
      isLoading.value = false
      usageBlocked.value = true
      const relativeTime = formatRelativeTime(data.block_reset_time)
      usageBlockMessage.value = `${data.block_reason}\n${relativeTime}`
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        messages.value.pop()
      }
      const userMsg = messages.value[messages.value.length - 1]
      if (userMsg && userMsg.role === 'user') {
        messages.value.pop()
      }
    }))
    unlisteners.push(Events.On('device_banned', () => {
      isLoading.value = false
    }))
    unlisteners.push(Events.On('version_outdated', () => {
      isLoading.value = false
    }))
    await loadDialogues()
    try {
      isConnected.value = await ChatService.IsConnected()
      if (!isConnected.value) {
        isConnecting.value = true
        await ChatService.Connect()
      }
    } catch (e) {
      const errStr = String(e)
      console.error('Connection check failed:', e)
      isConnecting.value = false
      if (errStr.includes('502') || errStr.includes('Bad Gateway') || errStr.includes('连接失败')) {
        serverUnavailable.value = true
      }
    }
    await fetchUsageStatus()
  }
  function cleanup() {
    clearReconnectTimer()
    unlisteners.forEach(unlisten => unlisten())
    unlisteners.length = 0
  }
  async function loadDialogues() {
    try {
      const localConvs = await ChatService.GetLocalConversations() as unknown as LocalConversation[]
      conversations.value = (localConvs || []).map(conv => ({
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
      const localMsgs = await ChatService.GetLocalMessages(conversationId) as unknown as LocalMessage[]
      messages.value = (localMsgs || []).map(msg => ({
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
        currentConversationId.value = await ChatService.GenerateConversationId()
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
        await ChatService.SaveLocalMessage(currentConversationId.value, 'user', content.trim())
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
      await ChatService.SendMessage(serverConvId, content.trim())
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
    clearReconnectTimer()
    reconnectAttempts.value = 0
    try {
      isConnecting.value = true
      await ChatService.Connect()
    } catch (e) {
      error.value = String(e)
      isConnecting.value = false
    }
  }
  function clearError() {
    error.value = null
  }
  async function reportError(errorMessage: string, conversationId: string): Promise<boolean> {
    try {
      await ChatService.ReportError(errorMessage, conversationId)
      error.value = null
      return true
    } catch (e) {
      console.error('发送错误报告失败:', e)
      return false
    }
  }
  async function renameConversation(id: string, name: string) {
    try {
      await ChatService.RenameConversation(id, name)
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
      await ChatService.DeleteLocalConversation(id)
      conversations.value = conversations.value.filter(c => c.conversation_id !== id)
      if (currentConversationId.value === id) {
        newConversation()
      }
    } catch (e) {
      console.error('删除对话失败:', e)
    }
  }
  async function updateDeviceNotice(notice: string): Promise<boolean> {
    try {
      await ChatService.UpdateDeviceNotice(notice)
      return true
    } catch (e) {
      console.error('更新设备备注失败:', e)
      return false
    }
  }
  return {
    conversations,
    currentConversationId,
    currentConversation,
    messages,
    isConnected,
    isConnecting,
    isLoading,
    streamingContent,
    error,
    hasMessages,
    usageStatus,
    usageBlocked,
    usageBlockMessage,
    serverUnavailable,
    reconnectAttempts,
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
    fetchUsageStatus,
    reportError,
    updateDeviceNotice
  }
}
