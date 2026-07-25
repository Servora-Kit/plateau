import { createRequestHandler, type RequestMeta } from '@servora/proto-utils/client'

import {
  createUserHTTPServiceClient,
  type ClientTransport,
  type User,
} from '@/api/generated/example/service/v1'

const requestHandler = createRequestHandler({
  baseUrl: import.meta.env.VITE_API_BASE_URL ?? '',
  timeoutMs: 10_000,
})

const transport: ClientTransport = {
  unary<T>(path: string, method: string, body: string | null, meta: RequestMeta): Promise<T> {
    return requestHandler<T>({ path, method, body }, meta)
  },
  serverStream() {
    throw new Error('The User reference API does not expose server streams')
  },
  duplexStream() {
    throw new Error('The User reference API does not expose duplex streams')
  },
}

export const userApi = createUserHTTPServiceClient(transport)
export type { User }
