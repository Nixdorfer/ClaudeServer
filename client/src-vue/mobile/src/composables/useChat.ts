import { ref, computed } from 'vue'
import type { Message, Conversation, UsageStatus, UpdateCheckResult, VersionInfo } from '../types'

declare const __APP_VERSION__: string
const WS_URL = 'wss://claude.nixdorfer.com/data/websocket/create'
const API_URL = 'https://claude.nixdorfer.com/api/device/status'
const USAGE_URL = 'https://claude.nixdorfer.com/api/usage'
const UPDATE_URL = 'https://raw.githubusercontent.com/Nixdorfer/ClaudeServer/main/info.json'
const CURRENT_VERSION = __APP_VERSION__
const STORAGE_KEY_CONVERSATIONS = 'claude_conversations'
const STORAGE_KEY_MESSAGES = 'claude_messages_'
const STORAGE_KEY_DEVICE_ID = 'claude_device_id'

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
const isBanned = ref(false)
const bannedReason = ref('')
const serverUnavailable = ref(false)
const versionOutdated = ref(false)
const versionOutdatedMessage = ref('')
const offlineMode = ref(false)
const updateInfo = ref<UpdateCheckResult | null>(null)
const reconnectAttempts = ref(0)
const maxReconnectAttempts = 5

let ws: WebSocket | null = null
let pendingUserMessage: string | null = null
let reconnectTimer: number | null = null

function getDeviceId(): string {
  let deviceId = localStorage.getItem(STORAGE_KEY_DEVICE_ID)
  if (!deviceId) {
    deviceId = 'web_' + Date.now().toString(36) + Math.random().toString(36).substr(2, 9)
    localStorage.setItem(STORAGE_KEY_DEVICE_ID, deviceId)
  }
  return deviceId
}

