import {
  EventStreamContentType,
  fetchEventSource,
} from "@microsoft/fetch-event-source";
import type { ServerStream, StreamClient, TransportMeta } from "./types.ts";
import {
  asError,
  resolveBrowserURL,
  StreamCallbacks,
  streamApiError,
} from "./stream-utils.ts";

const TRANSIENT_STATUSES = new Set([408, 429, 500, 502, 503, 504]);
const DEFAULT_RETRY_DELAY = 1_000;
const MAX_RETRY_DELAY = 30_000;

export interface SSEOptions {
  signal?: AbortSignal;
  timeout?: number;
}

class FatalSSEError extends Error {
  constructor(readonly streamError: Error) {
    super(streamError.message, { cause: streamError });
    this.name = "FatalSSEError";
  }
}

class RetryableSSEError extends Error {
  constructor(readonly streamError: Error) {
    super(streamError.message, { cause: streamError });
    this.name = "RetryableSSEError";
  }
}

interface HttpFailureFacts {
  responseBody?: unknown;
  cause: unknown;
}

async function readHttpFailure(response: Response): Promise<HttpFailureFacts> {
  try {
    const text = await response.text();
    if (text === "") return { cause: response };
    try {
      return { responseBody: JSON.parse(text) as unknown, cause: response };
    } catch {
      return { responseBody: text, cause: response };
    }
  } catch (error) {
    return { cause: error };
  }
}

