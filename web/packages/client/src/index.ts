import type { HttpClient, HttpRequestOptions } from "./http-client.ts";
import { createServerStream, type SSEOptions } from "./sse.ts";
import { createDuplexStream, type WebSocketOptions } from "./websocket.ts";
import type { TransportMeta } from "./types.ts";

export { createHttpClient } from "./http-client.ts";
export type {
  HttpClient,
  HttpClientOptions,
  HttpRequestOptions,
} from "./http-client.ts";
export { createServerStream } from "./sse.ts";
export { createDuplexStream } from "./websocket.ts";
export type { SSEOptions } from "./sse.ts";
export type { WebSocketOptions } from "./websocket.ts";
export type { TransportMeta, ServerStream, DuplexStream } from "./types.ts";

export interface TransportOptions {
  signal?: AbortSignal;
  http?: HttpRequestOptions<"json">;
  sse?: SSEOptions;
  websocket?: WebSocketOptions;
}

export function createTransport(
  client: HttpClient,
  options: TransportOptions = {},
) {
  return {
    unary<T>(
      path: string,
      method: string,
      body: string | null,
      meta: TransportMeta,
    ): Promise<T> {
      const headers = new Headers(options.http?.headers);
      headers.set("accept", "application/json");
      if (body !== null) headers.set("content-type", "application/json");
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
    serverStream<T>(path: string, meta: TransportMeta) {
      return createServerStream<T>(client, path, meta, {
        signal: options.signal,
        ...options.sse,
      });
    },
    duplexStream<TIn, TOut>(
      path: string,
      meta: TransportMeta,
      encode?: (data: TIn) => unknown,
    ) {
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
