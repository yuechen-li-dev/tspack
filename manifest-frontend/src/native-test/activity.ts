import { AsyncLocalStorage } from "node:async_hooks";

export type ActivityTracker = {
  markAssert: () => void;
  markExpect: () => void;
  markSkip: () => void;
  markArtifactWrite: () => void;
};

const activityStorage = new AsyncLocalStorage<ActivityTracker | undefined>();
let fallbackTracker: ActivityTracker | undefined;

export function setActivityTracker(next: ActivityTracker | undefined): void {
  fallbackTracker = next;
  activityStorage.enterWith(next);
}

function currentTracker(): ActivityTracker | undefined {
  return activityStorage.getStore() ?? fallbackTracker;
}

export function markAssertActivity(): void {
  currentTracker()?.markAssert();
}

export function markExpectationActivity(): void {
  currentTracker()?.markExpect();
}

export function markSkipActivity(): void {
  currentTracker()?.markSkip();
}

export function markArtifactWriteActivity(): void {
  currentTracker()?.markArtifactWrite();
}
