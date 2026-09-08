import type { DuplexStream, StreamClient, TransportMeta } from "./types.ts";
import {
  asError,
  resolveBrowserURL,
  StreamCallbacks,
  streamApiError,
} from "./stream-utils.ts";

const CONTROL_PREFIX = "\u001e";
const CONTROL_END = `${CONTROL_PREFIX}end`;
const CONTROL_ERROR = `${CONTROL_PREFIX}error:`;
const DEFAULT_MAX_BUFFERED_BYTES = 1024 * 1024;

export interface WebSocketOptions {
  signal?: AbortSignal;
  timeout?: number;
  maxBufferedBytes?: number;
  headers?: HeadersInit;
}

interface QueuedFrame {
  data: string;
  bytes: number;
}
function utf8ByteLength(value: string): number {
  let bytes = 0;
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code < 0x80) bytes += 1;
    else if (code < 0x800) bytes += 2;
    else if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      if (next >= 0xdc00 && next <= 0xdfff) {
        bytes += 4;
        index += 1;
      } else bytes += 3;
    } else bytes += 3;
  }
  return bytes;
}

export function createDuplexStream<TIn, TOut>(
  client: StreamClient,
  path: string,
  meta: TransportMeta,
  options: WebSocketOptions = {},
  encode?: (data: TIn) => unknown,
): DuplexStream<TIn, TOut> {
  const callbacks = new StreamCallbacks<TOut>();
  const queue: QueuedFrame[] = [];
  const timeout = options.timeout ?? client.timeout;
  const maxBufferedBytes =
    options.maxBufferedBytes ?? DEFAULT_MAX_BUFFERED_BYTES;
  let socket: WebSocket | undefined;
  let queuedBytes = 0;
  let connectTimer: number | undefined;
  let sendClosed = false;
  let finished = false;

  let setupError: Error | undefined;
  if (options.headers !== undefined) {
    setupError = new TypeError("浏览器 WebSocket 不支持自定义握手请求头");
  } else if (!Number.isFinite(timeout) || timeout < 0) {
    setupError = new TypeError("WebSocket 建连超时必须是非负有限数");
  } else if (!Number.isSafeInteger(maxBufferedBytes) || maxBufferedBytes <= 0) {
    setupError = new TypeError("WebSocket 缓冲上限必须是正安全整数");
  }

  const removeSocketListeners = (): void => {
    socket?.removeEventListener("open", handleOpen);
    socket?.removeEventListener("message", handleMessage);
    socket?.removeEventListener("error", handleSocketError);
    socket?.removeEventListener("close", handleClose);
  };

  const finish = (
    error?: Error,
    closeSocket = true,
    closeCode?: number,
  ): void => {
    if (finished) return;
    finished = true;
    sendClosed = true;
    queue.length = 0;
    queuedBytes = 0;
    if (connectTimer !== undefined) {
      window.clearTimeout(connectTimer);
      connectTimer = undefined;
    }
    options.signal?.removeEventListener("abort", abort);
    if (typeof window !== "undefined")
      window.removeEventListener("pagehide", close);
    removeSocketListeners();
    const closingSocket = socket;
    socket = undefined;

    if (
      closeSocket &&
      closingSocket &&
      closingSocket.readyState < WebSocket.CLOSING
    ) {
      try {
        if (closeCode === undefined) closingSocket.close();
        else closingSocket.close(closeCode);
      } catch {}
    }
    if (error) callbacks.fail(error);
    else callbacks.finish();
  };

  const close = (): void => finish(undefined, true, 1000);

  const abort = (): void => {
    finish(
      streamApiError("cancelled", "WebSocket 流已取消", meta, {
        cause: options.signal?.reason,
      }),
    );
  };

  const assertSendable = (): void => {
    if (setupError) throw setupError;
    if (options.signal?.aborted) {
      throw streamApiError("cancelled", "WebSocket 流已取消", meta, {
        cause: options.signal.reason,
      });
    }
    if (finished) throw new Error("WebSocket 流已关闭");
    if (sendClosed) throw new Error("WebSocket 发送侧已关闭");
    if (socket && socket.readyState >= WebSocket.CLOSING) {
      throw new Error("WebSocket 连接正在关闭或已经关闭");
    }
  };

  const write = (data: string): void => {
    assertSendable();
    const bytes = utf8ByteLength(data);
    const nativeBuffered =
      socket?.readyState === WebSocket.OPEN ? socket.bufferedAmount : 0;
    if (queuedBytes + nativeBuffered + bytes > maxBufferedBytes) {
      throw new RangeError(
        `WebSocket 发送缓冲超过 ${maxBufferedBytes} 字节上限`,
      );
    }

    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(data);
      return;
    }
    queue.push({ data, bytes });
    queuedBytes += bytes;
  };

  const send = (data: TIn): void => {
    assertSendable();
    const mapped = encode ? encode(data) : data;
    const serialized = JSON.stringify(mapped);
    if (serialized === undefined)
      throw new TypeError("WebSocket 消息不能序列化为 JSON");
    write(serialized);
  };

  const closeSend = (): void => {
    if (sendClosed) return;
    write(CONTROL_END);
    sendClosed = true;
  };

  function handleOpen(): void {
    if (finished || !socket) return;
    if (connectTimer !== undefined) {
      window.clearTimeout(connectTimer);
      connectTimer = undefined;
    }

    try {
      for (const frame of queue) socket.send(frame.data);
      queue.length = 0;
      queuedBytes = 0;
    } catch (error) {
      finish(new Error("WebSocket 待发送消息写入失败", { cause: error }));
    }
  }

  function handleMessage(event: MessageEvent<unknown>): void {
    if (finished) return;
    if (typeof event.data !== "string") {
      finish(new Error("WebSocket 收到非文本数据帧", { cause: event.data }));
      return;
    }
    if (event.data === CONTROL_END) {
      finish(undefined, true, 1000);
      return;
    }
    if (event.data.startsWith(CONTROL_ERROR)) {
      const detail = event.data.slice(CONTROL_ERROR.length);
      finish(
        new Error("WebSocket 远端流返回错误", {
          cause: new Error(detail || "远端未提供错误详情"),
        }),
      );
      return;
    }
    if (event.data.startsWith(CONTROL_PREFIX)) {
      finish(new Error(`WebSocket 收到未知控制帧：${event.data.slice(1)}`));
      return;
    }

    try {
      callbacks.emit(JSON.parse(event.data) as TOut);
    } catch (error) {
      finish(new Error("WebSocket 数据帧不是有效 JSON", { cause: error }));
    }
  }

  function handleSocketError(event: Event): void {
    finish(
      streamApiError("network", "WebSocket 连接失败", meta, {
        cause: event,
      }),
    );
  }

  function handleClose(event: CloseEvent): void {
    if (event.code === 1000 || event.code === 1001) {
      finish(undefined, false);
      return;
    }
    finish(
      new Error(
        `WebSocket 异常关闭：${event.code}${event.reason ? ` ${event.reason}` : ""}`,
        {
          cause: event,
        },
      ),
      false,
    );
  }

  options.signal?.addEventListener("abort", abort, { once: true });

  const stream: DuplexStream<TIn, TOut> = {
    send,
    closeSend,
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
    if (setupError) {
      finish(setupError, false);
      return;
    }
    if (typeof window === "undefined" || typeof WebSocket === "undefined") {
      finish(new Error("WebSocket 流只能在浏览器中使用"), false);
      return;
    }

    window.addEventListener("pagehide", close);

    try {
      const url = resolveBrowserURL(client.baseURL, path);
      if (url.username || url.password) {
        throw new TypeError("WebSocket 地址不能包含认证凭据");
      }
      if (url.protocol === "http:") url.protocol = "ws:";
      else if (url.protocol === "https:") url.protocol = "wss:";
      else if (url.protocol !== "ws:" && url.protocol !== "wss:") {
        throw new TypeError(`WebSocket 不支持 ${url.protocol} 地址`);
      }
      socket = new WebSocket(url);
      socket.addEventListener("open", handleOpen);
      socket.addEventListener("message", handleMessage);
      socket.addEventListener("error", handleSocketError);
      socket.addEventListener("close", handleClose);
      if (timeout > 0) {
        connectTimer = window.setTimeout(() => {
          finish(
            streamApiError("timeout", "WebSocket 建连超时", meta, {
              cause: new DOMException("WebSocket 建连超时", "TimeoutError"),
            }),
          );
        }, timeout);
      }
    } catch (error) {
      finish(asError(error, "WebSocket 地址或构造参数无效"), false);
    }
  });

  return stream;
}
