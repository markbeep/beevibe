import { createEffect, For, type JSX } from "solid-js";
import type { Message, MessageKind } from "../lib/types";

const ENTRY_CLASS: Record<MessageKind, string> = {
  transcript: "chat-entry chat-transcript",
  agent_status: "chat-entry chat-status",
  admin: "chat-entry chat-admin",
  system: "chat-entry chat-system",
  help: "chat-entry chat-help",
};

const clock = new Intl.DateTimeFormat(undefined, {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

export interface ChatPanelProps {
  messages: Message[];
  viewer: "user" | "admin";
  // Shown for the viewer's own messages in the admin drawer (never "You").
  userName?: string;
}

// Who each entry is from, from the viewer's perspective.
function senderLabel(
  kind: MessageKind,
  viewer: "user" | "admin",
  userName: string | undefined,
): string {
  switch (kind) {
    case "system":
      return "System";
    case "agent_status":
      return "Agent";
    case "admin":
      return "Admin";
    default:
      // transcript / help are the user's own entries.
      return viewer === "user" ? "You" : (userName?.trim() || "User");
  }
}

export function ChatPanel(props: ChatPanelProps): JSX.Element {
  let scroller: HTMLDivElement | undefined;
  let stick = true;

  const onScroll = () => {
    if (!scroller) return;
    // Pause auto-scroll once the user leaves the bottom by more than 40 px.
    stick =
      scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 40;
  };

  createEffect(() => {
    // Track length so the effect re-runs on every appended entry.
    const count = props.messages.length;
    if (!scroller || count === 0 || !stick) return;
    scroller.scrollTop = scroller.scrollHeight;
  });

  return (
    <div class="chat-panel" ref={scroller} onScroll={onScroll}>
      <For each={props.messages}>
        {(message) => (
          <div class={ENTRY_CLASS[message.kind] ?? "chat-entry"}>
            <span class="chat-time">
              {clock.format(new Date(message.at * 1000))}
            </span>
            <div class="chat-body">
              <span class="chat-sender">
                {senderLabel(message.kind, props.viewer, props.userName)}
              </span>
              <span class="chat-text">{message.text}</span>
            </div>
          </div>
        )}
      </For>
    </div>
  );
}
