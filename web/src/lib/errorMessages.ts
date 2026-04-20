import messages from '@errors'

export interface ApiErrorDetail {
  // クォータ超過時のフィールド
  resource?: string
  limit?: number
  requested?: number
  current?: number
  // ERR_INVALID_STATE のサブ理由
  reason?: string
  [key: string]: unknown
}

export interface ApiError {
  code: string
  message: string
  detail?: ApiErrorDetail
}

/**
 * API エラーレスポンスからユーザーフレンドリーな日本語メッセージを生成する。
 * メッセージ文字列は internal/apierror/messages.json から読み込まれる。
 */
export function getErrorMessage(error: ApiError): string {
  const { code, message, detail } = error

  switch (code) {
    case 'ERR_NO_HOST':
      return messages.ERR_NO_HOST

    case 'ERR_INSUFFICIENT_RESOURCES':
      return messages.ERR_INSUFFICIENT_RESOURCES

    case 'ERR_QUOTA_VCPU':
    case 'ERR_QUOTA_MEMORY':
    case 'ERR_QUOTA_VOLUME_GB':
    case 'ERR_QUOTA_VM_COUNT':
    case 'ERR_QUOTA_VOLUME_COUNT':
    case 'ERR_QUOTA_SNAPSHOT_COUNT':
    case 'ERR_QUOTA_NETWORK_COUNT':
    case 'ERR_QUOTA_EGRESS_COUNT':
    case 'ERR_QUOTA_INGRESS_COUNT':
    case 'ERR_QUOTA_EXCEEDED': {
      if (detail?.resource != null && detail?.limit != null && detail?.current != null) {
        return messages.ERR_QUOTA.with_detail
          .replace('{resource}', detail.resource)
          .replace('{current}', String(detail.current))
          .replace('{limit}', String(detail.limit))
      }
      return messages.ERR_QUOTA.fallback
    }

    case 'ERR_INVALID_STATE': {
      const reason = detail?.reason ?? 'default'
      const stateMessages = messages.ERR_INVALID_STATE as Record<string, string>
      return stateMessages[reason] ?? stateMessages['default']
    }

    case 'ERR_CONFLICT':
      return messages.ERR_CONFLICT

    case 'ERR_NOT_FOUND':
      return messages.ERR_NOT_FOUND

    case 'ERR_UNAUTHORIZED':
      return messages.ERR_UNAUTHORIZED.web

    case 'ERR_FORBIDDEN':
      return messages.ERR_FORBIDDEN

    case 'ERR_BAD_REQUEST':
      return message
        ? messages.ERR_BAD_REQUEST.with_message.replace('{message}', message)
        : messages.ERR_BAD_REQUEST.fallback

    default:
      return message || 'エラーが発生しました。'
  }
}

/**
 * ApiError を Error として throw できるクラス。
 * catch (e: unknown) で e instanceof Error として扱えるようにする。
 */
export class ApiErrorClass extends Error implements ApiError {
  code: string
  detail?: ApiErrorDetail
  constructor(apiError: ApiError) {
    super(getErrorMessage(apiError))
    this.name = 'ApiErrorClass'
    this.code = apiError.code
    this.detail = apiError.detail
  }
}
