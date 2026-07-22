import { useEffect, useRef, useMemo, useCallback } from 'react';
import { debounce } from 'lodash';
import { ScoutmarkSocket } from '../lib/websocket';
import type { WSServerMessage, WSSaveDraftPayload } from '../lib/types';

const getToken = () => localStorage.getItem('session_token');

// Singleton socket instance shared across the app.
// We intentionally never nullify this — every hook that calls getSocket()
// must always receive the same object, otherwise useDraftSync and
// useWebSocket end up with different instances (split-brain).
const socketInstance = new ScoutmarkSocket(getToken);

const getSocket = (): ScoutmarkSocket => socketInstance;

/**
 * useWebSocket manages the WebSocket connection lifecycle.
 * Call this once at the app level. Pass `isAuthenticated` so the
 * socket connects after login and disconnects after logout.
 */
export const useWebSocket = (isAuthenticated = false) => {
  const socket = getSocket();

  useEffect(() => {
    if (isAuthenticated) {
      const token = getToken();
      if (token && !socket.connected) {
        socket.connect();
      }
    } else {
      // Reset clears handlers, queues, and disconnects — but keeps the
      // same object so every hook still references the same instance.
      socket.reset();
    }
  }, [socket, isAuthenticated]);

  return socket;
};

/**
 * useSessionSubscription subscribes to a session for live updates.
 */
export const useSessionSubscription = (
  sessionId: string | undefined,
  onMessage?: (msg: WSServerMessage) => void,
) => {
  const socket = getSocket();

  useEffect(() => {
    if (!sessionId) return;

    socket.subscribeSession(sessionId);
    const unsubscribeMessageHandler = onMessage ? socket.onMessage(onMessage) : undefined;

    return () => {
      socket.unsubscribeSession(sessionId);
      unsubscribeMessageHandler?.();
    };
  }, [socket, sessionId, onMessage]);
};

/**
 * usePresence sends periodic presence heartbeats so other users
 * know we're viewing a patrol. Fires immediately on patrol change
 * and every 15s while the patrol stays the same.
 * Optionally includes which criterion is being commented on.
 */
export const usePresence = (sessionId: string | undefined, patrolId: string | undefined, commentingOn?: string) => {
  const socket = getSocket();

  useEffect(() => {
    if (!sessionId || !patrolId) return;

    // Send immediately
    socket.sendPresence(sessionId, patrolId, commentingOn);

    // Then every 15 seconds
    const interval = setInterval(() => {
      socket.sendPresence(sessionId, patrolId, commentingOn);
    }, 15_000);

    return () => clearInterval(interval);
  }, [socket, sessionId, patrolId, commentingOn]);
};

/**
 * useDraftSync provides a debounced draft-save function over WebSocket.
 * Scores are auto-saved 500ms after the last change.
 */
export const useDraftSync = (
  sessionId: string,
  patrolId: string,
  onSaveResponse?: (msg: WSServerMessage) => void,
) => {
  const socket = getSocket();
  const lastSavedRef = useRef<string | null>(null);

  const saveDraft = useCallback(
    (scores: Record<string, number>) => {
      const payload: WSSaveDraftPayload = {
        session_id: sessionId,
        patrol_id: patrolId,
        scores,
      };
      return socket.saveDraft(payload).then((response) => {
        if (response.type === 'draft_saved') {
          lastSavedRef.current = new Date().toISOString();
        }
        onSaveResponse?.(response);
        return response;
      });
    },
    [socket, sessionId, patrolId, onSaveResponse],
  );

  const debouncedSave = useMemo(
    () => debounce(saveDraft, 500, { maxWait: 2000 }),
    [saveDraft],
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedSave.flush();
    };
  }, [debouncedSave]);

  const flushDraft = useCallback(async () => {
    // Flush fires the debounced save immediately and returns its result
    await debouncedSave.flush();
    // Small delay to ensure the WebSocket round-trip completes
    await new Promise((r) => setTimeout(r, 200));
  }, [debouncedSave]);

  return {
    saveDraft: debouncedSave,
    flushDraft,
    lastSaved: lastSavedRef,
    connected: socket.connected,
  };
};
