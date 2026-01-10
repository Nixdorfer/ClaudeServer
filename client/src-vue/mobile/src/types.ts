export interface Message {
  id: string
  role: 'user' | 'assistant'
  content: string
  timestamp: Date
  isStreaming?: boolean
}

export interface Conversation {
  conversation_id: string
  name: string
  first_message: string
  message_count: number
  last_used_time: string
  is_generating: boolean
}

export interface UsageStatus {
  five_hour: number
  five_hour_reset: string
  seven_day: number
  seven_day_reset: string
  seven_day_sonnet: number
  seven_day_sonnet_reset: string
  limit_five_hour: number
  limit_seven_day: number
  is_blocked: boolean
  block_reason: string
  block_reset_time: string
}

export interface WSContentData {
  text?: string
  delta?: string
}

export interface WSConversationData {
  conversation_id: string
}

export interface WSDoneData {
  response: string
  conversation_id?: string
}

export interface WSErrorData {
  error: string
  message?: string
}

export interface UpdateCheckResult {
  has_update: boolean
  current_version: string
  latest_version: string
  notes: string[]
  download_url: string
}

export interface VersionInfo {
  version: string
  note: string[]
  url: string
}
