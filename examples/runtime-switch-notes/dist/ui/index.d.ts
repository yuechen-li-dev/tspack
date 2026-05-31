export type RuntimeStatus = {
  name: "Node.js" | "Bun" | "Deno";
  port: number;
  runTarget: string;
  state: "ready" | "optional";
};
export type Note = {
  id: string;
  text: string;
};
export type RuntimeSwitchNotesModel = {
  runtimes: RuntimeStatus[];
  notes: Note[];
};
export declare function createDefaultModel(): RuntimeSwitchNotesModel;
export declare function renderRuntimeSwitchNotes(model?: RuntimeSwitchNotesModel): string;
