import { createSignal, For, onMount, Show, type JSX } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { FaSolidPlus, FaSolidRightFromBracket } from "solid-icons/fa";
import { api, ApiError } from "../lib/api";
import { STATE_TAG, TRANSITIONS } from "../lib/rooms";
import type { Room, RoomUpdateFrame } from "../lib/types";
import { Modal } from "../components/Modal";
import { useAdminSocket, useSession } from "../session";

const createdFormat = new Intl.DateTimeFormat(undefined, {
  dateStyle: "short",
  timeStyle: "short",
});

export default function AdminRooms(): JSX.Element {
  const session = useSession();
  const navigate = useNavigate();
  const socket = useAdminSocket();
  const [rooms, setRooms] = createSignal<Room[]>([]);
  const [error, setError] = createSignal<string | null>(null);
  const [createOpen, setCreateOpen] = createSignal(false);
  const [createName, setCreateName] = createSignal("");
  const [renameTarget, setRenameTarget] = createSignal<Room | null>(null);
  const [renameValue, setRenameValue] = createSignal("");
  const [deleteTarget, setDeleteTarget] = createSignal<Room | null>(null);

  const load = async () => {
    try {
      const body = (await api.get("/api/rooms")) as { rooms: Room[] };
      setRooms(body.rooms);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not load rooms.");
    }
  };

  onMount(() => {
    void load();
  });

  onMount(() =>
    socket.on((frame) => {
      if (frame.type === "room.update") {
        const room = (frame as RoomUpdateFrame).room;
        setRooms((prev) => {
          const index = prev.findIndex((item) => item.id === room.id);
          if (index < 0) return [...prev, room];
          const next = [...prev];
          next[index] = room;
          return next;
        });
      } else if (frame.type === "hello") {
        void load();
      }
    }),
  );

  const runAction = async (room: Room, action: string) => {
    try {
      await api.post(`/api/rooms/${room.id}/${action}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Action failed.");
    }
  };

  const submitCreate = async () => {
    try {
      const body = (await api.post("/api/rooms", {
        name: createName().trim() || null,
      })) as { room: Room };
      setCreateOpen(false);
      setCreateName("");
      navigate(`/admin/rooms/${body.room.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create room.");
    }
  };

  const submitRename = async () => {
    const room = renameTarget();
    if (!room) return;
    try {
      await api.patch(`/api/rooms/${room.id}`, {
        name: renameValue().trim() || null,
      });
      setRenameTarget(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Rename failed.");
    }
  };

  const confirmDelete = async () => {
    const room = deleteTarget();
    if (!room) return;
    try {
      await api.del(`/api/rooms/${room.id}`);
      setRooms((prev) => prev.filter((item) => item.id !== room.id));
      setDeleteTarget(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Delete failed.");
    }
  };

  const logout = async () => {
    await session.logout();
    navigate("/", { replace: true });
  };

  return (
    <section class="admin-page">
      <header class="page-head">
        <div>
          <h1 class="title is-4">Rooms</h1>
          <p class="subtitle is-6">All event rooms, live.</p>
        </div>
        <div class="page-head-actions">
          <button class="button is-primary" onClick={() => setCreateOpen(true)}>
            <FaSolidPlus />
            <span>Create room</span>
          </button>
          <button class="button" onClick={() => void logout()}>
            <FaSolidRightFromBracket />
            <span>Log out</span>
          </button>
        </div>
      </header>

      <Show when={error()}>
        <div class="notification is-danger is-light">{error()}</div>
      </Show>

      <div class="table-container">
        <table class="table is-fullwidth is-hoverable">
          <thead>
            <tr>
              <th>Name</th>
              <th>ID</th>
              <th>Users</th>
              <th>State</th>
              <th>Created</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <For each={rooms()}>
              {(room) => (
                <tr>
                  <td>
                    <div class="room-name-cell">
                      <span>{room.name || "—"}</span>
                      <button
                        class="button is-small is-ghost"
                        onClick={() => {
                          setRenameTarget(room);
                          setRenameValue(room.name ?? "");
                        }}
                      >
                        rename
                      </button>
                      <a
                        class="button is-small is-ghost"
                        href={`/admin/rooms/${room.id}`}
                      >
                        settings
                      </a>
                      <a
                        class="button is-small is-ghost"
                        href={`/admin/rooms/${room.id}/live`}
                      >
                        live
                      </a>
                    </div>
                  </td>
                  <td>
                    <code>{room.id}</code>
                  </td>
                  <td>{room.userCount}</td>
                  <td>
                    <span class={STATE_TAG[room.state]}>
                      {room.state}
                    </span>
                  </td>
                  <td>{createdFormat.format(new Date(room.createdAt * 1000))}</td>
                  <td>
                    <div class="buttons are-small">
                      <For each={TRANSITIONS[room.state]}>
                        {(transition) => (
                          <button
                            class="button"
                            onClick={() => void runAction(room, transition.action)}
                          >
                            {transition.label}
                          </button>
                        )}
                      </For>
                      <Show when={room.state === "archived"}>
                        <button
                          class="button is-danger"
                          onClick={() => setDeleteTarget(room)}
                        >
                          Delete
                        </button>
                      </Show>
                    </div>
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>

      <Show when={rooms().length === 0}>
        <p class="has-text-grey">No rooms yet — create one to get started.</p>
      </Show>

      <Show when={createOpen()}>
        <Modal
          title="Create room"
          onClose={() => setCreateOpen(false)}
          footer={
            <>
              <button class="button is-primary" onClick={() => void submitCreate()}>
                Create
              </button>
              <button class="button" onClick={() => setCreateOpen(false)}>
                Cancel
              </button>
            </>
          }
        >
          <div class="field">
            <label class="label">Name (optional)</label>
            <div class="control">
              <input
                class="input"
                type="text"
                value={createName()}
                onInput={(event) => setCreateName(event.currentTarget.value)}
              />
            </div>
          </div>
        </Modal>
      </Show>

      <Show when={renameTarget()}>
        <Modal
          title="Rename room"
          onClose={() => setRenameTarget(null)}
          footer={
            <>
              <button class="button is-primary" onClick={() => void submitRename()}>
                Save
              </button>
              <button class="button" onClick={() => setRenameTarget(null)}>
                Cancel
              </button>
            </>
          }
        >
          <div class="field">
            <label class="label">Name</label>
            <div class="control">
              <input
                class="input"
                type="text"
                value={renameValue()}
                onInput={(event) => setRenameValue(event.currentTarget.value)}
              />
            </div>
          </div>
        </Modal>
      </Show>

      <Show when={deleteTarget()}>
        {(room) => (
          <Modal
            title="Delete room"
            onClose={() => setDeleteTarget(null)}
            footer={
              <>
                <button class="button is-danger" onClick={() => void confirmDelete()}>
                  Delete permanently
                </button>
                <button class="button" onClick={() => setDeleteTarget(null)}>
                  Cancel
                </button>
              </>
            }
          >
            <p>
              Deleting room <code>{room().id}</code> removes its users,
              subdirectories and messages. This cannot be undone.
            </p>
          </Modal>
        )}
      </Show>
    </section>
  );
}
