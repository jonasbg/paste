export type FitViewportOptions = {
	/** Ancestor whose matching descendants' heights are reserved below the node. */
	containerSelector?: string;
	/** Elements inside the container whose height must stay visible. */
	reserveSelector?: string;
	bottomGap?: number;
	minHeight?: number;
};

/**
 * Svelte action that caps a preview element's max-height so it, plus any
 * reserved elements (e.g. the file row below it), fits in the visible viewport.
 */
export function fitViewport(node: HTMLElement, options: FitViewportOptions = {}) {
	let frame = 0;
	let resizeObserver: ResizeObserver | null = null;
	let currentOptions = options;

	const getOuterHeight = (element: Element) => {
		const styles = window.getComputedStyle(element as HTMLElement);
		const marginTop = parseFloat(styles.marginTop) || 0;
		const marginBottom = parseFloat(styles.marginBottom) || 0;
		return (element as HTMLElement).getBoundingClientRect().height + marginTop + marginBottom;
	};

	const update = () => {
		frame = 0;
		const rect = node.getBoundingClientRect();
		const viewportHeight = window.visualViewport?.height || window.innerHeight;
		const { containerSelector, reserveSelector } = currentOptions;
		const reserveHeight =
			containerSelector && reserveSelector
				? Array.from(
						node.closest(containerSelector)?.querySelectorAll(reserveSelector) ?? []
					).reduce((total, element) => total + getOuterHeight(element), 0)
				: 0;
		const bottomGap = currentOptions.bottomGap ?? 24;
		const minHeight = currentOptions.minHeight ?? 120;
		const availableHeight = Math.max(
			minHeight,
			Math.floor(viewportHeight - rect.top - reserveHeight - bottomGap)
		);
		node.style.maxHeight = `${availableHeight}px`;
	};

	const scheduleUpdate = () => {
		if (frame) cancelAnimationFrame(frame);
		frame = requestAnimationFrame(update);
	};

	scheduleUpdate();
	window.addEventListener('resize', scheduleUpdate);
	window.visualViewport?.addEventListener('resize', scheduleUpdate);

	if ('ResizeObserver' in window) {
		resizeObserver = new ResizeObserver(scheduleUpdate);
		resizeObserver.observe(document.body);
	}

	return {
		update(nextOptions: FitViewportOptions = currentOptions) {
			currentOptions = nextOptions;
			scheduleUpdate();
		},
		destroy() {
			if (frame) cancelAnimationFrame(frame);
			window.removeEventListener('resize', scheduleUpdate);
			window.visualViewport?.removeEventListener('resize', scheduleUpdate);
			resizeObserver?.disconnect();
		}
	};
}