export function createServerStream<T>(
  client: StreamClient,
  path: string,
  meta: TransportMeta,
  options: SSEOptions = {},
): ServerStream<T> {
  const callbacks = new StreamCallbacks<T>();
  let streamController: AbortController | undefined = new AbortController();
  const timeout = options.timeout ?? client.timeout;
  let activeRequestController: AbortController | undefined;
  let activeRequestCleanup: (() => void) | undefined;
  let retryAttempt = 0;
  let serverRetryDelay: number | undefined;
  let finished = false;

  const releaseActiveRequest = (): void => {
    activeRequestCleanup?.();
    activeRequestCleanup = undefined;
    activeRequestController?.abort();
    activeRequestController = undefined;
  };

  const finish = (error?: Error): void => {
    if (finished) return;
    finished = true;
    releaseActiveRequest();
    const lifecycleController = streamController;
    streamController = undefined;
    lifecycleController?.abort();
    options.signal?.removeEventListener("abort", abort);
    if (typeof window !== "undefined")
      window.removeEventListener("pagehide", close);
    if (error) callbacks.fail(error);
    else callbacks.finish();
  };

  const close = (): void => finish();

  const abort = (): void => {
    finish(
      streamApiError("cancelled", "SSE 流已取消", meta, {
        cause: options.signal?.reason,
      }),
    );
  };

  options.signal?.addEventListener("abort", abort, { once: true });

  const stream: ServerStream<T> = {
    onEvent: (listener) => callbacks.onEvent(listener),
    onError: (handler) => callbacks.onError(handler),
    close,
  };

  queueMicrotask(() => {
    if (finished) return;
    if (options.signal?.aborted) {
      abort();
      return;
    }
    if (typeof window === "undefined" || typeof document === "undefined") {
      finish(new Error("SSE 流只能在浏览器中使用"));
      return;
    }
    if (!Number.isFinite(timeout) || timeout < 0) {
      finish(new TypeError("SSE 建连超时必须是非负有限数"));
      return;
    }

    window.addEventListener("pagehide", close);

    let url: URL;
    try {
      url = resolveBrowserURL(client.baseURL, path);
    } catch (error) {
      finish(asError(error, "SSE 地址无效"));
      return;
    }

    const headers = Object.fromEntries(client.headers.entries());
    headers.accept = EventStreamContentType;

    const timedFetch: typeof globalThis.fetch = async (input, init) => {
      releaseActiveRequest();
      const requestController = new AbortController();
      activeRequestController = requestController;
      let timedOut = false;
      let timer: number | undefined;
      const requestSignal = init?.signal;
      const propagateAbort = (): void =>
        requestController.abort(requestSignal?.reason);

      if (requestSignal?.aborted) propagateAbort();
      else
        requestSignal?.addEventListener("abort", propagateAbort, {
          once: true,
        });
      activeRequestCleanup = () => {
        if (timer !== undefined) window.clearTimeout(timer);
        requestSignal?.removeEventListener("abort", propagateAbort);
      };
      if (timeout > 0) {
        timer = window.setTimeout(() => {
          timedOut = true;
          requestController.abort(
            new DOMException("SSE 建连超时", "TimeoutError"),
          );
        }, timeout);
      }

      try {
        const response = await client.fetch(input, {
          ...init,
          signal: requestController.signal,
        });
        if (timer !== undefined) window.clearTimeout(timer);
        timer = undefined;
        return response;
      } catch (error) {
        if (timer !== undefined) window.clearTimeout(timer);
        timer = undefined;
        if (!timedOut) throw error;
        throw new RetryableSSEError(
          streamApiError("timeout", "SSE 建连超时", meta, { cause: error }),
        );
      }
    };

    const retryDelay = (): number => {
      if (serverRetryDelay !== undefined) return serverRetryDelay;
      const exponential = Math.min(
        MAX_RETRY_DELAY,
        DEFAULT_RETRY_DELAY * 2 ** retryAttempt++,
      );
      return Math.min(
        MAX_RETRY_DELAY,
        Math.round(exponential * (0.5 + Math.random())),
      );
    };

    const lifecycleController = streamController;
    if (!lifecycleController) return;

    void fetchEventSource(url.toString(), {
      method: "GET",
      credentials: client.credentials,
      headers,
      signal: lifecycleController.signal,
      openWhenHidden: true,
      fetch: timedFetch,
      async onopen(response) {
        if (!response.ok) {
          const failure = await readHttpFailure(response);
          const apiError = streamApiError(
            "http",
            `SSE 建连失败：HTTP ${response.status}`,
            meta,
            {
              httpStatus: response.status,
              ...failure,
            },
          );
          if (TRANSIENT_STATUSES.has(response.status))
            throw new RetryableSSEError(apiError);
          throw new FatalSSEError(apiError);
        }

        const mediaType = response.headers
          .get("content-type")
          ?.split(";", 1)[0]
          ?.trim()
          .toLowerCase();
        if (mediaType !== EventStreamContentType || response.body === null) {
          throw new FatalSSEError(
            new Error("SSE 响应协议无效", {
              cause: new TypeError(
                response.body === null
                  ? "SSE 响应缺少消息体"
                  : `SSE 响应 Content-Type 无效：${mediaType ?? "缺失"}`,
              ),
            }),
          );
        }
        retryAttempt = 0;
      },
      onmessage(event) {
        if (
          event.retry !== undefined &&
          Number.isSafeInteger(event.retry) &&
          event.retry >= 0
        ) {
          serverRetryDelay = Math.min(event.retry, MAX_RETRY_DELAY);
        }

        if (event.event === "error") {
          let cause: unknown = event.data;
          try {
            cause = JSON.parse(event.data) as unknown;
          } catch {}
          throw new FatalSSEError(new Error("SSE 远端流返回错误", { cause }));
        }
        if (event.event !== "" && event.event !== "message") return;
        if (event.data === "") return;

        try {
          callbacks.emit(JSON.parse(event.data) as T);
        } catch (error) {
          throw new FatalSSEError(
            new Error("SSE 消息不是有效 JSON", { cause: error }),
          );
        }
      },
      onclose() {
        finish();
      },
      onerror(error) {
        releaseActiveRequest();
        if (error instanceof FatalSSEError) {
          finish(error.streamError);
          throw error;
        }
        if (finished) throw asError(error, "SSE 流已结束");
        return retryDelay();
      },
    }).catch((error: unknown) => {
      if (!finished) finish(asError(error, "SSE 流异常终止"));
    });
  });

  return stream;
}
