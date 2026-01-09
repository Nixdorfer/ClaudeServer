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
  response: string
  done: boolean
}

export interface WSErrorData {
  error: string
}

export interface APIResponse {
  request_id: number
  conversations?: Conversation[]
  messages?: DBMessage[]
  [key: string]: unknown
}

export interface DBMessage {
  id: number
  conversation_id: string
  exchange_number: number
  request: string
  response: string
  status: string
  receive_time: string
  send_time?: string
  response_time?: string
  duration?: number
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

export interface UpdateCheckResult {
  has_update: boolean
  current_version: string
  latest_version: string
  notes: string[]
  download_url: string
}
