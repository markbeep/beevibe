import { downsample, floatToPCM16, TARGET_SAMPLE_RATE } from "./wav";
import workletUrl from "../audio/pcm-worklet.js?url";

// Microphone engine: source -> GainNode -> AnalyserNode -> AudioWorkletNode
// (-> destination, so the graph is pulled). No VAD gate: everything between
// PTT press and release is kept (Q-STT-10).
export class Mic {
  private context: AudioContext | null = null;
  private stream: MediaStream | null = null;
  private source: MediaStreamAudioSourceNode | null = null;
  private gainNode: GainNode | null = null;
  private analyser: AnalyserNode | null = null;
  private worklet: AudioWorkletNode | null = null;
  private deviceId = "";
  private gainValue = 1;
  private capturing = false;
  private chunks: Float32Array[] = [];
  private levelFrame = 0;
  private levelBuffer: Float32Array<ArrayBuffer> | null = null;

  onLevel: ((level: number) => void) | null = null;

  get sampleRate(): number {
    return this.context?.sampleRate ?? TARGET_SAMPLE_RATE;
  }

  static async listInputs(): Promise<MediaDeviceInfo[]> {
    if (!navigator.mediaDevices?.enumerateDevices) return [];
    const devices = await navigator.mediaDevices.enumerateDevices();
    return devices.filter((d) => d.kind === "audioinput");
  }

  async open(deviceId: string, gain: number): Promise<void> {
    if (this.context && this.deviceId === deviceId) {
      this.setGain(gain);
      if (this.context.state === "suspended") await this.context.resume();
      return;
    }
    this.close();

    const stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        deviceId: deviceId ? { exact: deviceId } : undefined,
        echoCancellation: false,
        noiseSuppression: false,
        autoGainControl: false,
      },
    });
    const context = new AudioContext();
    await context.audioWorklet.addModule(workletUrl);

    const source = context.createMediaStreamSource(stream);
    const gainNode = context.createGain();
    gainNode.gain.value = gain;
    const analyser = context.createAnalyser();
    analyser.fftSize = 1024;
    const worklet = new AudioWorkletNode(context, "pcm-capture");
    worklet.port.onmessage = (event: MessageEvent) => {
      if (!this.capturing) return;
      this.chunks.push(event.data as Float32Array);
    };

    source.connect(gainNode);
    gainNode.connect(analyser);
    gainNode.connect(worklet);
    worklet.connect(context.destination);

    this.context = context;
    this.stream = stream;
    this.source = source;
    this.gainNode = gainNode;
    this.analyser = analyser;
    this.worklet = worklet;
    this.deviceId = deviceId;
    this.gainValue = gain;
    if (context.state === "suspended") await context.resume();
    this.startLevelLoop();
  }

  setGain(value: number): void {
    this.gainValue = value;
    if (this.gainNode) this.gainNode.gain.value = value;
  }

  private startLevelLoop(): void {
    const analyser = this.analyser;
    if (!analyser) return;
    this.levelBuffer = new Float32Array(analyser.fftSize);
    const tick = () => {
      if (!this.analyser || !this.levelBuffer) return;
      this.analyser.getFloatTimeDomainData(this.levelBuffer);
      let sum = 0;
      for (let i = 0; i < this.levelBuffer.length; i++) {
        sum += this.levelBuffer[i] * this.levelBuffer[i];
      }
      const rms = Math.sqrt(sum / this.levelBuffer.length);
      const db = 20 * Math.log10(rms || 1e-8);
      const level = Math.max(0, Math.min(100, Math.round(((db + 60) / 60) * 100)));
      this.onLevel?.(level);
      this.levelFrame = requestAnimationFrame(tick);
    };
    this.levelFrame = requestAnimationFrame(tick);
  }

  begin(): void {
    this.chunks = [];
    this.capturing = true;
  }

  // Stop collecting and return 16 kHz mono PCM16 samples (null when empty).
  end(): Int16Array | null {
    this.capturing = false;
    const total = this.chunks.reduce((n, c) => n + c.length, 0);
    if (total === 0) return null;
    const merged = new Float32Array(total);
    let offset = 0;
    for (const chunk of this.chunks) {
      merged.set(chunk, offset);
      offset += chunk.length;
    }
    this.chunks = [];
    return floatToPCM16(downsample(merged, this.sampleRate, TARGET_SAMPLE_RATE));
  }

  close(): void {
    this.capturing = false;
    this.chunks = [];
    if (this.levelFrame) {
      cancelAnimationFrame(this.levelFrame);
      this.levelFrame = 0;
    }
    this.levelBuffer = null;
    if (this.worklet) {
      this.worklet.port.onmessage = null;
      this.worklet.disconnect();
      this.worklet = null;
    }
    if (this.source) {
      this.source.disconnect();
      this.source = null;
    }
    if (this.gainNode) {
      this.gainNode.disconnect();
      this.gainNode = null;
    }
    if (this.analyser) {
      this.analyser.disconnect();
      this.analyser = null;
    }
    if (this.stream) {
      for (const track of this.stream.getTracks()) track.stop();
      this.stream = null;
    }
    if (this.context) {
      void this.context.close();
      this.context = null;
    }
    this.deviceId = "";
  }
}
