import {
  createContext,
  createEffect,
  createMemo,
  createSignal,
  on,
  onCleanup,
  onMount,
  Show,
  useContext,
  type Accessor,
  type JSX,
} from "solid-js";
import { Navigate } from "@solidjs/router";
import { api } from "./lib/api";
import type { AnyFrame, Me, Role } from "./lib/types";
import { Socket } from "./lib/ws";

interface SessionValue {
  me: Accessor<Me | null>;
  loading: Accessor<boolean>;
  refresh: () => Promise<Me | null>;
  logout: () => Promise<void>;
  setMe: (value: Me | null) => void;
}

const SessionContext = createContext<SessionValue>();

export function useSession(): SessionValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error("useSession used outside SessionProvider");
  return value;
}

export function SessionProvider(props: { children: JSX.Element }): JSX.Element {
  const [me, setMe] = createSignal<Me | null>(null);
  const [loading, setLoading] = createSignal(true);

  const refresh = async (): Promise<Me | null> => {
    try {
      const result = (await api.get("/api/me")) as Me;
      // Keep the previous object when the payload is unchanged: downstream
      // effects depend on `me` identity, and a new-but-equal object must never
      // tear down a WebSocket (see AdminSocketProvider).
      setMe((prev) =>
        prev && JSON.stringify(prev) === JSON.stringify(result) ? prev : result,
      );
      return result;
    } catch {
      // 401/403 (D7) — treat as signed out so guards eject to "/".
      setMe(null);
      return null;
    }
  };

  const logout = async (): Promise<void> => {
    try {
      await api.post("/api/logout");
    } catch {
      // already gone server-side is fine
    }
    setMe(null);
  };

  onMount(() => {
    void refresh().then(() => setLoading(false));
  });

  return (
    <SessionContext.Provider value={{ me, loading, refresh, logout, setMe }}>
      {props.children}
    </SessionContext.Provider>
  );
}

// --- shared admin WebSocket ------------------------------------------------

type Listener = (frame: AnyFrame) => void;

interface AdminSocketValue {
  on: (listener: Listener) => () => void;
  connected: Accessor<boolean>;
}

const AdminSocketContext = createContext<AdminSocketValue>();

export function useAdminSocket(): AdminSocketValue {
  const value = useContext(AdminSocketContext);
  if (!value) throw new Error("useAdminSocket used outside AdminSocketProvider");
  return value;
}

// One `/ws/admin` connection shared by every admin route (plan 7.4), so the
// room list keeps updating live while only one socket exists.
export function AdminSocketProvider(props: {
  children: JSX.Element;
}): JSX.Element {
  const session = useSession();
  const [connected, setConnected] = createSignal(false);
  const listeners = new Set<Listener>();

  const value: AdminSocketValue = {
    on(listener: Listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    connected,
  };

  let socket: Socket | null = null;

  // Primitive identity: a refresh that returns equal data must not re-create
  // the socket. The effect depends ONLY on this string (value equality), and
  // the callback body runs untracked, so `session.me()` reads inside it add no
  // dependency.
  const identity = createMemo(() => {
    const current = session.me();
    return current ? `${current.role}:${current.userId ?? ""}` : null;
  });

  createEffect(
    on(identity, (id) => {
      if (socket) {
        socket.close();
        socket = null;
      }
      if (id === null || session.me()?.role !== "admin") {
        setConnected(false);
        return;
      }
      socket = new Socket("admin", {
        onMessage: (frame) => {
          for (const listener of [...listeners]) listener(frame);
        },
        onStatus: (up) => setConnected(up),
        // `hello` is the frozen resync signal, but resync must not re-create
        // the socket — it only refreshes REST state.
        onHello: () => {
          void session.refresh();
        },
      });
    }),
  );

  onCleanup(() => socket?.close());

  return (
    <AdminSocketContext.Provider value={value}>
      {props.children}
    </AdminSocketContext.Provider>
  );
}

// --- route guard -----------------------------------------------------------

export function RequireRole(props: {
  role: Role;
  children: JSX.Element;
}): JSX.Element {
  const session = useSession();
  const allowed = () => session.me()?.role === props.role;
  return (
    <Show
      when={!session.loading()}
      fallback={<div class="app-loading">Loading…</div>}
    >
      <Show when={allowed()} fallback={<Navigate href="/" />}>
        {props.children}
      </Show>
    </Show>
  );
}
