type Handler = (payload: unknown) => void;

class WS {
  private socket: WebSocket | null = null;
  private handlers = new Map<string, Set<Handler>>();
  private subscriptions = new Set<string>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  connect() {
    if (this.socket?.readyState === WebSocket.OPEN) return;

    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    this.socket = new WebSocket(`${proto}//${location.host}/ws`);

    this.socket.onopen = () => {
      // Re-subscribe to channels
      for (const ch of this.subscriptions) {
        this.send({ type: "subscribe", channel: ch });
      }
    };

    this.socket.onmessage = (e) => {
      const msg = JSON.parse(e.data);
      const handlers = this.handlers.get(msg.type);
      if (handlers) {
        for (const h of handlers) h(msg.payload);
      }
    };

    this.socket.onclose = () => {
      this.reconnectTimer = setTimeout(() => this.connect(), 3000);
    };
  }

  disconnect() {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.socket?.close();
    this.socket = null;
  }

  subscribe(channel: string) {
    this.subscriptions.add(channel);
    this.send({ type: "subscribe", channel });
  }

  unsubscribe(channel: string) {
    this.subscriptions.delete(channel);
    this.send({ type: "unsubscribe", channel });
  }

  on(type: string, handler: Handler): () => void {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, new Set());
    }
    this.handlers.get(type)!.add(handler);
    return () => this.handlers.get(type)?.delete(handler);
  }

  private send(data: unknown) {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(data));
    }
  }
}

export const ws = new WS();
