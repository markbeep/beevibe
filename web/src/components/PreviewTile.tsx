import {
  createEffect,
  createSignal,
  on,
  onCleanup,
  Show,
  type JSX,
} from "solid-js";
import { FaSolidMicrophone, FaSolidMicrophoneSlash, FaSolidRotate } from "solid-icons/fa";
import { api } from "../lib/api";
import type { AgentState, User } from "../lib/types";

const STATE_LABEL: Record<AgentState, string> = {
  idle: "idle",
  queued: "queued",
  thinking: "thinking",
  editing: "editing",
  error: "error",
};

const ageFormat = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

export interface PreviewTileProps {
  roomId: string;
  user: User;
  helpPending: boolean;
  onOpenDrawer: () => void;
  onDismissHelp: () => void;
}

// A server-rendered preview tile (AOV-1). The ETag is kept in component memory
// and re-sent as an explicit If-None-Match on every fetch, because the
// `Cache-Control: no-store` response means the browser owns no cache entry and
// would never revalidate by itself (decision D1).
export function PreviewTile(props: PreviewTileProps): JSX.Element {
  const [src, setSrc] = createSignal<string | null>(null);
  const [fetchedAt, setFetchedAt] = createSignal<number | null>(null);
  const [now, setNow] = createSignal(Date.now());

  let etag: string | null = null;
  let objectUrl: string | null = null;
  let disposed = false;
  const timers: number[] = [];

  const load = async () => {
    try {
      const res = await fetch(
        `/api/rooms/${props.roomId}/users/${props.user.id}/preview`,
        {
          credentials: "same-origin",
          headers: etag ? { "If-None-Match": etag } : {},
        },
      );
      if (disposed) return;
      if (res.status === 304) return; // unchanged — keep the current image
      if (res.status === 404) {
        setSrc(null);
        return;
      }
      if (!res.ok) return;
      const blob = await res.blob();
      if (disposed) return;
      const url = URL.createObjectURL(blob);
      if (objectUrl) URL.revokeObjectURL(objectUrl);
      objectUrl = url;
      etag = res.headers.get("ETag");
      setFetchedAt(Date.now());
      setSrc(url);
    } catch {
      // renderer/network hiccup: keep the last image or the colour fallback
    }
  };

  const refresh = async () => {
    try {
      await api.post(
        `/api/rooms/${props.roomId}/users/${props.user.id}/preview/refresh`,
      );
    } catch {
      // a 202 is expected; failures fall through to the retries below
    }
    for (const delay of [1200, 2800, 5000]) {
      timers.push(window.setTimeout(() => void load(), delay));
    }
  };

  createEffect(
    on(
      () => props.user,
      () => void load(),
    ),
  );

  const ageTimer = window.setInterval(() => setNow(Date.now()), 15000);
  onCleanup(() => {
    disposed = true;
    window.clearInterval(ageTimer);
    for (const timer of timers) window.clearTimeout(timer);
    if (objectUrl) URL.revokeObjectURL(objectUrl);
  });

  // The one identity colour in the system (DESIGN.md): the golden-angle hue
  // wheel, muted so a wall of tiles still sits inside the Sepia Slate board.
  const colour = () => `hsl(${(props.user.id * 137.508) % 360}, 48%, 40%)`;

  const age = () => {
    const at = fetchedAt();
    if (at === null) return "no preview";
    const seconds = Math.round((now() - at) / 1000);
    if (seconds < 45) return "just now";
    if (seconds < 3600) return ageFormat.format(-Math.round(seconds / 60), "minute");
    return ageFormat.format(-Math.round(seconds / 3600), "hour");
  };

  const openSite = () => {
    window.open(`/rooms/${props.roomId}/${props.user.id}/`, "_blank");
  };

  return (
    <div
      class={`preview-tile ${props.user.online ? "" : "is-offline"} ${
        props.helpPending ? "has-help" : ""
      }`}
    >
      <div class="tile-preview" onClick={openSite}>
        <Show
          when={src()}
          fallback={
            <div class="tile-fallback" style={{ background: colour() }}>
              <span>{props.user.name}</span>
            </div>
          }
        >
          {(url) => <img src={url()} alt={props.user.name} />}
        </Show>
        <div class="tile-actions">
          <button
            class="tile-chat"
            type="button"
            title="Open chat and site drawer"
            onClick={(event) => {
              event.stopPropagation();
              props.onOpenDrawer();
            }}
          >
            Chat
          </button>
          <button
            class="tile-refresh"
            type="button"
            title="Refresh preview"
            onClick={(event) => {
              event.stopPropagation();
              void refresh();
            }}
          >
            <FaSolidRotate />
          </button>
        </div>
        <Show when={props.helpPending}>
          <button
            class="tile-help"
            type="button"
            onClick={(event) => {
              event.stopPropagation();
              props.onDismissHelp();
            }}
          >
            help
          </button>
        </Show>
      </div>

      <div class="tile-meta" onClick={() => props.onOpenDrawer()}>
        <div class="tile-name-row">
          <span class="tile-name">{props.user.name}</span>
          <Show when={!props.user.online}>
            <span class="tag is-dark">offline</span>
          </Show>
          <Show when={props.user.micOn}>
            <span class="tile-icon is-mic" title="microphone on">
              <FaSolidMicrophone />
            </span>
          </Show>
          <Show when={!props.user.micOn}>
            <span class="tile-icon is-muted" title="microphone off">
              <FaSolidMicrophoneSlash />
            </span>
          </Show>
        </div>
        <div class="tile-stats">
          <span class={`state state-${props.user.agentState}`}>
            {STATE_LABEL[props.user.agentState]}
          </span>
          <Show when={props.user.queueDepth > 0}>
            <span class="tile-queue">+{props.user.queueDepth} queued</span>
          </Show>
          <span>
            {props.user.tokensUsed} tok
            {props.user.tokenLimit !== null ? ` / ${props.user.tokenLimit}` : ""}
          </span>
          <span class="tile-age">{age()}</span>
        </div>
      </div>
    </div>
  );
}
