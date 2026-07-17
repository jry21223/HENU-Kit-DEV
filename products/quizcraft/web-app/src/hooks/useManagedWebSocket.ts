import { useCallback, useEffect, useRef } from 'react';
import { openWebSocket } from '@/api/webSocket';

export const useManagedWebSocket = () => {
  const socketRef = useRef<WebSocket | null>(null);

  const close = useCallback(() => {
    socketRef.current?.close();
    socketRef.current = null;
  }, []);

  const connect = useCallback(
    (url: string) => {
      close();
      const socket = openWebSocket(url);
      socketRef.current = socket;
      socket.addEventListener(
        'close',
        () => {
          if (socketRef.current === socket) {
            socketRef.current = null;
          }
        },
        { once: true },
      );
      return socket;
    },
    [close],
  );

  useEffect(() => close, [close]);

  return { connect, close };
};
