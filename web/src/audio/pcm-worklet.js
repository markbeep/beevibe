// AudioWorklet that forwards raw mono Float32 frames to the main thread.
// Transport only: downsampling, gain and WAV packing happen there.
class PCMCaptureProcessor extends AudioWorkletProcessor {
  process(inputs) {
    const input = inputs[0];
    if (input && input[0] && input[0].length > 0) {
      this.port.postMessage(new Float32Array(input[0]));
    }
    return true;
  }
}

registerProcessor("pcm-capture", PCMCaptureProcessor);
