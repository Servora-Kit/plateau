import type { HttpClient, HttpRequestOptions } from "./http.js";
import { createServerStream, type SSEOptions } from "./sse.js";
import { createDuplexStream, type WebSocketOptions } from "./websocket.js";
import type {
  ClientTransport,
  DuplexStream,
  ServerStream,
  TransportMeta,
} from "./types.js";

export { createHttpClient } from "./http.js";
export type {
  HttpClient,
  HttpClientOptions,
  HttpRequestOptions,
} from "./http.js";
export { createServerStream } from "./sse.js";
export { createDuplexStream } from "./websocket.js";
export type { SSEOptions } from "./sse.js";
export type { WebSocketOptions } from "./websocket.js";
export type {
  ClientTransport,
  DuplexStream,
  ServerStream,
  TransportMeta,
} from "./types.js";

export interface TransportOptions {
  signal?: AbortSignal;
  http?: HttpRequestOptions<"json">;
  sse?: SSEOptions;
  websocket?: WebSocketOptions;
}

function isSafeHttpMethod(method: string): boolean {
  const upper = method.toUpperCase();
  return upper === "GET" || upper === "HEAD" || upper === "OPTIONS";
}

export function createTransport(
  client: HttpClient,
  options: TransportOptions = {},
): ClientTransport {
  return {
    unary<T>(
      path: string,
      method: string,
      body: string | null,
      meta: TransportMeta,
    ): Promise<T> {
      const headers = new Headers(options.http?.headers);
      headers.set("accept", "application/json");
      if (body !== null || !isSafeHttpMethod(method)) {
        headers.set("content-type", "application/json");
      }
      return client.request<T>(path, {
        ...options.http,
        signal: options.http?.signal ?? options.signal,
        headers,
        method,
        body,
        responseType: "json",
        operation: meta,
      });
    },
    serverStream<T>(path: string, meta: TransportMeta): ServerStream<T> {
      return createServerStream<T>(client, path, meta, {
        signal: options.signal,
        ...options.sse,
      });
    },
    duplexStream<TIn, TOut>(
      path: string,
      meta: TransportMeta,
      encode?: (data: TIn) => unknown,
    ): DuplexStream<TIn, TOut> {
      return createDuplexStream<TIn, TOut>(
        client,
        path,
        meta,
        { signal: options.signal, ...options.websocket },
        encode,
      );
    },
  };
}
