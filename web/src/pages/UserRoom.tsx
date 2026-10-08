import {
  createMemo,
  createSignal,
  onCleanup,
  onMount,
  Show,
  type JSX,
} from "solid-js";
import { useNavigate, useParams } from "@solidjs/router";
import {
  FaSolidChartSimple,
  FaSolidCircleStop,
  FaSolidComments,
  FaSolidCopy,
  FaSolidGear,
  FaSolidHand,
  FaSolidRightFromBracket,
  FaSolidRotate,
} from "solid-icons/fa";
import { api, ApiError, postAudio } from "../lib/api";
import { Mic } from "../lib/audio";
import { mergeMessages } from "../lib/messages";
import type { AgentState, Message, RoomState, Stats } from "../lib/types";
import { Socket } from "../lib/ws";
import { encodeWAV } from "../lib/wav";
import { ChatPanel } from "../components/ChatPanel";
import { MicButton } from "../components/MicButton";
import { MicSettings } from "../components/MicSettings";
import { Modal } from "../components/Modal";
import { useSession } from "../session";

const MAX_SECONDS = 30;
const MIN_SAMPLES = 1600; // ignore taps shorter than ~0.1 s

const BLOCKED_TEXT: Record<string, string> = {
  resetting: "Resetting your site…",
  limit: "Token limit reached",
  notstarted: "The room hasn't started yet.",
  closed: "The room is closed — editing is disabled.",
};

