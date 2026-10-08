// Wire types — mirror plans/api.md exactly. Field names, nullability and
// epoch-seconds timestamps are the frozen contract; do not "improve" them.

export type RoomState = "open" | "started" | "closed" | "archived";

export type AgentState = "idle" | "queued" | "thinking" | "editing" | "error";

export type MessageKind =
  | "transcript"
  | "agent_status"
  | "admin"
  | "system"
  | "help";

export type PreviewState = "ok" | "missing" | "failed";

export type Role = "admin" | "user";

export interface Room {
  id: string;
  name: string | null;
  state: RoomState;
  model: string;
  userCount: number;
  hasTemplate: boolean;
  createdAt: number;
  startedAt: number | null;
  closedAt: number | null;
  archivedAt: number | null;
}

export interface User {
  id: number;
  roomId: string;
  name: string;
  token: string;
  createdAt: number;
  kickedAt: number | null;
  tokenLimit: number | null;
  tokensUsed: number;
  online: boolean;
  micOn: boolean;
  helpPending: boolean;
  agentState: AgentState;
  queueDepth: number;
  previewState: PreviewState;
}

export interface Message {
  id: number;
  kind: MessageKind;
  text: string;
  at: number;
}

export interface Stats {
  tokensUsed: number;
  tokenLimit: number | null;
  loc: number;
  sitePath: string;
  agentContextMessages: number;
}

export interface Me {
  role: Role;
  roomId: string | null;
  userId: number | null;
  name: string | null;
  roomState: RoomState | null;
  stats?: Stats;
}

// --- WebSocket frames (server -> client) -----------------------------------

export interface HelloFrame {
  type: "hello";
  role: Role;
  roomId: string | null;
  userId: number | null;
  roomState: RoomState | null;
  agentState: AgentState;
  queueDepth: number;
  blocked: boolean;
}

export interface ChatAppendFrame {
  type: "chat.append";
  message: Message;
  // Additive attribution for the admin channel (decision D3): the drawer shows
  // messages for the drawer user and room-wide (null) messages. When the server
  // omits it the client re-fetches the drawer instead of guessing.
  userId?: number | null;
}

export interface AgentStateFrame {
  type: "agent.state";
  state: AgentState;
  queueDepth: number;
  runId: number | null;
  blocked: boolean;
}

export interface SiteUpdatedFrame {
  type: "site.updated";
  rev: number;
}

export interface RoomStateFrame {
  type: "room.state";
  state: RoomState;
}

export interface UserUpdateFrame {
  type: "user.update";
  user: User;
}

export interface RoomUpdateFrame {
  type: "room.update";
  room: Room;
}

// Decision D2: additive admin message carrying template/bulk reset progress.
export interface ResetProgressFrame {
  type: "reset.progress";
  scope: "template" | "bulk";
  total: number;
  done: number;
  running: boolean;
}

export interface ErrorFrame {
  type: "error";
  code: string;
  message: string;
}

export type ServerFrame =
  | HelloFrame
  | ChatAppendFrame
  | AgentStateFrame
  | SiteUpdatedFrame
  | RoomStateFrame
  | UserUpdateFrame
  | RoomUpdateFrame
  | ResetProgressFrame
  | ErrorFrame;

// The client receives ServerFrames; anything whose `type` is not in the union
// is dropped by the socket (plans/api.md §4: unknown types MUST be ignored).
export type AnyFrame = ServerFrame;

export type FrameType = ServerFrame["type"];
