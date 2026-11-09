import { formatEta } from '$lib/utils/filePreview';

/**
 * Eases a displayed percentage toward the latest reported progress and derives
 * an ETA from the elapsed time of the current run.
 *
 * States: idle → running (begin) → finishing (end: eases to the last target)
 *         or complete (complete: snaps to 100). begin() always starts a fresh
 *         run, so a retry never reuses the previous attempt's start time.
 */
export class SmoothProgress {
	display = $state(0);
	eta = $state('');

	#target = 0;
	#startTime = 0;
	#frame = 0;
	#running = false;

	/** Start a new run from 0%. */
	begin() {
		this.#cancel();
		this.#target = 0;
		this.#startTime = 0;
		this.display = 0;
		this.eta = '';
		this.#running = true;
	}

	/** Report the latest real progress (0–100). */
	set(progress: number) {
		if (progress === this.#target && progress === this.display) return;
		this.#target = progress;
		if (this.#startTime === 0 && progress > 0) this.#startTime = Date.now();
		this.#schedule();
	}

	/** Stop the run but let the bar ease to the last reported value. */
	end() {
		this.#running = false;
		this.#schedule();
	}

	/** Finish the run at 100%. */
	complete() {
		this.#cancel();
		this.#running = false;
		this.#target = 100;
		this.display = 100;
		this.eta = '';
	}

	/** Back to idle at 0%. */
	reset() {
		this.#cancel();
		this.#running = false;
		this.#target = 0;
		this.#startTime = 0;
		this.display = 0;
		this.eta = '';
	}

	/** Jump straight to a value without animating (e.g. on mount). */
	jumpTo(progress: number) {
		this.#cancel();
		this.#target = progress;
		this.display = progress;
	}

	destroy() {
		this.#cancel();
	}

	#cancel() {
		if (this.#frame) cancelAnimationFrame(this.#frame);
		this.#frame = 0;
	}

	#schedule() {
		this.#cancel();
		this.#frame = requestAnimationFrame(this.#tick);
	}

	#tick = () => {
		this.#frame = 0;
		const diff = this.#target - this.display;
		if (Math.abs(diff) < 0.2) {
			this.display = this.#target;
		} else {
			this.display += diff * 0.1;
		}

		if (this.display > 2 && this.display < 99 && this.#startTime > 0) {
			const elapsed = (Date.now() - this.#startTime) / 1000;
			if (elapsed > 0.5) {
				const rate = this.display / elapsed;
				this.eta = formatEta((100 - this.display) / rate);
			}
		} else if (this.display >= 99) {
			this.eta = '';
		}

		// Keep ticking while a run is active so the ETA keeps updating even when
		// progress reports stall.
		if (this.display !== this.#target || this.#running) {
			this.#frame = requestAnimationFrame(this.#tick);
		}
	};
}
