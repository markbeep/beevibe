import { createEffect, createSignal, onMount, Show, type JSX } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { api, ApiError } from "../lib/api";
import type { Me } from "../lib/types";
import { useSession } from "../session";

const REMEMBER_KEY = "beevibe_token";

export default function Index(): JSX.Element {
  const session = useSession();
  const navigate = useNavigate();
  const [token, setToken] = createSignal("");
  const [remember, setRemember] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [busy, setBusy] = createSignal(false);

  onMount(() => {
    const saved = localStorage.getItem(REMEMBER_KEY);
    if (saved) {
      setToken(saved);
      setRemember(true);
    }
  });

  createEffect(() => {
    const me = session.me();
    if (!me) return;
    if (me.role === "admin") {
      navigate("/admin", { replace: true });
    } else if (me.roomId) {
      navigate(`/room/${me.roomId}`, { replace: true });
    }
  });

  const submit = async (event: Event) => {
    event.preventDefault();
    const value = token().trim();
    if (!value || busy()) return;
    setBusy(true);
    setError(null);
    try {
      const me = (await api.post("/api/login", { token: value })) as Me;
      // Remember-me applies to user tokens only; the admin password is never
      // stored in the browser (IDX-6, Q-UI-19).
      if (me.role === "user" && remember()) {
        localStorage.setItem(REMEMBER_KEY, value);
      } else {
        localStorage.removeItem(REMEMBER_KEY);
      }
      session.setMe(me);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Login failed — please try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <section class="index-page">
      <div class="index-board">
        <div class="index-brand">
          <h1 class="index-mark">beevibe</h1>
          <div class="index-rule" aria-hidden="true"></div>
          <p class="index-blurb">
            No text, just rawdog voice-based vibe-coding with the feeling of
            being inside a call center.
          </p>
        </div>

        <div class="index-panel">
          <form class="index-form" onSubmit={submit}>
            <div class="index-field">
              <label class="index-label" for="index-token">
                Your token
              </label>
              <input
                id="index-token"
                class="index-input"
                type="text"
                autocomplete="off"
                autocapitalize="off"
                spellcheck={false}
                autofocus
                placeholder="a1B2c3D4"
                value={token()}
                onInput={(event) => setToken(event.currentTarget.value)}
              />
            </div>

            <label class="index-remember">
              <input
                type="checkbox"
                checked={remember()}
                onChange={(event) => setRemember(event.currentTarget.checked)}
              />
              <span>Remember my token on this device</span>
            </label>

            <Show when={error()}>
              <div class="index-error" role="alert">
                {error()}
              </div>
            </Show>

            <button class="index-submit" type="submit" disabled={busy()}>
              {busy() ? "Signing in…" : "Enter"}
            </button>
          </form>
        </div>
      </div>
    </section>
  );
}
