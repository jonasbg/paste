<script lang="ts">
	import { generateQrMatrix, type QrMatrix } from '$lib/utils/qrcode';

	interface Props {
		value: string;
		size?: number;
		title?: string;
		subtitle?: string;
		compact?: boolean;
	}

	let { value, size = 156, title = '', subtitle = '', compact = false }: Props = $props();

	const QUIET_ZONE = 3;

	let qr = $derived.by<QrMatrix | null>(() => {
		if (!value) return null;
		try {
			return generateQrMatrix(value);
		} catch {
			return null;
		}
	});

	function isFinderRegion(r: number, c: number, n: number): boolean {
		return (
			(r < 7 && c < 7) ||
			(r < 7 && c >= n - 7) ||
			(r >= n - 7 && c < 7)
		);
	}

	let finderPositions = $derived(
		qr
			? [
					{ x: QUIET_ZONE, y: QUIET_ZONE },
					{ x: QUIET_ZONE + qr.size - 7, y: QUIET_ZONE },
					{ x: QUIET_ZONE, y: QUIET_ZONE + qr.size - 7 }
				]
			: []
	);

	let dataModules = $derived.by(() => {
		if (!qr) return [];
		const dots: Array<{ x: number; y: number }> = [];
		const n = qr.size;
		for (let r = 0; r < n; r++) {
			for (let c = 0; c < n; c++) {
				if (qr.modules[r][c] && !isFinderRegion(r, c, n)) {
					dots.push({ x: c + QUIET_ZONE, y: r + QUIET_ZONE });
				}
			}
		}
		return dots;
	});

	let viewBoxSize = $derived(qr ? qr.size + QUIET_ZONE * 2 : 0);
</script>

{#if qr}
	<div class="qr-card" class:compact>
		<div class="qr-frame" style="width: {size}px; height: {size}px;">
			<svg
				viewBox="0 0 {viewBoxSize} {viewBoxSize}"
				width={size}
				height={size}
				role="img"
				aria-label={title || 'QR code'}
				shape-rendering="geometricPrecision"
			>
				<rect x="0" y="0" width={viewBoxSize} height={viewBoxSize} rx="2" fill="#ffffff" />

				<!-- Rounded finder rings with filled circular corner dots -->
				{#each finderPositions as pos}
					<rect
						x={pos.x + 0.5}
						y={pos.y + 0.5}
						width="6"
						height="6"
						rx="1.85"
						fill="none"
						stroke="#166534"
						stroke-width="1"
					/>
					<circle
						cx={pos.x + 3.5}
						cy={pos.y + 3.5}
						r="1.5"
						fill="#166534"
					/>
				{/each}

				<!-- Circular data modules -->
				{#each dataModules as dot}
					<circle
						cx={dot.x + 0.5}
						cy={dot.y + 0.5}
						r="0.43"
						fill="#166534"
					/>
				{/each}
			</svg>
		</div>

		{#if title || subtitle}
			<div class="qr-caption">
				{#if title}
					<div class="qr-title">
						<svg
							width="15"
							height="15"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<rect x="5" y="2" width="14" height="20" rx="2" ry="2" />
							<line x1="12" y1="18" x2="12.01" y2="18" />
						</svg>
						<span>{title}</span>
					</div>
				{/if}
				{#if subtitle}
					<p class="qr-subtitle">{subtitle}</p>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.qr-card {
		display: flex;
		align-items: center;
		gap: 1.15rem;
		padding: 0.9rem 1.15rem;
		background: #f6fbf8;
		border: 1px solid #d9efe2;
		border-radius: 12px;
	}

	.qr-card.compact {
		flex-direction: column;
		gap: 0.55rem;
		padding: 0.75rem;
		text-align: center;
	}

	.qr-frame {
		flex-shrink: 0;
		background: #ffffff;
		border-radius: 10px;
		padding: 6px;
		box-sizing: content-box;
		border: 1px solid #d9efe2;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.qr-frame svg {
		display: block;
		width: 100%;
		height: 100%;
	}

	.qr-caption {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		min-width: 0;
	}

	.qr-title {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.92rem;
		font-weight: 600;
		color: #111827;
	}

	.qr-card.compact .qr-title {
		justify-content: center;
		font-size: 0.82rem;
	}

	.qr-title svg {
		color: var(--primary-green);
		flex-shrink: 0;
	}

	.qr-subtitle {
		margin: 0;
		font-size: 0.8rem;
		line-height: 1.45;
		color: #6b7280;
	}

	@media (max-width: 600px) {
		.qr-card {
			flex-direction: column;
			text-align: center;
		}
		.qr-title {
			justify-content: center;
		}
	}
</style>
