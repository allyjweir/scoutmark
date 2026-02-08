import { useEffect, useRef, useMemo, useCallback } from 'react';
import { debounce } from 'lodash';
import { ScoutmarkSocket } from '../lib/websocket';
import type { WSServerMessage, WSSaveDraftPayload } from '../lib/types';

const getToken = () => localStorage.getItem('session_token');

// Singleton socket instance shared across the app
let socketInstance: ScoutmarkSocket | null = null;

const getSocket = (): ScoutmarkSocket => {
  if (!socketInstance) {
    socketInstance = new ScoutmarkSocket(getToken);
  }
  return socketInstance;
};

/**
 * useWebSocket manages the WebSocket connection lifecycle.
 * Call this once at the app level.
 */
export const useWebSocket = () => {
  const socket = useMemo(getSocket, []);

  useEffect(() => {
    const token = getToken();
    if (token) {
      socket.connect();
    }

    return () => {
      // Don't disconnect on unmount — keep alive across navigations
    };
  }, [socket]);

  return socket;
};

/**
 * useSessionSubscription subscribes to a session for live updates.
 */
export const useSessionSubscription = (
  sessionId: string | undefined,
  onMessage?: (msg: WSServerMessage) => void,
) => {
  const socket = useMemo(getSocket, []);

  useEffect(() => {
    if (!sessionId) return;

    socket.subscribeSession(sessionId);

    if (onMessage) {
      return socket.onMessage(onMessage);
    }
  }, [socket, sessionId, onMessage]);
};

/**
 * useDraftSync provides a debounced draft-save function over WebSocket.
 * Scores are auto-saved 500ms after the last change.
 */
export const useDraftSync = (sessionId: string, patrolId: string) => {
  const socket = useMemo(getSocket, []);
  const lastSavedRef = useRef<string | null>(null);

  const saveDraft = useCallback(
    (scores: Record<string, number>) => {
      const payload: WSSaveDraftPayload = {
        session_id: sessionId,
        patrol_id: patrolId,
        scores,
      };
      socket.saveDraft(payload).then((response) => {
        if (response.type === 'draft_saved') {
          lastSavedRef.current = new Date().toISOString();
        }
      });
    },
    [socket, sessionId, patrolId],
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

  return {
    saveDraft: debouncedSave,
    flushDraft: () => debouncedSave.flush(),
    lastSaved: lastSavedRef,
    connected: socket.connected,
  };
};
