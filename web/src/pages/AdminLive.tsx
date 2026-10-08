import {
  createEffect,
  createMemo,
  createSignal,
  For,
  on,
  onCleanup,
  onMount,
  Show,
  untrack,
  type JSX,
} from "solid-js";
import { useParams, useSearchParams } from "@solidjs/router";
import { api, ApiError } from "../lib/api";
import type { Message, ResetProgressFrame, Room, User } from "../lib/types";
import { mergeMessages } from "../lib/messages";
import { ChatPanel } from "../components/ChatPanel";
import { Drawer } from "../components/Drawer";
import { PreviewTile } from "../components/PreviewTile";
import { useAdminSocket } from "../session";

const STATE_LABEL: Record<string, string> = {
  open: "open",
  started: "started",
  closed: "closed",
  archived: "archived",
};

export default function AdminLive(): JSX.Element {
  const params = useParams<{ roomId: string }>();
  const socket = useAdminSocket();
  const roomId = () => params.roomId;
  const [searchParams, setSearchParams] = useSearchParams<{ user?: string }>();

  const [room, setRoom] = createSignal<Room | null>(null);
  const [users, setUsers] = createSignal<User[]>([]);
  const [error, setError] = createSignal<string | null>(null);
  const [filter, setFilter] = createSignal("");
  const [page, setPage] = createSignal(1);
  const [perPage, setPerPage] = createSignal(8);
  const [broadcast, setBroadcast] = createSignal("");
  const [progress, setProgress] = createSignal<ResetProgressFrame | null>(null);
  const [dismissedHelp, setDismissedHelp] = createSignal<Set<number>>(
    new Set(),
  );

  const [drawerUser, setDrawerUser] = createSignal<User | null>(null);
  const [drawerMessages, setDrawerMessages] = createSignal<Message[]>([]);
  const [drawerText, setDrawerText] = createSignal("");
  const [drawerHasOlder, setDrawerHasOlder] = createSignal(false);

  let grid: HTMLDivElement | undefined;
  let refetchTimer: number | null = null;

  const report = (err: unknown, fallback: string) => {
    setError(err instanceof ApiError ? err.message : fallback);
  };

  const loadUsers = async () => {
    try {
      const body = (await api.get(`/api/rooms/${roomId()}/users`)) as {
        users: User[];
      };
      setUsers(body.users);
      setError(null);
    } catch (err) {
      report(err, "Could not load users.");
    }
  };

  const loadRoom = async () => {
    try {
      const body = (await api.get(`/api/rooms/${roomId()}`)) as { room: Room };
      setRoom(body.room);
    } catch (err) {
      report(err, "Could not load the room.");
    }
  };

  const loadDrawerMessages = async (before?: number) => {
    const user = drawerUser();
    if (!user) return;
    const query = before ? `?limit=50&before=${before}` : "?limit=50";
    try {
      const body = (await api.get(
        `/api/rooms/${roomId()}/users/${user.id}/messages${query}`,
      )) as { messages: Message[] };
      setDrawerMessages((prev) =>
        mergeMessages(before ? prev : [], body.messages),
      );
      setDrawerHasOlder(body.messages.length === 50);
    } catch (err) {
      report(err, "Could not load the chat.");
    }
  };

  const openDrawer = (user: User) => {
    setDrawerUser(user);
    setDrawerMessages([]);
    setDrawerText("");
    dismissHelp(user.id);
    void loadDrawerMessages();
    setSearchParams({ user: String(user.id) }, { replace: true });
  };

  const closeDrawer = () => {
    setDrawerUser(null);
    setDrawerMessages([]);
    setSearchParams({ user: undefined }, { replace: true });
  };

  // Deep link: /admin/rooms/{id}/live?user={id} opens that user's drawer. The
  // params and the loaded users drive it; `drawerUser` is read untracked so
  // closing the drawer cannot re-open it before the query param clears.
  createEffect(
    on(
      () => [searchParams.user, users()] as const,
      ([raw]) => {
        if (raw === undefined) return;
        const id = Number(raw);
        if (!Number.isFinite(id)) return;
        const user = users().find((item) => item.id === id);
        if (!user) return;
        untrack(() => {
          if (drawerUser()?.id !== id) openDrawer(user);
        });
      },
    ),
  );

  // Server-side clear (helpPending false) resets the local dismissal so a later
  // raise-hand lights the tile up again; dismissal itself is client-side (D4).
  const resetHelpDismissal = (id: number) => {
    setDismissedHelp((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  };

  const dismissHelp = (id: number) => {
    setDismissedHelp((prev) => {
      const next = new Set(prev);
      next.add(id);
      return next;
    });
  };

  onMount(() => {
    void loadRoom();
    void loadUsers();
  });

  onMount(() => {
    if (!grid) return;
    const observer = new ResizeObserver(() => {
      const width = grid ? grid.clientWidth : 0;
      const height = grid ? grid.clientHeight : 0;
      const columns = Math.max(1, Math.floor(width / 320));
      const rows = Math.max(1, Math.floor(height / 180));
      setPerPage(columns * rows);
    });
    observer.observe(grid);
    onCleanup(() => observer.disconnect());
  });

  onMount(() =>
    socket.on((frame) => {
      if (frame.type === "user.update") {
        const user = frame.user;
        setUsers((prev) => {
          const index = prev.findIndex((item) => item.id === user.id);
          if (index < 0) return [...prev, user];
          const next = [...prev];
          next[index] = user;
          return next;
        });
        // A fresh raise-hand after the server cleared it must light up again.
        if (!user.helpPending) resetHelpDismissal(user.id);
        if (drawerUser()?.id === user.id) setDrawerUser(user);
      } else if (frame.type === "room.update") {
        setRoom(frame.room);
      } else if (frame.type === "reset.progress") {
        setProgress(frame);
      } else if (frame.type === "chat.append") {
        const user = drawerUser();
        if (!user) return;
        const message = frame.message;
        if (frame.userId === null || frame.userId === user.id) {
          setDrawerMessages((prev) => mergeMessages(prev, [message]));
        } else if (frame.userId === undefined) {
          // No attribution: re-fetch rather than showing another user's message.
          if (refetchTimer !== null) window.clearTimeout(refetchTimer);
          refetchTimer = window.setTimeout(
            () => void loadDrawerMessages(),
            300,
          );
        }
      } else if (frame.type === "hello") {
        void loadRoom();
        void loadUsers();
      }
    }),
  );

  onCleanup(() => {
    if (refetchTimer !== null) window.clearTimeout(refetchTimer);
  });

  const filtered = createMemo(() => {
    const needle = filter().trim().toLowerCase();
    if (!needle) return users();
    return users().filter((user) => user.name.toLowerCase().includes(needle));
  });

  const pageCount = createMemo(() =>
    Math.max(1, Math.ceil(filtered().length / perPage())),
  );

  createEffect(() => {
    if (page() > pageCount()) setPage(pageCount());
  });

  const pageUsers = createMemo(() =>
    filtered().slice((page() - 1) * perPage(), page() * perPage()),
  );

  const isHelpPending = (user: User) =>
    user.helpPending && !dismissedHelp().has(user.id);

  const sendBroadcast = async () => {
    const text = broadcast().trim();
    if (!text) return;
    try {
      await api.post(`/api/rooms/${roomId()}/broadcast`, { text });
      setBroadcast("");
    } catch (err) {
      report(err, "Broadcast failed.");
    }
  };

  const sendTargeted = async () => {
    const user = drawerUser();
    const text = drawerText().trim();
    if (!user || !text) return;
    try {
      await api.post(`/api/rooms/${roomId()}/users/bulk`, {
        userIds: [user.id],
        action: "message",
        text,
      });
      setDrawerText("");
      void loadDrawerMessages();
    } catch (err) {
      report(err, "Message failed.");
    }
  };

  // Admin-side clear; the server emits user.update, which drops the badge.
  const clearHelp = async (user: User) => {
    try {
      await api.post(`/api/rooms/${roomId()}/users/${user.id}/help`, {
        pending: false,
      });
    } catch (err) {
      report(err, "Could not clear the help request.");
    }
  };

  const oldestId = () => drawerMessages()[0]?.id;

  return (
    <section class="admin-page admin-live">
      <header class="page-head">
        <div>
          <h1 class="title is-4">
            {room()?.name || "Room"}{" "}
            <span class="tag is-light">
              {room() ? STATE_LABEL[room()!.state] : "…"}
            </span>
          </h1>
          <p class="subtitle is-6">
            <code>{roomId()}</code> · {users().length} users
          </p>
        </div>
        <div class="page-head-actions">
          <a class="button" href={`/admin/rooms/${roomId()}`}>
            Settings
          </a>
          <a class="button" href="/admin">
            All rooms
          </a>
        </div>
      </header>

      <Show when={error()}>
        <div class="notification is-danger is-light">{error()}</div>
      </Show>

      <Show when={progress()?.running}>
        <div class="template-progress">
          <progress
            class="progress is-small is-link"
            max={Math.max(1, progress()!.total)}
            value={progress()!.done}
          >
            {progress()!.done}/{progress()!.total}
          </progress>
          <span class="is-size-7">
            resetting {progress()!.done}/{progress()!.total} users
          </span>
        </div>
      </Show>

      <div class="live-toolbar">
        <div class="field">
          <div class="control">
            <input
              class="input is-small"
              type="search"
              placeholder="Filter by name"
              value={filter()}
              onInput={(event) => {
                setFilter(event.currentTarget.value);
                setPage(1);
              }}
            />
          </div>
        </div>

        <div class="field has-addons broadcast-field">
          <div class="control is-expanded">
            <input
              class="input is-small"
              type="text"
              placeholder="Message all users"
              value={broadcast()}
              onInput={(event) => setBroadcast(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void sendBroadcast();
              }}
            />
          </div>
          <div class="control">
            <button
              class="button is-small is-primary"
              onClick={() => void sendBroadcast()}
            >
              Send all
            </button>
          </div>
        </div>

        <div class="pager">
          <button
            class="button is-small"
            disabled={page() <= 1}
            onClick={() => setPage((value) => Math.max(1, value - 1))}
          >
            Prev
          </button>
          <span>
            {page()} / {pageCount()}
          </span>
          <button
            class="button is-small"
            disabled={page() >= pageCount()}
            onClick={() => setPage((value) => Math.min(pageCount(), value + 1))}
          >
            Next
          </button>
        </div>
      </div>

      <div class="tile-grid" ref={grid}>
        <For each={pageUsers()}>
          {(user) => (
            <PreviewTile
              roomId={roomId()}
              user={user}
              helpPending={isHelpPending(user)}
              onOpenDrawer={() => openDrawer(user)}
              onDismissHelp={() => dismissHelp(user.id)}
            />
          )}
        </For>
      </div>

      <Show when={filtered().length === 0}>
        <p class="has-text-grey">No users match this filter.</p>
      </Show>

      <Drawer
        title={drawerUser() ? `${drawerUser()!.name} — chat and site` : "Chat"}
        open={drawerUser() !== null}
        onClose={closeDrawer}
      >
        <Show when={drawerUser()?.helpPending}>
          <button
            class="button is-warning is-small is-fullwidth drawer-clear-help"
            onClick={() => void clearHelp(drawerUser()!)}
          >
            Clear help
          </button>
        </Show>
        <div class="drawer-chat">
          <Show when={drawerHasOlder()}>
            <button
              class="button is-small is-fullwidth"
              onClick={() => void loadDrawerMessages(oldestId())}
            >
              Load older
            </button>
          </Show>
          <ChatPanel
            messages={drawerMessages()}
            viewer="admin"
            userName={drawerUser()?.name}
          />
        </div>
        <div class="drawer-message field has-addons">
          <div class="control is-expanded">
            <input
              class="input"
              type="text"
              placeholder="Message this user"
              value={drawerText()}
              onInput={(event) => setDrawerText(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void sendTargeted();
              }}
            />
          </div>
          <div class="control">
            <button
              class="button is-primary"
              onClick={() => void sendTargeted()}
            >
              Send
            </button>
          </div>
        </div>
      </Drawer>
    </section>
  );
}
