import { For, type JSX } from "solid-js";

export interface MicSettingsProps {
  devices: MediaDeviceInfo[];
  device: string;
  onDevice: (deviceId: string) => void;
  gain: number;
  onGain: (value: number) => void;
  level: number;
}

// Device picker, input gain and a live level meter. No VAD threshold
// (Q-STT-10): the meter is informational only.
export function MicSettings(props: MicSettingsProps): JSX.Element {
  return (
    <div class="mic-settings box">
      <div class="field">
        <label class="label is-small">Input device</label>
        <div class="select is-small is-fullwidth">
          <select
            value={props.device}
            onChange={(event) => props.onDevice(event.currentTarget.value)}
          >
            <option value="">System default</option>
            <For each={props.devices}>
              {(device, index) => (
                <option value={device.deviceId}>
                  {device.label || `Microphone ${index() + 1}`}
                </option>
              )}
            </For>
          </select>
        </div>
      </div>

      <div class="field">
        <label class="label is-small">
          Input gain — {Math.round(props.gain * 100)}%
        </label>
        <input
          class="slider"
          type="range"
          min="0"
          max="2"
          step="0.05"
          value={props.gain}
          onInput={(event) => props.onGain(Number(event.currentTarget.value))}
        />
      </div>

      <div class="field">
        <label class="label is-small">Input level</label>
        <progress class="progress is-small is-link" max="100" value={props.level}>
          {props.level}%
        </progress>
      </div>
    </div>
  );
}
