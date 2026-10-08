import type { AnyFrame, FrameType } from "./types";

// Static registry of dispatched server frame types; anything else is ignored.
const SERVER_TYPE_FLAGS: Record<string, true> = {
  hello: true,
  "chat.append": true,
  "agent.state": true,
  "site.updated": true,
  "room.state": true,
  "user.update": true,
  "room.update": true,
  "reset.progress": true,
  error: true,
};

function isFrameType(type: string): type is FrameType {
  return SERVER_TYPE_FLAGS[type] === true;
}

export interface SocketHandlers {
  onMessage: (frame: AnyFrame) => void;
  // `hello` doubles as the resync signal (plans/api.md §4 "Reconnect").
  onHello?: (frame: AnyFrame) => void;
  onStatus?: (connected: boolean) => void;
}

const MAX_BACKOFF = 5000;
const PING_INTERVAL = 30000;

// One WebSocket per channel with reconnect + keepalive + unknown-type
// tolerance, as required by plans/api.md §4.
export class Socket {
  private ws: WebSocket | null = null;
  private backoff = 500;
  private stopped = false;
  private reconnectTimer: number | null = null;
  private pingTimer: number | null = null;

  constructor(
    private readonly kind: "user" | "admin",
    private readonly handlers: SocketHandlers,
  ) {
    this.connect();
  }

  private connect(): void {
    if (this.stopped) return;
    const scheme = location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${scheme}//${location.host}/ws/${this.kind}`);
    this.ws = ws;

    ws.onopen = () => {
      this.backoff = 500;
      this.handlers.onStatus?.(true);
      this.pingTimer = window.setInterval(
        () => this.send({ type: "ping" }),
        PING_INTERVAL,
      );
    };

    ws.onmessage = (event: MessageEvent) => {
      if (typeof event.data !== "string") return;
      let parsed: unknown;
      try {
        parsed = JSON.parse(event.data);
      } catch {
        return;
      }
      if (
        typeof parsed !== "object" ||
        parsed === null ||
        !("type" in parsed) ||
        typeof parsed.type !== "string"
      ) {
        return;
      }
      const type = parsed.type;
      if (type === "ping") {
        this.send({ type: "pong" });
        return;
      }
      if (type === "pong" || !isFrameType(type)) return;
      // Validated above: the payload is a flat object with a known discriminator.
      const frame = parsed as AnyFrame;
      if (frame.type === "hello") this.handlers.onHello?.(frame);
      this.handlers.onMessage(frame);
    };

    ws.onclose = () => {
      this.clearTimers();
      this.handlers.onStatus?.(false);
      this.scheduleReconnect();
    };

    ws.onerror = () => {
      // onclose always follows; nothing to do beyond surfacing the drop.
    };
  }

  private scheduleReconnect(): void {
    if (this.stopped) return;
    const jitter = 0.8 + Math.random() * 0.4;
    const delay = Math.min(this.backoff, MAX_BACKOFF) * jitter;
    this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF);
    this.reconnectTimer = window.setTimeout(() => this.connect(), delay);
  }

  private clearTimers(): void {
    if (this.pingTimer !== null) {
      window.clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  send(value: Record<string, unknown>): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(value));
    }
  }

  close(): void {
    this.stopped = true;
    this.clearTimers();
    if (this.ws) {
      try {
        this.ws.close(1000, "client closing");
      } catch {
        // already closed
      }
    }
  }
}
