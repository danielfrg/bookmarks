/**
 * Human-readable capture stages, so a failed capture says where it stopped
 * ("timed out while fetching resources (12/40)") instead of only "timed out".
 */

/** The parts of a single-file-core ProgressEvent the capture script reads. */
export interface SingleFileProgress {
  type: string;
  detail?: { max?: number; step?: number };
}

/** Tracks single-file-core progress events and describes the current stage. */
export class StageTracker {
  #resources = 0;
  #loaded = 0;

  /** The stage for `event`, or null when the event does not change it. */
  update(event: SingleFileProgress): string | null {
    switch (event.type) {
      case "page-loading":
        return "reading the page";
      case "resources-initializing":
        return "listing resources";
      case "resources-initialized":
        this.#resources = event.detail?.max ?? 0;
        this.#loaded = 0;
        return this.#fetching();
      case "resource-loaded":
        this.#loaded++;
        return this.#fetching();
      case "stage-started":
        return `processing the page (step ${(event.detail?.step ?? 0) + 1} of 6)`;
      case "page-ended":
        return "serializing the page";
      default:
        return null;
    }
  }

  #fetching(): string {
    return `fetching resources (${this.#loaded}/${this.#resources})`;
  }
}

/** Stages the background reports on its own, before and after the script runs. */
export const STAGE_INJECTING = "injecting the capture script";
export const STAGE_STARTING = "waiting for the capture script to start";
export const STAGE_DEFERRED = "loading lazy images and deferred content";

export function captureTimeoutMessage(opts: {
  stage: string;
  elapsedMs: number;
  sinceProgressMs: number;
}): string {
  const s = (ms: number) => `${Math.round(ms / 1000)} s`;
  return (
    `capture timed out after ${s(opts.elapsedMs)} while ${opts.stage} ` +
    `(last progress ${s(opts.sinceProgressMs)} ago)`
  );
}
