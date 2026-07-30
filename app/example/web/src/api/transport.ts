import { ApiError } from '@servora/proto-utils/errors'

import type { ClientTransport } from '@/api/generated/example/service/v1'
type UnaryParameters = Parameters<ClientTransport['unary']>
type UnaryPath = UnaryParameters[0]
type UnaryMethod = UnaryParameters[1]
type UnaryBody = UnaryParameters[2]
type UnaryMeta = UnaryParameters[3]

const baseUrl = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/+$/, '')
const requestTimeoutMs = 10_000

async function readResponseBody(response: Response): Promise<unknown> {
  const text = await response.text()
  if (text === '') return undefined

  try {
    return JSON.parse(text) as unknown
  } catch {
    return text
  }
}

export const transport: ClientTransport = {
  async unary<T>(
    path: UnaryPath,
    method: UnaryMethod,
    body: UnaryBody,
    meta: UnaryMeta,
  ): Promise<T> {
    const controller = new AbortController()
    let timedOut = false
    const timeout = setTimeout(() => {
      timedOut = true
      controller.abort()
    }, requestTimeoutMs)

    try {
      let response: Response
      try {
        const headers: Record<string, string> = {
          Accept: 'application/json',
        }
        if (body !== null) headers['Content-Type'] = 'application/json'

        const normalizedPath = path.startsWith('/') ? path : `/${path}`
        response = await fetch(`${baseUrl}${normalizedPath}`, {
          method,
          headers,
          body: body ?? undefined,
          signal: controller.signal,
        })
      } catch (cause: unknown) {
        if (timedOut) {
          throw new ApiError({
            kind: 'timeout',
            message: `Request timed out after ${requestTimeoutMs}ms`,
            service: meta.service,
            method: meta.method,
            cause,
          })
        }
        if (cause instanceof TypeError) {
          throw new ApiError({
            kind: 'network',
            message: 'Network request failed',
            service: meta.service,
            method: meta.method,
            cause,
          })
        }
        throw cause
      }

      try {
        const responseBody = await readResponseBody(response)

        if (!response.ok) {
          throw new ApiError({
            kind: 'http',
            message: `Request failed with HTTP ${response.status}`,
            httpStatus: response.status,
            responseBody,
            service: meta.service,
            method: meta.method,
          })
        }

        return responseBody as T
      } catch (cause: unknown) {
        if (cause instanceof ApiError) throw cause
        if (timedOut) {
          throw new ApiError({
            kind: 'timeout',
            message: `Request timed out after ${requestTimeoutMs}ms`,
            service: meta.service,
            method: meta.method,
            cause,
          })
        }
        throw cause
      }
    } finally {
      clearTimeout(timeout)
    }
  },
  serverStream() {
    throw new Error('The User reference API does not expose server streams')
  },
  duplexStream() {
    throw new Error('The User reference API does not expose duplex streams')
  },
}
