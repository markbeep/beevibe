import {
  createMemo,
  createSignal,
  For,
  onMount,
  Show,
  type JSX,
} from "solid-js";
import { useParams } from "@solidjs/router";
import { FaSolidCopy, FaSolidPlus, FaSolidTrash } from "solid-icons/fa";
import { api, ApiError } from "../lib/api";
import type { ResetProgressFrame, Room, RoomState, User } from "../lib/types";
import { Modal } from "../components/Modal";
import { useAdminSocket } from "../session";

const STATE_LABEL: Record<RoomState, string> = {
  open: "open",
  started: "started",
  closed: "closed",
  archived: "archived",
};

const TRANSITIONS: Record<RoomState, { label: string; action: string }[]> = {
  open: [{ label: "Start", action: "start" }],
  started: [{ label: "Close", action: "close" }],
  closed: [
    { label: "Reopen", action: "reopen" },
    { label: "Archive", action: "archive" },
  ],
  archived: [],
};

const MODELS = ["deepseek-flash", "deepseek-chat"];

export default function AdminRoom(): JSX.Element {
  const params = useParams<{ roomId: string }>();
  const socket = useAdminSocket();
  const roomId = () => params.roomId;

  const [room, setRoom] = createSignal<Room | null>(null);
  const [users, setUsers] = createSignal<User[]>([]);
  const [error, setError] = createSignal<string | null>(null);
  const [filter, setFilter] = createSignal("");
  const [selected, setSelected] = createSignal<Set<number>>(new Set());
  const [copiedId, setCopiedId] = createSignal<number | null>(null);
  const [revealedTokenId, setRevealedTokenId] = createSignal<number | null>(null);

  const [newName, setNewName] = createSignal("");
  const [defaultLimit, setDefaultLimit] = createSignal("");

  const [renameOpen, setRenameOpen] = createSignal(false);
  const [renameValue, setRenameValue] = createSignal("");
  const [deleteRoomOpen, setDeleteRoomOpen] = createSignal(false);

  const [templateFiles, setTemplateFiles] = createSignal<File[]>([]);
  const [templateConfirm, setTemplateConfirm] = createSignal(false);
  const [templateBusy, setTemplateBusy] = createSignal(false);
  const [progress, setProgress] = createSignal<ResetProgressFrame | null>(null);

  const [messageTarget, setMessageTarget] = createSignal<User | null>(null);
  const [messageText, setMessageText] = createSignal("");
  const [limitTarget, setLimitTarget] = createSignal<User | null>(null);
  const [limitValue, setLimitValue] = createSignal("");
  const [limitUnlimited, setLimitUnlimited] = createSignal(false);
  const [resetTarget, setResetTarget] = createSignal<User | null>(null);
  const [deleteUserTarget, setDeleteUserTarget] = createSignal<User | null>(null);

  const [bulkMessageOpen, setBulkMessageOpen] = createSignal(false);
  const [bulkMessageText, setBulkMessageText] = createSignal("");
  const [bulkLimitOpen, setBulkLimitOpen] = createSignal(false);
  const [bulkLimitValue, setBulkLimitValue] = createSignal("");
  const [bulkLimitUnlimited, setBulkLimitUnlimited] = createSignal(true);
  const [bulkConfirm, setBulkConfirm] = createSignal<
    "kick" | "delete" | "reset" | null
  >(null);

  let fileInput: HTMLInputElement | undefined;

  const report = (err: unknown, fallback: string) => {
    setError(err instanceof ApiError ? err.message : fallback);
  };

  const loadRoom = async () => {
    try {
      const body = (await api.get(`/api/rooms/${roomId()}`)) as { room: Room };
      setRoom(body.room);
    } catch (err) {
      report(err, "Could not load the room.");
    }
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

  onMount(() => {
    void loadRoom();
    void loadUsers();
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
      } else if (frame.type === "room.update") {
        setRoom(frame.room);
      } else if (frame.type === "reset.progress") {
        setProgress(frame);
        if (!frame.running) {
          setTemplateBusy(false);
          void loadRoom();
          void loadUsers();
        }
      } else if (frame.type === "hello") {
        void loadRoom();
        void loadUsers();
      }
    }),
  );

  const filtered = createMemo(() => {
    const needle = filter().trim().toLowerCase();
    if (!needle) return users();
    return users().filter((user) => user.name.toLowerCase().includes(needle));
  });

  const allFilteredSelected = createMemo(() => {
    const list = filtered();
    return list.length > 0 && list.every((user) => selected().has(user.id));
  });

  const toggleUser = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleAllFiltered = () => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allFilteredSelected()) {
        for (const user of filtered()) next.delete(user.id);
      } else {
        for (const user of filtered()) next.add(user.id);
      }
      return next;
    });
  };

  const runRoomAction = async (action: string) => {
    try {
      await api.post(`/api/rooms/${roomId()}/${action}`);
      void loadRoom();
    } catch (err) {
      report(err, "Action failed.");
    }
  };

  const saveRename = async () => {
    try {
      await api.patch(`/api/rooms/${roomId()}`, {
        name: renameValue().trim() || null,
      });
      setRenameOpen(false);
      void loadRoom();
    } catch (err) {
      report(err, "Rename failed.");
    }
  };

  const saveModel = async (model: string) => {
    try {
      await api.patch(`/api/rooms/${roomId()}`, { model });
      void loadRoom();
    } catch (err) {
      report(err, "Could not change the model.");
    }
  };

  const deleteRoom = async () => {
    try {
      await api.del(`/api/rooms/${roomId()}`);
      window.location.assign("/admin");
    } catch (err) {
      report(err, "Delete failed.");
    }
  };

  const createUser = async () => {
    const name = newName().trim();
    if (!name) return;
    const rawLimit = defaultLimit().trim();
    const tokenLimit = rawLimit === "" ? null : Number(rawLimit);
    try {
      await api.post(`/api/rooms/${roomId()}/users`, { name, tokenLimit });
      setNewName("");
      void loadUsers();
      void loadRoom();
    } catch (err) {
      report(err, "Could not create the user.");
    }
  };

  const copyToken = async (user: User) => {
    try {
      await navigator.clipboard.writeText(user.token);
      setCopiedId(user.id);
      window.setTimeout(() => setCopiedId(null), 1500);
    } catch {
      report(null, "Clipboard unavailable — copy the token manually.");
    }
  };

  const kickUser = async (user: User) => {
    try {
      await api.post(`/api/rooms/${roomId()}/users/${user.id}/kick`);
      void loadUsers();
    } catch (err) {
      report(err, "Kick failed.");
    }
  };

  const cancelUser = async (user: User) => {
    try {
      await api.post(`/api/rooms/${roomId()}/users/${user.id}/cancel`);
      void loadUsers();
    } catch (err) {
      report(err, "Nothing to cancel.");
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

  const confirmReset = async () => {
    const user = resetTarget();
    if (!user) return;
    try {
      await api.post(`/api/rooms/${roomId()}/users/${user.id}/reset`);
      setResetTarget(null);
      void loadUsers();
    } catch (err) {
      report(err, "Reset failed.");
    }
  };

  const confirmDeleteUser = async () => {
    const user = deleteUserTarget();
    if (!user) return;
    try {
      await api.del(`/api/rooms/${roomId()}/users/${user.id}`);
      setDeleteUserTarget(null);
      setUsers((prev) => prev.filter((item) => item.id !== user.id));
      void loadRoom();
    } catch (err) {
      report(err, "Delete failed.");
    }
  };

  const sendTargeted = async () => {
    const user = messageTarget();
    const text = messageText().trim();
    if (!user || !text) return;
    try {
      await api.post(`/api/rooms/${roomId()}/users/bulk`, {
        userIds: [user.id],
        action: "message",
        text,
      });
      setMessageTarget(null);
      setMessageText("");
    } catch (err) {
      report(err, "Message failed.");
    }
  };

  const saveLimit = async () => {
    const user = limitTarget();
    if (!user) return;
    const tokenLimit = limitUnlimited() ? null : Number(limitValue());
    try {
      const body = (await api.patch(
        `/api/rooms/${roomId()}/users/${user.id}`,
        { tokenLimit },
      )) as { user: User };
      setUsers((prev) =>
        prev.map((item) => (item.id === body.user.id ? body.user : item)),
      );
      setLimitTarget(null);
    } catch (err) {
      report(err, "Could not set the limit.");
    }
  };

  const selectedIds = () => [...selected()];

  const runBulk = async (
    action: string,
    extra: { text?: string; tokenLimit?: number | null } = {},
    clearSelection = true,
  ) => {
    const ids = selectedIds();
    if (ids.length === 0) return;
    try {
      await api.post(`/api/rooms/${roomId()}/users/bulk`, {
        userIds: ids,
        action,
        ...extra,
      });
      if (clearSelection) setSelected(new Set<number>());
      void loadUsers();
      void loadRoom();
    } catch (err) {
      report(err, "Bulk action failed.");
    }
  };

  const confirmBulk = async () => {
    const action = bulkConfirm();
    if (!action) return;
    setBulkConfirm(null);
    await runBulk(action);
  };

  const sendBulkMessage = async () => {
    const text = bulkMessageText().trim();
    if (!text) return;
    setBulkMessageOpen(false);
    setBulkMessageText("");
    await runBulk("message", { text });
  };

  const saveBulkLimit = async () => {
    const tokenLimit = bulkLimitUnlimited() ? null : Number(bulkLimitValue());
    setBulkLimitOpen(false);
    await runBulk("set-token-limit", { tokenLimit });
  };

  const onPickTemplate = (event: Event) => {
    const input = event.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    if (files.length === 0) return;
    setTemplateFiles(files);
    setTemplateConfirm(true);
    input.value = "";
  };

  const uploadTemplate = async () => {
    const files = templateFiles();
    if (files.length === 0) return;
    const form = new FormData();
    for (const file of files) {
      const relative =
        (file as File & { webkitRelativePath?: string }).webkitRelativePath ||
        file.name;
      form.append("files", file, relative);
    }
    setTemplateConfirm(false);
    setTemplateBusy(true);
    setError(null);
    try {
      await api.upload(`/api/rooms/${roomId()}/template`, form);
      setTemplateFiles([]);
      void loadRoom();
    } catch (err) {
      setTemplateBusy(false);
      report(err, "Template upload rejected.");
    }
  };

  const deleteTemplate = async () => {
    try {
      await api.del(`/api/rooms/${roomId()}/template`);
      void loadRoom();
    } catch (err) {
      report(err, "Could not delete the template.");
    }
  };

  const clearTemplateSelection = () => {
    setTemplateConfirm(false);
    setTemplateFiles([]);
  };

  const canDeleteRoom = () => room()?.state === "archived";

  const progressPercent = () => {
    const current = progress();
    if (!current || current.total === 0) return 0;
    return Math.round((current.done / current.total) * 100);
  };

  return (
    <section class="admin-page">
      <Show when={room()} fallback={<p class="has-text-grey">Loading room…</p>}>
        {(current) => (
          <>
            <header class="page-head">
              <div>
                <h1 class="title is-4">
                  {current().name || "Untitled room"}{" "}
                  <span class="tag is-light">{STATE_LABEL[current().state]}</span>
                </h1>
                <p class="subtitle is-6">
                  <code>{current().id}</code> · {current().userCount} users
                </p>
              </div>
              <div class="page-head-actions">
                <button
                  class="button"
                  onClick={() => {
                    setRenameValue(current().name ?? "");
                    setRenameOpen(true);
                  }}
                >
                  Rename
                </button>
                <For each={TRANSITIONS[current().state]}>
                  {(transition) => (
                    <button
                      class="button"
                      onClick={() => void runRoomAction(transition.action)}
                    >
                      {transition.label}
                    </button>
                  )}
                </For>
                <Show when={canDeleteRoom()}>
                  <button
                    class="button is-danger"
                    onClick={() => setDeleteRoomOpen(true)}
                  >
                    Delete room
                  </button>
                </Show>
                <a class="button is-link" href={`/admin/rooms/${roomId()}/live`}>
                  Live overview
                </a>
              </div>
            </header>

            <Show when={error()}>
              <div class="notification is-danger is-light">{error()}</div>
            </Show>

            <div class="room-config box">
              <div class="field is-horizontal">
                <div class="field-label is-small">
                  <label class="label">Agent model</label>
                </div>
                <div class="field-body">
                  <div class="field">
                    <div class="control">
                      <div class="select is-small">
                        <select
                          value={current().model}
                          onChange={(event) =>
                            void saveModel(event.currentTarget.value)
                          }
                        >
                          <For each={MODELS}>
                            {(model) => <option value={model}>{model}</option>}
                          </For>
                        </select>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="box template-panel">
              <h2 class="title is-6">Subdirectory template</h2>
              <p class="is-size-7 has-text-grey">
                Uploading or replacing re-seeds every user's subdirectory, wipes
                their files and agent history, and locks the room until it
                completes.
              </p>
              <Show when={progress()}>
                {(current) => (
                  <div class="template-progress">
                    <progress
                      class="progress is-small is-link"
                      max={Math.max(1, current().total)}
                      value={current().done}
                    >
                      {progressPercent()}%
                    </progress>
                    <span class="is-size-7">
                      {current().done}/{current().total} users reset
                    </span>
                  </div>
                )}
              </Show>
              <div class="buttons">
                <button
                  class="button"
                  disabled={templateBusy()}
                  onClick={() => fileInput?.click()}
                >
                  Upload template
                </button>
                <Show when={current().hasTemplate}>
                  <button
                    class="button is-danger is-light"
                    disabled={templateBusy()}
                    onClick={() => void deleteTemplate()}
                  >
                    <FaSolidTrash />
                    <span>Delete template</span>
                  </button>
                </Show>
                <input
                  ref={fileInput}
                  type="file"
                  multiple
                  hidden
                  onChange={onPickTemplate}
                />
              </div>
            </div>

            <div class="box">
              <div class="user-toolbar">
                <div class="field has-addons">
                  <div class="control">
                    <input
                      class="input is-small"
                      type="text"
                      placeholder="New user name"
                      value={newName()}
                      onInput={(event) => setNewName(event.currentTarget.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") void createUser();
                      }}
                    />
                  </div>
                  <div class="control">
                    <input
                      class="input is-small"
                      type="number"
                      min="0"
                      placeholder="limit"
                      value={defaultLimit()}
                      onInput={(event) =>
                        setDefaultLimit(event.currentTarget.value)
                      }
                    />
                  </div>
                  <div class="control">
                    <button class="button is-small is-primary" onClick={() => void createUser()}>
                      <FaSolidPlus />
                      <span>Add user</span>
                    </button>
                  </div>
                </div>

                <div class="field">
                  <div class="control">
                    <input
                      class="input is-small"
                      type="search"
                      placeholder="Filter by name"
                      value={filter()}
                      onInput={(event) => setFilter(event.currentTarget.value)}
                    />
                  </div>
                </div>

                <a class="button is-small" href={`/api/rooms/${roomId()}/users.csv`}>
                  Export CSV
                </a>
              </div>

              <Show when={selected().size > 0}>
                <div class="bulk-bar notification is-info is-light">
                  <span>{selected().size} selected</span>
                  <div class="buttons are-small">
                    <button class="button" onClick={() => setBulkConfirm("kick")}>
                      Kick
                    </button>
                    <button class="button" onClick={() => void runBulk("reset")}>
                      Reset
                    </button>
                    <button
                      class="button"
                      onClick={() => {
                        setBulkMessageText("");
                        setBulkMessageOpen(true);
                      }}
                    >
                      Message
                    </button>
                    <button
                      class="button"
                      onClick={() => {
                        setBulkLimitValue("");
                        setBulkLimitUnlimited(true);
                        setBulkLimitOpen(true);
                      }}
                    >
                      Set limit
                    </button>
                    <button
                      class="button is-danger"
                      onClick={() => setBulkConfirm("delete")}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              </Show>

              <table class="table is-fullwidth is-narrow">
                <thead>
                  <tr>
                    <th>
                      <input
                        type="checkbox"
                        checked={allFilteredSelected()}
                        onChange={toggleAllFiltered}
                      />
                    </th>
                    <th>Name</th>
                    <th>Token</th>
                    <th>State</th>
                    <th>Tokens</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={filtered()}>
                    {(user) => (
                      <tr class={user.online ? "" : "is-offline-row"}>
                        <td>
                          <input
                            type="checkbox"
                            checked={selected().has(user.id)}
                            onChange={() => toggleUser(user.id)}
                          />
                        </td>
                        <td>
                          <span class={`online-dot ${user.online ? "is-on" : ""}`} />
                          {user.name}
                        </td>
                        <td>
                          <button
                            type="button"
                            class="token-cell token-mask"
                            title={
                              revealedTokenId() === user.id
                                ? "Hide token"
                                : "Reveal token"
                            }
                            onClick={() =>
                              setRevealedTokenId((prev) =>
                                prev === user.id ? null : user.id,
                              )
                            }
                          >
                            {revealedTokenId() === user.id
                              ? user.token
                              : "••••••••"}
                          </button>
                          <button
                            class="button is-small is-ghost"
                            title="Copy token"
                            onClick={() => void copyToken(user)}
                          >
                            {copiedId() === user.id ? "copied" : <FaSolidCopy />}
                          </button>
                        </td>
                        <td>
                          <span class="state-label">{user.agentState}</span>
                          <Show when={user.queueDepth > 0}>
                            <span class="tag is-warning is-light">
                              +{user.queueDepth}
                            </span>
                          </Show>
                          <Show when={user.helpPending}>
                            <span class="tag is-link">help</span>
                          </Show>
                        </td>
                        <td>
                          {user.tokensUsed}
                          {user.tokenLimit !== null
                            ? ` / ${user.tokenLimit}`
                            : " / unlimited"}
                        </td>
                        <td>
                          <div class="buttons are-small">
                            <a
                              class="button"
                              href={`/admin/rooms/${roomId()}/live?user=${user.id}`}
                            >
                              Chat
                            </a>
                            <Show when={user.helpPending}>
                              <button
                                class="button is-warning"
                                onClick={() => void clearHelp(user)}
                              >
                                Clear help
                              </button>
                            </Show>
                            <button
                              class="button"
                              disabled={
                                user.agentState === "idle" &&
                                user.queueDepth === 0
                              }
                              onClick={() => void cancelUser(user)}
                            >
                              Cancel
                            </button>
                            <button class="button" onClick={() => void kickUser(user)}>
                              Kick
                            </button>
                            <button
                              class="button"
                              onClick={() => setResetTarget(user)}
                            >
                              Reset
                            </button>
                            <button
                              class="button"
                              onClick={() => {
                                setMessageText("");
                                setMessageTarget(user);
                              }}
                            >
                              Message
                            </button>
                            <button
                              class="button"
                              onClick={() => {
                                setLimitValue(
                                  user.tokenLimit !== null
                                    ? String(user.tokenLimit)
                                    : "",
                                );
                                setLimitUnlimited(user.tokenLimit === null);
                                setLimitTarget(user);
                              }}
                            >
                              Limit
                            </button>
                            <button
                              class="button is-danger is-light"
                              onClick={() => setDeleteUserTarget(user)}
                            >
                              Delete
                            </button>
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>

              <Show when={filtered().length === 0}>
                <p class="has-text-grey">No users match this filter.</p>
              </Show>
            </div>
          </>
        )}
      </Show>

      <Show when={renameOpen()}>
        <Modal
          title="Rename room"
          onClose={() => setRenameOpen(false)}
          footer={
            <>
              <button class="button is-primary" onClick={() => void saveRename()}>
                Save
              </button>
              <button class="button" onClick={() => setRenameOpen(false)}>
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

      <Show when={deleteRoomOpen()}>
        <Modal
          title="Delete room"
          onClose={() => setDeleteRoomOpen(false)}
          footer={
            <>
              <button class="button is-danger" onClick={() => void deleteRoom()}>
                Delete permanently
              </button>
              <button class="button" onClick={() => setDeleteRoomOpen(false)}>
                Cancel
              </button>
            </>
          }
        >
          <p>
            This removes every user, subdirectory and message in the room. It
            cannot be undone.
          </p>
        </Modal>
      </Show>

      <Show when={templateConfirm()}>
        <Modal
          title="Replace template"
          onClose={clearTemplateSelection}
          footer={
            <>
              <button class="button is-danger" onClick={() => void uploadTemplate()}>
                Upload and re-seed
              </button>
              <button class="button" onClick={clearTemplateSelection}>
                Cancel
              </button>
            </>
          }
        >
          <p>
            {templateFiles().length} file(s) uploaded.{" "}
            {room()?.userCount ?? 0} users will lose their current files and
            agent memory.
          </p>
        </Modal>
      </Show>

      <Show when={messageTarget()}>
        {(user) => (
          <Modal
            title={`Message ${user().name}`}
            onClose={() => setMessageTarget(null)}
            footer={
              <>
                <button class="button is-primary" onClick={() => void sendTargeted()}>
                  Send
                </button>
                <button class="button" onClick={() => setMessageTarget(null)}>
                  Cancel
                </button>
              </>
            }
          >
            <div class="field">
              <div class="control">
                <textarea
                  class="textarea"
                  value={messageText()}
                  onInput={(event) => setMessageText(event.currentTarget.value)}
                />
              </div>
            </div>
          </Modal>
        )}
      </Show>

      <Show when={limitTarget()}>
        {(user) => (
          <Modal
            title={`Token limit — ${user().name}`}
            onClose={() => setLimitTarget(null)}
            footer={
              <>
                <button class="button is-primary" onClick={() => void saveLimit()}>
                  Save
                </button>
                <button class="button" onClick={() => setLimitTarget(null)}>
                  Cancel
                </button>
              </>
            }
          >
            <div class="field">
              <label class="checkbox">
                <input
                  type="checkbox"
                  checked={limitUnlimited()}
                  onChange={(event) =>
                    setLimitUnlimited(event.currentTarget.checked)
                  }
                />
                Unlimited (clears the limit and resets usage)
              </label>
            </div>
            <div class="field">
              <label class="label">Limit</label>
              <div class="control">
                <input
                  class="input"
                  type="number"
                  min="0"
                  disabled={limitUnlimited()}
                  value={limitValue()}
                  onInput={(event) => setLimitValue(event.currentTarget.value)}
                />
              </div>
            </div>
          </Modal>
        )}
      </Show>

      <Show when={resetTarget()}>
        {(user) => (
          <Modal
            title="Reset subdirectory"
            onClose={() => setResetTarget(null)}
            footer={
              <>
                <button class="button is-danger" onClick={() => void confirmReset()}>
                  Reset
                </button>
                <button class="button" onClick={() => setResetTarget(null)}>
                  Cancel
                </button>
              </>
            }
          >
            <p>
              {user().name}'s files and agent history are deleted and re-seeded
              from the template. The user is locked out until it completes.
            </p>
          </Modal>
        )}
      </Show>

      <Show when={deleteUserTarget()}>
        {(user) => (
          <Modal
            title="Delete user"
            onClose={() => setDeleteUserTarget(null)}
            footer={
              <>
                <button
                  class="button is-danger"
                  onClick={() => void confirmDeleteUser()}
                >
                  Delete permanently
                </button>
                <button class="button" onClick={() => setDeleteUserTarget(null)}>
                  Cancel
                </button>
              </>
            }
          >
            <p>
              {user().name}'s token is invalidated, their subdirectory and
              history are removed. This cannot be undone.
            </p>
          </Modal>
        )}
      </Show>

      <Show when={bulkMessageOpen()}>
        <Modal
          title={`Message ${selected().size} users`}
          onClose={() => setBulkMessageOpen(false)}
          footer={
            <>
              <button class="button is-primary" onClick={() => void sendBulkMessage()}>
                Send
              </button>
              <button class="button" onClick={() => setBulkMessageOpen(false)}>
                Cancel
              </button>
            </>
          }
        >
          <div class="field">
            <div class="control">
              <textarea
                class="textarea"
                value={bulkMessageText()}
                onInput={(event) => setBulkMessageText(event.currentTarget.value)}
              />
            </div>
          </div>
        </Modal>
      </Show>

      <Show when={bulkLimitOpen()}>
        <Modal
          title={`Token limit — ${selected().size} users`}
          onClose={() => setBulkLimitOpen(false)}
          footer={
            <>
              <button class="button is-primary" onClick={() => void saveBulkLimit()}>
                Save
              </button>
              <button class="button" onClick={() => setBulkLimitOpen(false)}>
                Cancel
              </button>
            </>
          }
        >
          <div class="field">
            <label class="checkbox">
              <input
                type="checkbox"
                checked={bulkLimitUnlimited()}
                onChange={(event) =>
                  setBulkLimitUnlimited(event.currentTarget.checked)
                }
              />
              Unlimited
            </label>
          </div>
          <div class="field">
            <label class="label">Limit</label>
            <div class="control">
              <input
                class="input"
                type="number"
                min="0"
                disabled={bulkLimitUnlimited()}
                value={bulkLimitValue()}
                onInput={(event) => setBulkLimitValue(event.currentTarget.value)}
              />
            </div>
          </div>
        </Modal>
      </Show>

      <Show when={bulkConfirm()}>
        <Modal
          title={bulkConfirm() === "delete" ? "Delete users" : "Kick users"}
          onClose={() => setBulkConfirm(null)}
          footer={
            <>
              <button
                class={`button ${
                  bulkConfirm() === "delete" ? "is-danger" : "is-primary"
                }`}
                onClick={() => void confirmBulk()}
              >
                {bulkConfirm() === "delete" ? "Delete permanently" : "Kick"}
              </button>
              <button class="button" onClick={() => setBulkConfirm(null)}>
                Cancel
              </button>
            </>
          }
        >
          <p>
            This applies to {selected().size} selected user(s).
            {bulkConfirm() === "delete"
              ? " Their tokens, subdirectories and history are removed permanently."
              : ""}
          </p>
        </Modal>
      </Show>

    </section>
  );
}
