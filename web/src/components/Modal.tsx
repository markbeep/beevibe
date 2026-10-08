import { onCleanup, onMount, type JSX } from "solid-js";

export interface ModalProps {
  title: string;
  onClose: () => void;
  children?: JSX.Element;
  footer?: JSX.Element;
}

export function Modal(props: ModalProps): JSX.Element {
  onMount(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") props.onClose();
    };
    window.addEventListener("keydown", onKey);
    onCleanup(() => window.removeEventListener("keydown", onKey));
  });

  return (
    <div class="modal is-active">
      <div class="modal-background" onClick={() => props.onClose()} />
      <div class="modal-card">
        <header class="modal-card-head">
          <p class="modal-card-title">{props.title}</p>
          <button
            class="delete"
            aria-label="close"
            onClick={() => props.onClose()}
          />
        </header>
        <section class="modal-card-body">{props.children}</section>
        {props.footer ? (
          <footer class="modal-card-foot">{props.footer}</footer>
        ) : null}
      </div>
    </div>
  );
}
