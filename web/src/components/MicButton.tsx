import { createSignal, onCleanup, onMount, type JSX } from "solid-js";
import { FaSolidMicrophone } from "solid-icons/fa";

// Space inside these controls must keep its native behaviour (typing / opening
// the device picker), never start a recording.
const IGNORED_TAGS: Record<string, true> = {
  INPUT: true,
  TEXTAREA: true,
  SELECT: true,
};

export interface MicButtonProps {
  recording: boolean;
  disabled: boolean;
  countdown: number | null;
  onStart: () => void;
  onEnd: () => void;
}

// Push-to-talk: hold the button or hold Space. No persistent mic-on state.
export function MicButton(props: MicButtonProps): JSX.Element {
  const [pressed, setPressed] = createSignal(false);

  const start = () => {
    if (props.disabled || pressed()) return;
    setPressed(true);
    props.onStart();
  };

  const end = () => {
    if (!pressed()) return;
    setPressed(false);
    props.onEnd();
  };

  // Ignore Space originating from a form control (typed text, device picker).
  const ignoredTarget = (target: EventTarget | null): boolean => {
    const element = target as HTMLElement | null;
    return (
      element !== null &&
      (IGNORED_TAGS[element.tagName] === true || element.isContentEditable)
    );
  };

  // Press must be visible immediately: acquisition of the microphone can take
  // hundreds of milliseconds, so a cold press shows "Starting…" first.
  const label = () => {
    if (props.recording) return "Release to send";
    if (pressed()) return "Starting…";
    return "Hold to talk";
  };

  const stateClass = () => {
    if (props.recording) return "is-danger is-recording";
    if (pressed()) return "is-warning is-pressed";
    return "is-primary";
  };

  const onKeyDown = (event: KeyboardEvent) => {
    if (event.code !== "Space" || event.repeat || props.disabled) return;
    if (ignoredTarget(event.target)) return;
    event.preventDefault();
    start();
  };

  const onKeyUp = (event: KeyboardEvent) => {
    if (event.code !== "Space") return;
    if (ignoredTarget(event.target)) return;
    event.preventDefault();
    end();
  };

  onMount(() => {
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("keyup", onKeyUp);
  });

  onCleanup(() => {
    window.removeEventListener("keydown", onKeyDown);
    window.removeEventListener("keyup", onKeyUp);
  });

  return (
    <button
      type="button"
      class={`button mic-button ${stateClass()}`}
      disabled={props.disabled}
      onPointerDown={(event) => {
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        start();
      }}
      onPointerUp={(event) => {
        event.preventDefault();
        end();
      }}
      onPointerCancel={() => end()}
      onLostPointerCapture={() => end()}
      onContextMenu={(event) => event.preventDefault()}
    >
      <FaSolidMicrophone />
      <span>{label()}</span>
      {props.countdown !== null ? (
        <span class="mic-countdown">{props.countdown}</span>
      ) : null}
    </button>
  );
}
