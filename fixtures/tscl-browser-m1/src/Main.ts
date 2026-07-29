import { nanoid } from "nanoid";
import { dispatch, onClick, setText } from "@copeland/browser-v1";
import { AppEvent, AppState, Reduce, SendIncrement } from "./State";
import { Status } from "./View";

export function Main(): void {
    const generatedId: string = nanoid();
    const send: (event: AppEvent) => void = dispatch<AppState, AppEvent>(
        { count: 0, token: generatedId },
        Reduce,
        state => setText("status", Status(state)));

    onClick("increment", capture { send } () => SendIncrement(send));
}