function enterOfflineMode() {
  offlineMode.value = true
  isConnected.value = false
  isConnecting.value = false
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws) {
    ws.close()
    ws = null
  }
}
export function useChat() {
  const currentConversation = computed(() =>
    conversations.value.find(c => c.conversation_id === currentConversationId.value)
  )
  const hasMessages = computed(() => messages.value.length > 0)
  function generateId(): string {
    return Date.now().toString(36) + Math.random().toString(36).substr(2)
  }
  function saveConversations() {
    localStorage.setItem(STORAGE_KEY_CONVERSATIONS, JSON.stringify(conversations.value))
  }
  function loadConversationsFromStorage() {
    const data = localStorage.getItem(STORAGE_KEY_CONVERSATIONS)
    if (data) {
      try {
        conversations.value = JSON.parse(data)
      } catch (e) {
        conversations.value = []
      }
    }
  }
  function saveMessages(convId: string) {
    const msgs = messages.value.filter(m => !m.isStreaming)
    localStorage.setItem(STORAGE_KEY_MESSAGES + convId, JSON.stringify(msgs))
  }
  function loadMessagesFromStorage(convId: string) {
    const data = localStorage.getItem(STORAGE_KEY_MESSAGES + convId)
    if (data) {
      try {
        messages.value = JSON.parse(data).map((m: Message) => ({
          ...m,
          timestamp: new Date(m.timestamp)
        }))
      } catch (e) {
        messages.value = []
      }
    }
  }
  function connect() {
    console.log('[WS] connect() called, ws:', ws?.readyState, 'serverUnavailable:', serverUnavailable.value, 'offlineMode:', offlineMode.value)
    if (offlineMode.value) {
      console.log('[WS] Offline mode, skipping connection')
      return
    }
    if (ws && ws.readyState === WebSocket.OPEN) {
      console.log('[WS] Already connected, skipping')
      return
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (serverUnavailable.value) {
      console.log('[WS] Server unavailable, skipping connection')
      return
    }
    isConnecting.value = true
    const deviceId = getDeviceId()
    const url = `${WS_URL}?device_id=${deviceId}&platform=android`
    console.log('[WS] Connecting to:', url)
    console.log('[WS] Device ID:', deviceId)
    try {
      ws = new WebSocket(url)
    } catch (e) {
      console.error('[WS] WebSocket creation failed:', e)
      isConnecting.value = false
      return
    }
    ws.onopen = () => {
      console.log('[WS] Connection opened')
      isConnected.value = true
      isConnecting.value = false
      error.value = null
      reconnectAttempts.value = 0
    }
    ws.onclose = (event) => {
      console.log('[WS] Connection closed, code:', event.code, 'reason:', event.reason, 'wasClean:', event.wasClean)
      isConnected.value = false
      isConnecting.value = false
      if (!offlineMode.value && !isBanned.value && !serverUnavailable.value) {
        reconnectAttempts.value++
        console.log('[WS] Connect attempts:', reconnectAttempts.value)
        if (reconnectAttempts.value >= maxReconnectAttempts) {
          console.log('[WS] Max attempts reached, marking server unavailable')
          serverUnavailable.value = true
        } else {
          console.log('[WS] Will retry in 3 seconds...')
          reconnectTimer = window.setTimeout(() => {
            connect()
          }, 3000)
        }
      }
    }
    ws.onerror = (event) => {
      console.error('[WS] Connection error:', event)
      isConnected.value = false
      isConnecting.value = false
      if (!offlineMode.value) {
        reconnectAttempts.value++
        if (reconnectAttempts.value >= maxReconnectAttempts) {
          serverUnavailable.value = true
        }
      }
    }
    ws.onmessage = (event) => {
      console.log('[WS] Message received:', event.data.substring(0, 200))
      try {
        handleMessage(JSON.parse(event.data))
      } catch (e) {
        console.error('[WS] Parse message error:', e)
      }
    }
  }
  function handleMessage(msg: { type: string; data?: unknown }) {
    const data = msg.data as Record<string, unknown> | undefined
    if (msg.type === 'conversation_id' && data) {
      const oldId = currentConversationId.value
      currentConversationId.value = data.conversation_id as string
      if (!oldId || oldId.startsWith('local_')) {
        createLocalConversation(data.conversation_id as string, pendingUserMessage || '')
        if (pendingUserMessage) {
          saveMessages(data.conversation_id as string)
          pendingUserMessage = null
        }
      }
    } else if (msg.type === 'content' && data) {
      streamingContent.value = (data.text as string) || (data.delta as string) || ''
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        lastMsg.content = streamingContent.value
      }
    } else if (msg.type === 'done' && data) {
      isLoading.value = false
      streamingContent.value = ''
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant') {
        lastMsg.content = data.response as string
        lastMsg.isStreaming = false
        saveMessages(currentConversationId.value)
        updateConversationMeta()
      }
      if (data.dialogue_id) {
        sendAck(data.dialogue_id as number)
      }
      fetchUsageStatus()
    } else if (msg.type === 'error' && data) {
      error.value = (data.error as string) || (data.message as string) || '未知错误'
      isLoading.value = false
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        messages.value.pop()
      }
    } else if (msg.type === 'banned' && data) {
      isBanned.value = true
      bannedReason.value = (data.reason as string) || '您的设备已被封禁'
      isLoading.value = false
      enterOfflineMode()
    } else if (msg.type === 'usage_blocked' && data) {
      usageBlocked.value = true
      const reason = (data.block_reason as string) || ''
      const resetTime = (data.block_reset_time as string) || ''
      usageBlockMessage.value = formatUsageBlockMessage(reason, resetTime)
      isLoading.value = false
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
        messages.value.pop()
      }
      enterOfflineMode()
    } else if (msg.type === 'version_outdated' && data) {
      versionOutdated.value = true
      versionOutdatedMessage.value = (data.message as string) || '当前版本已过时，无法继续使用，请更新到最新版本'
      isLoading.value = false
      enterOfflineMode()
    }
  }
  function sendAck(dialogueId: number) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return
    ws.send(JSON.stringify({
      type: 'ack',
      data: { dialogue_id: dialogueId }
    }))
  }
  function formatUsageBlockMessage(reason: string, resetTime: string): string {
    let message = reason
    if (resetTime) {
      const resetDate = new Date(resetTime)
      const now = new Date()
      const diffMs = resetDate.getTime() - now.getTime()
      if (diffMs > 0) {
        const diffMinutes = Math.ceil(diffMs / (1000 * 60))
        const diffHours = Math.floor(diffMinutes / 60)
        const remainMinutes = diffMinutes % 60
        let timeStr = ''
        if (diffHours > 0) {
          timeStr = remainMinutes > 0 ? `${diffHours}小时${remainMinutes}分钟` : `${diffHours}小时`
        } else {
          timeStr = `${diffMinutes}分钟`
        }
        message += `\n${timeStr}后重置用量`
      }
    }
    return message
  }
  function formatResetTime(isoTime: string): string {
    try {
      const date = new Date(isoTime)
      return date.toLocaleString('zh-CN', {
        month: 'numeric',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      })
    } catch {
      return isoTime
    }
  }
  function createLocalConversation(id: string, firstMessage: string) {
    const existing = conversations.value.find(c => c.conversation_id === id)
    if (!existing) {
      conversations.value.unshift({
        conversation_id: id,
        name: '',
        first_message: firstMessage,
        message_count: 1,
        last_used_time: new Date().toISOString(),
        is_generating: true
      })
      saveConversations()
    }
  }
  function updateConversationMeta() {
    const conv = conversations.value.find(c => c.conversation_id === currentConversationId.value)
    if (conv) {
      conv.message_count = messages.value.filter(m => !m.isStreaming).length
      conv.last_used_time = new Date().toISOString()
      conv.is_generating = false
      saveConversations()
    }
  }
  async function sendMessage(content: string) {
    if (!content.trim() || isLoading.value) return
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      error.value = '未连接到服务器'
      return
    }
    if (usageBlocked.value) {
      error.value = usageBlockMessage.value
      return
    }
    error.value = null
    const isNewConversation = !currentConversationId.value || currentConversationId.value.startsWith('local_')
    if (isNewConversation) {
      pendingUserMessage = content.trim()
      currentConversationId.value = 'local_' + generateId()
    }
    const userMessage: Message = {
      id: generateId(),
      role: 'user',
      content: content.trim(),
      timestamp: new Date()
    }
    messages.value.push(userMessage)
    const assistantMessage: Message = {
      id: generateId(),
      role: 'assistant',
      content: '',
      timestamp: new Date(),
      isStreaming: true
    }
    messages.value.push(assistantMessage)
    isLoading.value = true
    const serverConvId = isNewConversation ? '' : currentConversationId.value
    ws.send(JSON.stringify({
      type: 'dialogue',
      data: {
        request: content.trim(),
        conversation_id: serverConvId,
        device_id: getDeviceId()
      }
    }))
    if (!isNewConversation) {
      saveMessages(currentConversationId.value)
    }
  }
  function selectConversation(conversationId: string) {
    if (conversationId === currentConversationId.value) return
    currentConversationId.value = conversationId
    messages.value = []
    if (conversationId) {
      loadMessagesFromStorage(conversationId)
    }
  }
  function newConversation() {
    currentConversationId.value = ''
    messages.value = []
    error.value = null
  }
  function reconnect() {
    if (ws) {
      ws.close()
    }
    serverUnavailable.value = false
    reconnectAttempts.value = 0
    connect()
  }
  function deleteConversation(id: string) {
    conversations.value = conversations.value.filter(c => c.conversation_id !== id)
    localStorage.removeItem(STORAGE_KEY_MESSAGES + id)
    saveConversations()
    if (currentConversationId.value === id) {
      newConversation()
    }
  }
  function renameConversation(id: string, name: string) {
    const conv = conversations.value.find(c => c.conversation_id === id)
    if (conv) {
      conv.name = name
      saveConversations()
    }
  }
  async function fetchUsageStatus() {
    const deviceId = getDeviceId()
    const url = `${USAGE_URL}?device_id=${deviceId}&platform=android`
    console.log('[API] Fetching usage status:', url)
    try {
      const response = await fetch(url, {
        method: 'GET',
        headers: { 'Accept': 'application/json' }
      })
      console.log('[API] Usage response status:', response.status)
      if (!response.ok) {
        console.log('[API] Usage response not OK')
        usageStatus.value = { five_hour: 0, five_hour_reset: '', seven_day: 0, seven_day_reset: '', seven_day_sonnet: 0, seven_day_sonnet_reset: '', limit_five_hour: 100, limit_seven_day: 100, is_blocked: false, block_reason: '', block_reset_time: '' }
        return
      }
      const data = await response.json()
      console.log('[API] Usage data:', JSON.stringify(data))
      usageStatus.value = {
        five_hour: data.five_hour_utilization || 0,
        five_hour_reset: data.five_hour_resets_at || '',
        seven_day: data.seven_day_utilization || 0,
        seven_day_reset: data.seven_day_resets_at || '',
        seven_day_sonnet: data.seven_day_opus_utilization || 0,
        seven_day_sonnet_reset: data.seven_day_opus_resets_at || '',
        limit_five_hour: 100,
        limit_seven_day: 100,
        is_blocked: data.is_blocked || false,
        block_reason: data.block_reason || '',
        block_reset_time: data.block_reset_time || ''
      }
      if (data.is_blocked) {
        usageBlocked.value = true
        usageBlockMessage.value = formatUsageBlockMessage(data.block_reason || '', data.block_reset_time || '')
      }
    } catch (e) {
      console.error('[API] Failed to fetch usage status:', e)
      usageStatus.value = { five_hour: 0, five_hour_reset: '', seven_day: 0, seven_day_reset: '', seven_day_sonnet: 0, seven_day_sonnet_reset: '', limit_five_hour: 100, limit_seven_day: 100, is_blocked: false, block_reason: '', block_reset_time: '' }
    }
  }
  async function checkDeviceStatusOnStartup() {
    console.log('[API] Checking device status...')
    try {
      const deviceId = getDeviceId()
      const url = `${API_URL}?device_id=${deviceId}&platform=android`
      console.log('[API] Request URL:', url)
      const response = await fetch(url)
      console.log('[API] Response status:', response.status)
      if (!response.ok) {
        console.log('[API] Response not OK, skipping')
        return
      }
      const data = await response.json()
      console.log('[API] Response data:', data)
      if (data.is_banned) {
        console.log('[API] Device is banned')
        isBanned.value = true
        bannedReason.value = (data.ban_reason as string) || '您的设备已被封禁'
        enterOfflineMode()
        return
      }
      if (data.is_blocked) {
        console.log('[API] Device is blocked')
        usageBlocked.value = true
        const reason = (data.block_reason as string) || ''
        const resetTime = (data.block_reset_time as string) || ''
        usageBlockMessage.value = formatUsageBlockMessage(reason, resetTime)
        enterOfflineMode()
      }
    } catch (e) {
      console.error('[API] Failed to check device status:', e)
    }
  }
  function compareVersions(current: string, latest: string): boolean {
    const currentParts = current.split('.').map(Number)
    const latestParts = latest.split('.').map(Number)
    for (let i = 0; i < Math.max(currentParts.length, latestParts.length); i++) {
      const curr = currentParts[i] || 0
      const lat = latestParts[i] || 0
      if (lat > curr) return true
      if (lat < curr) return false
    }
    return false
  }
  async function checkForUpdates(): Promise<UpdateCheckResult | null> {
    try {
      const response = await fetch(UPDATE_URL)
      if (!response.ok) return null
      const versions: VersionInfo[] = await response.json()
      const latest = versions[versions.length - 1]
      if (!latest) return null
      const hasUpdate = compareVersions(CURRENT_VERSION, latest.version)
      const result: UpdateCheckResult = {
        has_update: hasUpdate,
        current_version: CURRENT_VERSION,
        latest_version: latest.version,
        notes: latest.note,
        download_url: latest.url
      }
      updateInfo.value = result
      return result
    } catch (e) {
      console.error('Failed to check for updates:', e)
      return null
    }
  }
  function initialize() {
    console.log('[App] Initializing...')
    console.log('[App] WS_URL:', WS_URL)
    console.log('[App] API_URL:', API_URL)
    loadConversationsFromStorage()
    checkDeviceStatusOnStartup()
    fetchUsageStatus()
    connect()
  }
  function cleanup() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (ws) {
      ws.close()
      ws = null
    }
  }
  function clearError() {
    error.value = null
  }
  async function reportError(errorMessage: string, conversationId: string): Promise<boolean> {
    const apiUrl = 'https://claude.nixdorfer.com/api/error'
    try {
      const response = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          error: errorMessage,
          conversation_id: conversationId,
          device_id: getDeviceId(),
          platform: 'android',
          version: CURRENT_VERSION
        })
      })
      if (response.ok) {
        error.value = null
        return true
      }
      return false
    } catch (e) {
      console.error('Failed to report error:', e)
      return false
    }
  }
  async function updateDeviceNotice(notice: string): Promise<boolean> {
    const apiUrl = 'https://claude.nixdorfer.com/api/device/notice'
    try {
      const response = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          device_id: getDeviceId(),
          notice: notice
        })
      })
      return response.ok
    } catch (e) {
      console.error('Failed to update device notice:', e)
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
    isBanned,
    bannedReason,
    serverUnavailable,
    versionOutdated,
    versionOutdatedMessage,
    offlineMode,
    updateInfo,
    reconnectAttempts,
    initialize,
    cleanup,
    sendMessage,
    selectConversation,
    newConversation,
    reconnect,
    clearError,
    renameConversation,
    deleteConversation,
    checkForUpdates,
    fetchUsageStatus,
    reportError,
    updateDeviceNotice,
    currentVersion: CURRENT_VERSION,
    wsEndpoint: WS_URL
  }
}
