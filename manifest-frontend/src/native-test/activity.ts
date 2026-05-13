export type ActivityTracker = {
  markAssert: () => void;
  markExpect: () => void;
  markSkip: () => void;
  markArtifactWrite: () => void;
};

let tracker: ActivityTracker | undefined;

export function setActivityTracker(next: ActivityTracker | undefined): void {
  tracker = next;
}

export function markAssertActivity(): void {
  tracker?.markAssert();
}

export function markExpectationActivity(): void {
  tracker?.markExpect();
}

export function markSkipActivity(): void {
  tracker?.markSkip();
}

export function markArtifactWriteActivity(): void {
  tracker?.markArtifactWrite();
}
