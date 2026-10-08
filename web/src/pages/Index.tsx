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
      <div class="index-hero">
        <h1 class="title is-2">beevibe</h1>
        <p class="index-blurb">
          Speak a request and your own little website changes to match. Scan the
          token you were given, or talk to the admin.
        </p>
      </div>

      <div class="card index-card">
        <div class="card-content">
          <form onSubmit={submit}>
            <div class="field">
              <label class="label">Your token</label>
              <div class="control">
                <input
                  class="input"
                  type="text"
                  autocomplete="off"
                  autofocus
                  placeholder="a1B2c3D4"
                  value={token()}
                  onInput={(event) => setToken(event.currentTarget.value)}
                />
              </div>
            </div>

            <div class="field">
              <label class="checkbox">
                <input
                  type="checkbox"
                  checked={remember()}
                  onChange={(event) => setRemember(event.currentTarget.checked)}
                />
                Remember my token on this device
              </label>
            </div>

            <Show when={error()}>
              <div class="notification is-danger is-light">{error()}</div>
            </Show>

            <div class="field">
              <button
                class="button is-primary is-fullwidth"
                type="submit"
                disabled={busy()}
              >
                {busy() ? "Signing in…" : "Enter"}
              </button>
            </div>
          </form>
        </div>
      </div>
    </section>
  );
}
