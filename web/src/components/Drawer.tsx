import { onCleanup, onMount, Show, type JSX } from "solid-js";

export interface DrawerProps {
  title: string;
  open: boolean;
  onClose: () => void;
  children: JSX.Element;
}

export function Drawer(props: DrawerProps): JSX.Element {
  onMount(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") props.onClose();
    };
    window.addEventListener("keydown", onKey);
    onCleanup(() => window.removeEventListener("keydown", onKey));
  });

  return (
    <Show when={props.open}>
      <div class="drawer-backdrop" onClick={() => props.onClose()} />
      <aside class="drawer">
        <header class="drawer-head">
          <span class="drawer-title">{props.title}</span>
          <button
            class="delete"
            aria-label="close drawer"
            onClick={() => props.onClose()}
          />
        </header>
        <div class="drawer-body">{props.children}</div>
      </aside>
    </Show>
  );
}
