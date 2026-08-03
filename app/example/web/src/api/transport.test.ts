import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@servora/proto-utils/errors'

import { transport } from '@/api/transport'

const meta = {
  service: 'UserService',
  method: 'GetUser',
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('application ClientTransport', () => {
  it('sends bodyless requests with Accept but without Content-Type', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ name: 'tenants/demo/users/ada' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await transport.unary<{ name: string }>('v1/users/ada', 'GET', null, meta)

    expect(result).toEqual({ name: 'tenants/demo/users/ada' })
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, options] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v1/users/ada')
    expect(options.method).toBe('GET')
    expect(options.body).toBeUndefined()
    expect(options.headers).toEqual({ Accept: 'application/json' })
    expect(options.signal).toBeInstanceOf(AbortSignal)
  })

  it('sends generated ProtoJSON bodies without serializing them again', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ name: 'tenants/demo/users/ada' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const body = JSON.stringify({ revision: '9223372036854775807' })

    await transport.unary('/v1/users', 'POST', body, {
      service: 'UserService',
      method: 'CreateUser',
    })

    const [, options] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(options.body).toBe(body)
    expect(options.headers).toEqual({
      Accept: 'application/json',
      'Content-Type': 'application/json',
    })
  })

  it.each([
    {
      label: 'JSON',
      response: new Response(
        JSON.stringify({
          code: 409,
          reason: 'USER_ERROR_REASON_ALREADY_EXISTS',
          message: 'user already exists',
        }),
        { status: 409, headers: { 'Content-Type': 'application/json' } },
      ),
      expectedBody: {
        code: 409,
        reason: 'USER_ERROR_REASON_ALREADY_EXISTS',
        message: 'user already exists',
      },
    },
    {
      label: 'text',
      response: new Response('upstream unavailable', { status: 502 }),
      expectedBody: 'upstream unavailable',
    },
    {
      label: 'empty',
      response: new Response(null, { status: 503 }),
      expectedBody: undefined,
    },
  ])('preserves $label HTTP error bodies and call metadata', async ({ response, expectedBody }) => {
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockResolvedValue(response))

    let error: unknown
    try {
      await transport.unary('/v1/users/ada', 'GET', null, meta)
    } catch (cause: unknown) {
      error = cause
    }

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      kind: 'http',
      httpStatus: response.status,
      responseBody: expectedBody,
      service: 'UserService',
      method: 'GetUser',
    })
  })

  it('classifies native fetch TypeError failures as network errors', async () => {
    const cause = new TypeError('fetch failed')
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockRejectedValue(cause))

    let error: unknown
    try {
      await transport.unary('/v1/users/ada', 'GET', null, meta)
    } catch (caught: unknown) {
      error = caught
    }

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      kind: 'network',
      service: 'UserService',
      method: 'GetUser',
      cause,
    })
  })

  it('rethrows TypeError raised while reading a response body', async () => {
    const cause = new TypeError('body stream failed')
    const response = {
      ok: true,
      status: 200,
      text: vi.fn<() => Promise<string>>().mockRejectedValue(cause),
    } as unknown as Response
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockResolvedValue(response))

    await expect(transport.unary('/v1/users/ada', 'GET', null, meta)).rejects.toBe(cause)
  })

  it('rethrows errors that the adapter cannot classify', async () => {
    const cause = new Error('unexpected adapter failure')
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockRejectedValue(cause))

    await expect(transport.unary('/v1/users/ada', 'GET', null, meta)).rejects.toBe(cause)
  })

  it('classifies its own abort deadline as a timeout error', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, options: RequestInit) => {
        return new Promise<Response>((_resolve, reject) => {
          options.signal?.addEventListener('abort', () => {
            reject(new DOMException('The operation was aborted', 'AbortError'))
          })
        })
      }),
    )

    const request = transport.unary('/v1/users/ada', 'GET', null, meta)
    const requestResult = request.catch((error: unknown) => error)
    await vi.advanceTimersByTimeAsync(10_000)
    await expect(requestResult).resolves.toMatchObject({
      kind: 'timeout',
      service: 'UserService',
      method: 'GetUser',
    })
  })

  it('reports unsupported streaming modes explicitly', () => {
    expect(() => transport.serverStream('/v1/users:watch', meta)).toThrow(
      'does not expose server streams',
    )
    expect(() => transport.duplexStream('/v1/users:sync', meta)).toThrow(
      'does not expose duplex streams',
    )
  })
})
