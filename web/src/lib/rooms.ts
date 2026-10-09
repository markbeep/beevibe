// Shared room vocabulary. The room lifecycle is one state machine, so its tag
// colours and its legal transitions live here once and are read by the room
// list, the settings page and the live overview alike.
import type { RoomState } from "./types";

export const STATE_TAG: Record<RoomState, string> = {
  open: "tag is-info",
  started: "tag is-success",
  closed: "tag is-warning",
  archived: "tag is-dark",
};

export const TRANSITIONS: Record<RoomState, { label: string; action: string }[]> = {
  open: [{ label: "Start", action: "start" }],
  started: [{ label: "Close", action: "close" }],
  closed: [
    { label: "Reopen", action: "reopen" },
    { label: "Archive", action: "archive" },
  ],
  archived: [],
};
