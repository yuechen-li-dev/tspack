import { createDefaultModel, renderRuntimeSwitchNotes } from "../ui";

export function mountRuntimeSwitchNotes(root: Element): void {
  root.innerHTML = renderRuntimeSwitchNotes(createDefaultModel());
}

const appRoot = document.querySelector("#app");

if (appRoot) {
  mountRuntimeSwitchNotes(appRoot);
}
