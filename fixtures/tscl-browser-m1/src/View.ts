import { AppState } from "./State";

export function Status(state: AppState): string {
    return `Browser package call: ${state.token}; Count: ${state.count}`;
}
