import { ApiError } from "@servora/proto-utils/errors";
import {
  createFetch,
  FetchError,
  type FetchOptions,
  type MappedResponseType,
  type ResponseType,
} from "ofetch";
import type { StreamClient, TransportMeta } from "./types.ts";

export type HttpRequestOptions<R extends ResponseType = ResponseType> = Omit<
  FetchOptions<R>,
  "retryDelay"
> & {
  operation?: Partial<TransportMeta>;
  retryDelay?: FetchOptions["retryDelay"];
};

export interface HttpClientOptions extends FetchOptions {
  service: string;
  fetch?: typeof globalThis.fetch;
}

export interface HttpClient extends StreamClient {
  request<T = unknown, R extends ResponseType = "json">(
    path: string,
    options?: HttpRequestOptions<R>,
  ): Promise<MappedResponseType<R, T>>;
}

const transientStatuses = [408, 429, 500, 502, 503, 504];

function cancelled(meta: TransportMeta, signal: AbortSignal): ApiError {
  return new ApiError({
    ...meta,
    kind: "cancelled",
    message: "请求已取消",
    cause: signal.reason,
  });
}

function delay(
  milliseconds: number,
  signal?: AbortSignal | null,
): Promise<void> {
  if (signal?.aborted) return Promise.reject(signal.reason);
  const { promise, resolve, reject } = Promise.withResolvers<void>();
  const finish = () => {
    signal?.removeEventListener("abort", abort);
    resolve();
  };
  const timer = setTimeout(finish, milliseconds);
  const abort = () => {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
    reject(signal?.reason);
  };
  signal?.addEventListener("abort", abort, { once: true });
  return promise;
}

export function createHttpClient(options: HttpClientOptions): HttpClient {
  const {
    service,
    fetch: nativeFetch = globalThis.fetch.bind(globalThis),
    ...defaults
  } = options;
  const baseURL = defaults.baseURL ?? "/";
  if (typeof window === "undefined") {
    const url = new URL(baseURL);
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      throw new TypeError("Node HTTP 调用需要绝对 HTTP(S) baseURL");
    }
  }
  const browserOrigin =
    typeof window === "undefined" ? undefined : `${window.location.origin}/`;
  const resolvedBaseURL = browserOrigin
    ? new URL(baseURL, browserOrigin).href
    : baseURL;
  const headers = new Headers(defaults.headers);
  const cookiePolicy = defaults.credentials ?? "same-origin";
  const timeout = defaults.timeout ?? 10_000;
  const fetcher = createFetch({ fetch: nativeFetch });

  async function request<T = unknown, R extends ResponseType = "json">(
    path: string,
    options: HttpRequestOptions<R> = {},
  ): Promise<MappedResponseType<R, T>> {
    const { operation, ...overrides } = options;
    const method = (overrides.method ?? defaults.method ?? "GET").toUpperCase();
    const meta = {
      service: operation?.service ?? service,
      method: operation?.method ?? method,
    };
    const signal = overrides.signal ?? defaults.signal;
    const requestHeaders = new Headers(headers);
    new Headers(overrides.headers).forEach((value, key) =>
      requestHeaders.set(key, value),
    );
    if (overrides.body === null) requestHeaders.delete("content-type");
    const retry =
      overrides.retry ?? defaults.retry ?? (method === "GET" ? 1 : 0);
    const retries = retry === false ? 0 : Math.max(0, retry);
    const statuses =
      overrides.retryStatusCodes ??
      defaults.retryStatusCodes ??
      transientStatuses;
    const requestTimeout = overrides.timeout ?? timeout;
    const retryDelay = overrides.retryDelay ?? defaults.retryDelay ?? 0;
    const requestBaseURL =
      overrides.baseURL === undefined
        ? resolvedBaseURL
        : browserOrigin
          ? new URL(overrides.baseURL, browserOrigin).href
          : overrides.baseURL;

    for (let attempt = 0; ; attempt++) {
      if (signal?.aborted) throw cancelled(meta, signal);
      const controller = new AbortController();
      let timedOut = false;
      const abort = () => controller.abort(signal?.reason);
      signal?.addEventListener("abort", abort, { once: true });
      const timer =
        requestTimeout > 0
          ? setTimeout(() => {
              timedOut = true;
              controller.abort(new DOMException("请求超时", "TimeoutError"));
            }, requestTimeout)
          : undefined;
      let failure: ApiError;
      let original: unknown;
      try {
        const response = await fetcher.raw<T, R>(path, {
          ...defaults,
          ...overrides,
          baseURL: requestBaseURL,
          method,
          headers: requestHeaders,
          credentials: overrides.credentials ?? cookiePolicy,
          signal: controller.signal,
          timeout: 0,
          retry: 0,
        } as FetchOptions<R>);
        controller.signal.throwIfAborted();
        return response._data as MappedResponseType<R, T>;
      } catch (error) {
        original = error;
        if (signal?.aborted) {
          failure = new ApiError({
            ...meta,
            kind: "cancelled",
            message: "请求已取消",
            cause: error,
          });
        } else if (timedOut) {
          failure = new ApiError({
            ...meta,
            kind: "timeout",
            message: "请求超时",
            cause: error,
          });
        } else if (error instanceof FetchError) {
          failure = new ApiError({
            ...meta,
            kind: error.response ? "http" : "network",
            message: error.response ? "HTTP 请求失败" : "网络请求失败",
            httpStatus: error.response?.status,
            responseBody: error.data,
            cause: error,
          });
        } else {
          throw new Error("响应解析或请求处理失败", { cause: error });
        }
      } finally {
        clearTimeout(timer);
        signal?.removeEventListener("abort", abort);
      }
      const retryable =
        failure.kind === "network" ||
        (failure.kind === "http" && statuses.includes(failure.httpStatus!));
      if (!retryable || attempt >= retries) throw failure;
      const milliseconds =
        typeof retryDelay === "function" && original instanceof FetchError
          ? retryDelay({
              request: original.request ?? path,
              options: { ...original.options, headers: requestHeaders },
              response: original.response,
              error: original,
            })
          : typeof retryDelay === "number"
            ? retryDelay
            : 0;
      try {
        if (milliseconds > 0) await delay(milliseconds, signal);
      } catch {
        if (signal?.aborted) throw cancelled(meta, signal);
        throw failure;
      }
    }
  }

  return {
    request,
    baseURL,
    headers,
    credentials: cookiePolicy,
    timeout,
    fetch: nativeFetch,
  };
}
