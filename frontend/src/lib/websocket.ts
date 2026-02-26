import { uniqueId } from 'lodash';
import type { WSClientMessage, WSServerMessage, WSSaveDraftPayload } from './types';

type MessageHandler = (msg: WSServerMessage) => void;

export class ScoutmarkSocket {
  private ws: WebSocket | null = null;
  private messageQueue: WSClientMessage[] = [];
  private handlers: Map<string, MessageHandler> = new Map();
  private globalHandlers: Set<MessageHandler> = new Set();
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 10;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private _connected = false;
  private _shouldReconnect = true;

  constructor(private getToken: () => string | null) {}

  /** Reset all internal state (handlers, queues, counters). */
  reset(): void {
    this.disconnect();
    this.handlers.clear();
    this.globalHandlers.clear();
    this.messageQueue = [];
    this.reconnectAttempts = 0;
  }

  get connected(): boolean {
    return this._connected;
  }

  connect(): void {
    const token = this.getToken();
    if (!token) return;

    this._shouldReconnect = true;

    // Auth via HttpOnly cookie — no token in URL to avoid leaking in logs/history
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/ws`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      this._connected = true;
      this.reconnectAttempts = 0;
      // Flush queued messages
      const queue = [...this.messageQueue];
      this.messageQueue = [];
      queue.forEach(msg => this.sendRaw(msg));
    };

    this.ws.onmessage = (event) => {
      try {
        const msg: WSServerMessage = JSON.parse(event.data);

        // Route to specific request handler if present
        if (msg.request_id) {
          const handler = this.handlers.get(msg.request_id);
          if (handler) {
            handler(msg);
            this.handlers.delete(msg.request_id);
          }
        }

        // Also send to global handlers
        this.globalHandlers.forEach(h => h(msg));
      } catch {
        // Ignore malformed messages
      }
    };

    this.ws.onclose = () => {
      this._connected = false;
      if (this._shouldReconnect) {
        this.attemptReconnect();
      }
    };

    this.ws.onerror = () => {
      this._connected = false;
    };
  }

  disconnect(): void {
    this._shouldReconnect = false;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      // Remove handlers before closing to prevent phantom reconnect
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws.onmessage = null;
      this.ws.close();
      this.ws = null;
    }
    this._connected = false;
  }

  /**
   * Subscribe to all incoming messages.
   * Returns an unsubscribe function.
   */
  onMessage(handler: MessageHandler): () => void {
    this.globalHandlers.add(handler);
    return () => this.globalHandlers.delete(handler);
  }

  /**
   * Subscribe to a scoring session to receive broadcast updates.
   */
  subscribeSession(sessionId: string): void {
    this.send('subscribe_session', { session_id: sessionId });
  }

  /**
   * Send presence heartbeat — tells other users we're viewing a patrol.
   */
  sendPresence(sessionId: string, patrolId: string): void {
    this.send('presence', { session_id: sessionId, patrol_id: patrolId });
  }

  /**
   * Save a draft. Debounce this on the caller side.
   * Returns a promise that resolves when the server acknowledges.
   */
  saveDraft(payload: WSSaveDraftPayload): Promise<WSServerMessage> {
    return new Promise((resolve) => {
      const requestId = this.send('save_draft', payload);
      // Set a timeout so we don't hang forever
      const timeout = setTimeout(() => {
        this.handlers.delete(requestId);
        resolve({ type: 'error', payload: { code: 'TIMEOUT', message: 'save timed out' } });
      }, 5000);

      this.handlers.set(requestId, (msg) => {
        clearTimeout(timeout);
        resolve(msg);
      });
    });
  }

  private send(type: string, payload: unknown): string {
    const requestId = uniqueId('req_');
    const msg: WSClientMessage = { request_id: requestId, type: type as WSClientMessage['type'], payload };

    if (this._connected && this.ws?.readyState === WebSocket.OPEN) {
      this.sendRaw(msg);
    } else {
      this.messageQueue.push(msg);
    }

    return requestId;
  }

  private sendRaw(msg: WSClientMessage): void {
    this.ws?.send(JSON.stringify(msg));
  }

  private attemptReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) return;

    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
    this.reconnectAttempts++;

    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }
}
