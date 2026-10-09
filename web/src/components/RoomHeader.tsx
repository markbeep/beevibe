import { Show, type JSX } from "solid-js";
import { FaSolidArrowLeft } from "solid-icons/fa";
import type { RoomState } from "../lib/types";

export interface RoomHeaderProps {
  roomId: string;
  name: string;
  /** Absent while the room is still loading. */
  state?: RoomState;
  userCount: number;
  view: "settings" | "overview";
  /** The room's own controls. The view switch is always pinned to their right. */
  children?: JSX.Element;
}

/**
 * The admin header for a single room. Both room-scoped views render it, so the
 * way back to the room list and the Settings/Overview switch sit in the same
 * place whichever view you are on.
 */
export function RoomHeader(props: RoomHeaderProps): JSX.Element {
  const base = () => `/admin/rooms/${props.roomId}`;

  return (
    <header class="room-head">
      <a class="room-back" href="/admin">
        <FaSolidArrowLeft />
        <span>All rooms</span>
      </a>

      <div class="room-head-row">
        <div class="room-head-title">
          <h1 class="title is-4">
            {props.name}{" "}
            <Show when={props.state}>
              {(state) => <span class="tag is-light">{state()}</span>}
            </Show>
          </h1>
          <p class="subtitle is-6">
            <code>{props.roomId}</code> · {props.userCount} users
          </p>
        </div>

        <div class="room-head-actions">
          {props.children}
          <nav class="room-switch" aria-label="Room view">
            <a
              href={base()}
              class="room-switch-item"
              classList={{ "is-active": props.view === "settings" }}
              aria-current={props.view === "settings" ? "page" : undefined}
            >
              Settings
            </a>
            <a
              href={`${base()}/live`}
              class="room-switch-item"
              classList={{ "is-active": props.view === "overview" }}
              aria-current={props.view === "overview" ? "page" : undefined}
            >
              Overview
            </a>
          </nav>
        </div>
      </div>
    </header>
  );
}
