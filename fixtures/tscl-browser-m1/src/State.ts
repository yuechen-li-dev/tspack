export record AppState {
    count: int;
    token: string;
}

export enum AppEvent {
    Increment,
}

export function Reduce(state: AppState, event: AppEvent): AppState {
    return switch event {
        Increment => state with { count: state.count + 1 },
    };
}

export function SendIncrement(send: (event: AppEvent) => void): void {
    send(AppEvent.Increment);
}
