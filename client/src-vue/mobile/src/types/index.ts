export interface Message {
  id: string
  role: 'user' | 'assistant'
  content: string
  timestamp: Date
  isStreaming?: boolean
}

export interface Conversation {
  conversation_id: string
  name?: string
  message_count: number
  last_used_time: string
  is_generating: boolean
  first_message?: string
}

export interface WSContentData {
  delta: string
  text: string
}

export interface WSConversationData {
  conversation_id: string
}

export interface WSDoneData {
  conversation_id: string
  dialogue_id?: number
  response: string
  done: boolean
}

export interface LocalDialogue {
  id: number
  uid: string
  conversation_id: number
  order: number
  user_message: string
  assistant_message?: string
  create_time: string
  finish_time?: string
  request_time?: string
  status: string
  duration?: number
}

export interface LocalConv {
  id: number
  uid: string
  device_id: number
}

export interface WSErrorData {
  error: string
}

export interface UsageStatus {
  five_hour: number
  five_hour_reset: string
  seven_day: number
  seven_day_reset: string
  seven_day_sonnet: number
  seven_day_sonnet_reset: string
  is_blocked: boolean
  block_reason: string
  block_reset_time: string
  limit_five_hour: number
  limit_seven_day: number
}

export interface VersionInfo {
  version: string
  note: string[]
  url: string
}

export interface UpdateCheckResult {
  has_update: boolean
  current_version: string
  latest_version: string
  notes: string[]
  download_url: string
}

export interface VersionOutdatedData {
  current_version: string
  required_version: string
  message: string
}