export default function UserRoom(): JSX.Element {
  const params = useParams<{ roomId: string }>();
  const session = useSession();
  const navigate = useNavigate();

  const roomId = () => params.roomId;
  const userId = () => session.me()?.userId ?? null;

  const mic = new Mic();

  const [roomState, setRoomState] = createSignal<RoomState>(
    session.me()?.roomState ?? "open",
  );
  const [agentState, setAgentState] = createSignal<AgentState>("idle");
  const [queueDepth, setQueueDepth] = createSignal(0);
  const [runId, setRunId] = createSignal<number | null>(null);
  const [wsBlocked, setWsBlocked] = createSignal(false);
  const [stats, setStats] = createSignal<Stats | null>(null);
  const [messages, setMessages] = createSignal<Message[]>([]);
  const [rev, setRev] = createSignal(0);

  const [recording, setRecording] = createSignal(false);
  const [countdown, setCountdown] = createSignal<number | null>(null);
  const [devices, setDevices] = createSignal<MediaDeviceInfo[]>([]);
  const [device, setDevice] = createSignal("");
  const [gain, setGain] = createSignal(1);
  const [level, setLevel] = createSignal(0);

  const [settingsOpen, setSettingsOpen] = createSignal(false);
  const [chatOpen, setChatOpen] = createSignal(false);
  const [statsOpen, setStatsOpen] = createSignal(false);
  const [newSessionOpen, setNewSessionOpen] = createSignal(false);
  const [helpRaised, setHelpRaised] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  let socket: Socket | null = null;
  let pressWanted = false;
  let openPending = false;
  let pressStarted = 0;
  let tickTimer: number | null = null;

  mic.onLevel = (value) => setLevel(value);

  const limitReached = () => {
    const current = stats();
    return (
      current !== null &&
      current.tokenLimit !== null &&
      current.tokensUsed >= current.tokenLimit
    );
  };

  // Blocked-overlay derivation (decision D13): room state first, then limit,
  // otherwise a server-signalled reset.
  const blockedReason = (): keyof typeof BLOCKED_TEXT | null => {
    const state = roomState();
    if (state === "open") return "notstarted";
    if (state === "closed") return "closed";
    if (limitReached()) return "limit";
    if (wsBlocked()) return "resetting";
    return null;
  };

  const blocked = () => blockedReason() !== null;

  const loadStats = async () => {
    try {
      const body = (await api.get("/api/me/stats")) as { stats: Stats };
      setStats(body.stats);
    } catch {
      // stats are advisory; the overlay simply stays as-is
    }
  };

  const loadMessages = async () => {
    try {
      const body = (await api.get("/api/me/messages?limit=50")) as {
        messages: Message[];
      };
      setMessages((prev) => mergeMessages(prev, body.messages));
    } catch {
      // chat reload failures are non-fatal
    }
  };

  const resync = async () => {
    await session.refresh();
    await Promise.all([loadStats(), loadMessages()]);
  };

  const applyHello = (frame: {
    roomState: RoomState | null;
    agentState: AgentState;
    queueDepth: number;
    blocked: boolean;
  }) => {
    if (frame.roomState) setRoomState(frame.roomState);
    setAgentState(frame.agentState);
    setQueueDepth(frame.queueDepth);
    setWsBlocked(frame.blocked);
  };

  const refreshDevices = async () => {
    setDevices(await Mic.listInputs());
  };

  // Warm the capture graph so later presses start instantly: getUserMedia plus
  // the AudioWorklet load otherwise sit on the press critical path. Best-effort
  // — only when the permission is already granted, errors swallowed, and
  // Mic.open is idempotent for the same device.
  const warmMicrophone = async () => {
    if (!navigator.permissions?.query) return;
    try {
      const status = await navigator.permissions.query({
        name: "microphone",
      } as PermissionDescriptor);
      if (status.state === "granted") await mic.open(device(), gain());
    } catch {
      // unsupported or denied: presses acquire the mic on demand
    }
  };

  onMount(() => {
    void refreshDevices();
    void resync();
    void warmMicrophone();

    socket = new Socket("user", {
      onMessage: (frame) => {
        if (frame.type === "agent.state") {
          setAgentState(frame.state);
          setQueueDepth(frame.queueDepth);
          setRunId(frame.runId);
          setWsBlocked(frame.blocked);
          if (frame.state === "idle") void loadStats();
        } else if (frame.type === "chat.append") {
          setMessages((prev) => mergeMessages(prev, [frame.message]));
        } else if (frame.type === "site.updated") {
          setRev((value) => value + 1);
        } else if (frame.type === "room.state") {
          setRoomState(frame.state);
          if (frame.state === "archived") navigate("/", { replace: true });
        } else if (frame.type === "error") {
          setError(frame.message);
        }
      },
      onHello: (frame) => {
        if (frame.type !== "hello") return;
        applyHello(frame);
        void resync();
      },
    });
  });

  onCleanup(() => {
    if (tickTimer !== null) window.clearInterval(tickTimer);
    socket?.close();
    mic.close();
  });

  const sendMicState = (on: boolean) => socket?.send({ type: "mic.state", on });

  const stopTicker = () => {
    if (tickTimer !== null) {
      window.clearInterval(tickTimer);
      tickTimer = null;
    }
  };

  const endPress = async () => {
    if (!recording()) {
      // Released while the microphone was still opening: a quick tap. Say so
      // instead of dropping the press silently.
      if (openPending) {
        setError("That was too short — hold the button while you speak.");
      }
      return;
    }
    setRecording(false);
    setCountdown(null);
    stopTicker();
    sendMicState(false);

    const pcm = mic.end();
    if (!pcm || pcm.length < MIN_SAMPLES) {
      setError("That was too short — hold the button while you speak.");
      return;
    }
    try {
      const body = (await postAudio(
        "/api/stt",
        encodeWAV(pcm),
        "audio/wav",
      )) as { text: string };
      const text = (body.text ?? "").trim();
      if (!text) return;
      await api.post("/api/prompt", { text });
    } catch (err) {
      // 409 (blocked prompt) and 400 (unintelligible/empty STT) already post a
      // canned chat line server-side, so never surface the raw server message.
      if (err instanceof ApiError && (err.status === 409 || err.status === 400)) {
        return;
      }
      setError(
        err instanceof ApiError
          ? err.message
          : "Could not send that — please try again.",
      );
    }
  };

  const tick = () => {
    const elapsed = (Date.now() - pressStarted) / 1000;
    const remaining = MAX_SECONDS - elapsed;
    if (remaining <= 10) setCountdown(Math.max(0, Math.ceil(remaining)));
    if (elapsed >= MAX_SECONDS) void endPress();
  };

  const beginPress = async () => {
    if (blocked() || recording()) return;
    setError(null);
    openPending = true;
    try {
      await mic.open(device(), gain());
    } catch {
      openPending = false;
      setError("Microphone unavailable — check permissions and the input device.");
      return;
    }
    openPending = false;
    if (!pressWanted) return;
    void refreshDevices();
    mic.begin();
    setRecording(true);
    setCountdown(null);
    sendMicState(true);
    pressStarted = Date.now();
    tickTimer = window.setInterval(tick, 200);
  };

  const onStart = () => {
    pressWanted = true;
    void beginPress();
  };

  const onEnd = () => {
    pressWanted = false;
    void endPress();
  };

  const cancelRun = () => socket?.send({ type: "agent.cancel" });

  // The user channel never reports `helpPending`, so the toggle state is local.
  const toggleHelp = () => {
    const next = !helpRaised();
    setHelpRaised(next);
    socket?.send({ type: "help.request", on: next });
  };

  const resetContext = async () => {
    setNewSessionOpen(false);
    try {
      await api.post("/api/me/reset-context");
      setMessages([]);
      await loadMessages();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reset.");
    }
  };

  const leave = async () => {
    await session.logout();
    navigate("/", { replace: true });
  };

  const chooseDevice = (id: string) => {
    setDevice(id);
    void mic.open(id, gain());
  };

  const changeGain = (value: number) => {
    setGain(value);
    mic.setGain(value);
  };

  const siteSrc = createMemo(
    () => `/rooms/${roomId()}/${userId()}/?v=${rev()}`,
  );

  const canCancel = () => runId() !== null || agentState() !== "idle";

  const copySiteUrl = async () => {
    const path = stats()?.sitePath ?? `/rooms/${roomId()}/${userId()}/`;
    try {
      await navigator.clipboard.writeText(`${window.location.origin}${path}`);
    } catch {
      setError("Clipboard unavailable.");
    }
  };

  return (
    <section class="user-room">
      <div class="site-area">
        <Show when={userId() !== null && roomId()}>
          <Show when={siteSrc()} keyed>
            {(src) => (
              <iframe
                class="site-frame"
                sandbox="allow-scripts"
                src={src}
                title="Your site"
              />
            )}
          </Show>
        </Show>

        <Show when={blockedReason()}>
          {(reason) => (
            <div class="blocked-overlay">
              <div class="blocked-card">
                <p>{BLOCKED_TEXT[reason()]}</p>
              </div>
            </div>
          )}
        </Show>

        <button
          class="button is-small site-refresh"
          title="Reload your site"
          onClick={() => setRev((value) => value + 1)}
        >
          <FaSolidRotate />
        </button>
      </div>

      <Show when={error()}>
        <div class="notification is-danger is-light user-error">
          {error()}
        </div>
      </Show>

      <Show when={settingsOpen()}>
        <div class="mic-settings-popover">
          <MicSettings
            devices={devices()}
            device={device()}
            onDevice={chooseDevice}
            gain={gain()}
            onGain={changeGain}
            level={level()}
          />
        </div>
      </Show>

      <Show when={chatOpen()}>
        <div class="user-chat">
          <ChatPanel messages={messages()} viewer="user" />
        </div>
      </Show>

      <footer class="user-bar">
        <MicButton
          recording={recording()}
          disabled={blocked()}
          countdown={countdown()}
          onStart={onStart}
          onEnd={onEnd}
        />
        <button
          class="button"
          classList={{ "is-info": settingsOpen() }}
          title="Microphone settings"
          onClick={() => setSettingsOpen((value) => !value)}
        >
          <FaSolidGear />
          <span>Mic</span>
        </button>
        <button
          class="button"
          disabled={!canCancel()}
          title="Cancel the current prompt"
          onClick={cancelRun}
        >
          <FaSolidCircleStop />
          <span>Cancel</span>
        </button>
        <button class="button" onClick={() => setNewSessionOpen(true)}>
          <FaSolidRotate />
          <span>New session</span>
        </button>
        <button
          class="button"
          classList={{ "is-warning": helpRaised() }}
          onClick={toggleHelp}
        >
          <FaSolidHand />
          <span>{helpRaised() ? "Cancel help" : "Help"}</span>
        </button>
        <button
          class="button"
          classList={{ "is-info": chatOpen() }}
          onClick={() => setChatOpen((value) => !value)}
        >
          <FaSolidComments />
          <span>Chat</span>
        </button>
        <button class="button" onClick={() => setStatsOpen(true)}>
          <FaSolidChartSimple />
          <span>Stats</span>
        </button>
        <button class="button is-light" onClick={() => void leave()}>
          <FaSolidRightFromBracket />
          <span>Leave</span>
        </button>
      </footer>

      <Show when={newSessionOpen()}>
        <Modal
          title="New session"
          onClose={() => setNewSessionOpen(false)}
          footer={
            <>
              <button class="button is-primary" onClick={() => void resetContext()}>
                Start fresh
              </button>
              <button class="button" onClick={() => setNewSessionOpen(false)}>
                Cancel
              </button>
            </>
          }
        >
          <p>
            This clears the agent's memory of your conversation — your site and
            your chat log are kept.
          </p>
        </Modal>
      </Show>

      <Show when={statsOpen()}>
        <Modal title="Stats" onClose={() => setStatsOpen(false)}>
          <Show when={stats()} fallback={<p>Loading…</p>}>
            {(current) => (
              <div class="stats-list">
                <div class="stats-row">
                  <span>Tokens used</span>
                  <span>{current().tokensUsed}</span>
                </div>
                <div class="stats-row">
                  <span>Token limit</span>
                  <span>
                    {current().tokenLimit !== null
                      ? current().tokenLimit
                      : "unlimited"}
                  </span>
                </div>
                <div class="stats-row">
                  <span>Lines of code</span>
                  <span>{current().loc}</span>
                </div>
                <div class="stats-row">
                  <span>Your site</span>
                  <a
                    href={current().sitePath}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {`${window.location.origin}${current().sitePath}`}
                  </a>
                </div>
                <button class="button is-small" onClick={() => void copySiteUrl()}>
                  <FaSolidCopy />
                  <span>Copy site URL</span>
                </button>
              </div>
            )}
          </Show>
        </Modal>
      </Show>
    </section>
  );
}
