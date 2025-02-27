<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { parse, format } from 'date-fns';
	import ApexCharts from 'apexcharts';
	import { onMount } from 'svelte';
	import type {
		ActivitySummary,
		SecurityMetrics,
		StorageSummary,
		RequestMetrics,
		UploadHistoryItem
	} from '$lib/types';

	export let data: {
		activity: ActivitySummary[];
		metrics: SecurityMetrics;
		storage: StorageSummary;
		requests: RequestMetrics;
		uploadHistory: UploadHistoryItem[];
		range: string;
		error?: string;
	};

	let dateRange = data.range;
	let timeSeriesChartElement: HTMLElement;
	let statusChartElement: HTMLElement;
	let pathDistributionElement: HTMLElement;
	let latencyTimelineElement: HTMLElement;
	let storageDistributionElement: HTMLElement;
	let uploadHistoryChartElement: HTMLElement;

	let timeSeriesChart: ApexCharts;
	let statusChart: ApexCharts;
	let pathDistributionChart: ApexCharts;
	let latencyTimelineChart: ApexCharts;
	let storageDistributionChart: ApexCharts;
	let uploadHistoryChart: ApexCharts;

	const rangeOptions = [
		{ value: '24h', label: 'Last 24 Hours' },
		{ value: '7d', label: 'Last 7 Days' },
		{ value: '30d', label: 'Last 30 Days' },
		{ value: '90d', label: 'Last 90 Days' }
	];

	function formatBytes(bytes: number) {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
	}

	// Function to fill in missing dates in the upload history data
	function fillMissingDates(data: Array<{ date: string, file_count: number, total_size: number }>, rangeType: string) {
    if (data.length === 0) {
        // Even with empty data, we should generate dates for the selected range
        data = [{ date: new Date().toISOString().split('T')[0], file_count: 0, total_size: 0 }];
    }

    // Sort the data by date
    const sortedData = [...data].sort((a, b) => a.date.localeCompare(b.date));

    // Calculate start date based on the range selection
    const today = new Date();
    today.setHours(23, 59, 59, 999); // End of today
		today.setDate(today.getDate() + 1);
    let startDate: Date;

    switch (rangeType) {
        case '24h':
            startDate = new Date(today);
            startDate.setDate(today.getDate() - 1);
            break;
        case '7d':
            startDate = new Date(today);
            startDate.setDate(today.getDate() - 7);
            break;
        case '30d':
            startDate = new Date(today);
            startDate.setDate(today.getDate() - 30);
            break;
        case '90d':
            startDate = new Date(today);
            startDate.setDate(today.getDate() - 90);
            break;
        default:
            // If no specific range, use the data's own start date
            startDate = new Date(sortedData[0].date);
    }

    // Make sure we start at beginning of the day
    startDate.setHours(0, 0, 0, 0);

    // Create a map of existing dates for quick lookup
    const dateMap = new Map();
    sortedData.forEach(item => {
        dateMap.set(item.date, item);
    });

    // Create a new array with all dates in the range
    const result = [];
    const currentDate = new Date(startDate);

    while (currentDate <= today) {
        const dateStr = currentDate.toISOString().split('T')[0]; // YYYY-MM-DD format

        if (dateMap.has(dateStr)) {
            // Use existing data
            result.push(dateMap.get(dateStr));
        } else {
            // Add zero values for missing dates
            result.push({
                date: dateStr,
                file_count: 0,
                total_size: 0
            });
        }

        // Move to next day
        currentDate.setDate(currentDate.getDate() + 1);
    }

    return result;
}

	async function handleRangeChange() {
		const searchParams = new URLSearchParams($page.url.searchParams);
		searchParams.set('range', dateRange);
		goto(`?${searchParams.toString()}`, { replaceState: true });
	}

	function getStatusCodeColor(code: string) {
		const primaryGreen = getComputedStyle(document.documentElement)
			.getPropertyValue('--primary-green').trim();

		const num = parseInt(code);
		if (num < 300) return primaryGreen; // Green
		if (num < 400) return '#DCED31'; // Yellow
		if (num < 500) return '#E03616'; // Red
		return '#E03616'; // Purple
	}

	onMount(() => {
		if (data.activity && data.metrics && data.requests) {
			const primaryGreen = getComputedStyle(document.documentElement)
			.getPropertyValue('--primary-green').trim();

			// Process upload history to fill in missing dates with zeros
			const filledUploadHistory = fillMissingDates(data.uploadHistory, dateRange);

			// Main Time Series Chart (Unified view)
			const timeSeriesOptions = {
				chart: {
					type: 'area',
					height: 350,
					fontFamily: 'Inter var, system-ui, -apple-system, sans-serif',
					toolbar: {
						show: false
					},
					background: 'transparent'
				},
				dataLabels: {
					enabled: false
				},
				zoom: {
					enabled: false,
					allowMouseWheelZoom: false,
				},
				stroke: {
					curve: 'smooth',
					width: 2
				},
				fill: {
					type: 'gradient',
					gradient: {
						shadeIntensity: 1,
						opacityFrom: 0.7,
						opacityTo: 0.2,
						stops: [0, 90, 100]
					}
				},
				colors: [primaryGreen, primaryGreen, primaryGreen],
				series: [
					{
						name: 'Uploads',
						data: data.activity.map((d) => ({
							x: new Date(d.period).getTime(),
							y: d.uploads
						}))
					},
					{
						name: 'Downloads',
						data: data.activity.map((d) => ({
							x: new Date(d.period).getTime(),
							y: d.downloads
						}))
					},
					{
						name: 'Unique Visitors',
						data: data.activity.map((d) => ({
							x: new Date(d.period).getTime(),
							y: d.unique_visitors
						}))
					}
				],
				xaxis: {
					type: 'datetime',
					labels: {
						formatter: (val: string) => format(new Date(parseInt(val)), 'MMM d, yyyy')
					}
				},
				yaxis: {
					labels: {
						formatter: (val: number) => Math.round(val)
					}
				},
				legend: {
					show: false
				},
				tooltip: {
					theme: 'light',
					x: {
						format: 'dd MMM yyyy'
					},
					marker: {
						size: 6, // Smaller marker size
						strokeWidth: 0
					}
				}
			};

			// Status Distribution Chart
			const statusDistributionOptions = {
				chart: {
					type: 'donut',
					height: 280,
					background: 'transparent'
				},
				colors: Object.keys(data.requests.status_distribution).map((code) =>
					getStatusCodeColor(code)
				),
				series: Object.values(data.requests.status_distribution),
				labels: Object.keys(data.requests.status_distribution).map((code) => `${code} Status`),
				legend: {
					show: false
				},
				plotOptions: {
					pie: {
						donut: {
							size: '65%',
							labels: {
								show: true,
								total: {
									show: true,
									label: 'Total Requests',
									fontSize: '14px',
									fontWeight: 600,
									formatter: function (w) {
										return w.globals.seriesTotals.reduce((a, b) => a + b, 0).toLocaleString();
									}
								},
								value: {
									fontSize: '22px',
									fontWeight: 600
								}
							}
						}
					}
				}
			};

			// Path Distribution Chart (Top 10 paths)
			const pathDistributionOptions = {
				chart: {
					type: 'bar',
					height: 280,
					toolbar: {
						show: false
					},
					background: 'transparent'
				},
				plotOptions: {
					bar: {
						horizontal: true,
						borderRadius: 4,
						barHeight: '70%'
					}
				},
				colors: [primaryGreen, primaryGreen, primaryGreen],
				series: [
					{
						name: 'Requests',
						data: Object.entries(data.requests.path_distribution)
							.sort((a, b) => b[1] - a[1])
							.slice(0, 10)
							.map(([path, count]) => count)
					}
				],
				xaxis: {
					categories: Object.entries(data.requests.path_distribution)
						.sort((a, b) => b[1] - a[1])
						.slice(0, 10)
						.map(([path]) => path)
				},
				tooltip: {
					theme: 'light'
				}
			};

			// Storage Distribution Chart
			const storageDistributionOptions = {
				chart: {
					type: 'pie',
					height: 280,
					background: 'transparent'
				},
				series: Object.values(data.storage.file_size_distribution),
				labels: Object.keys(data.storage.file_size_distribution),
				colors: [primaryGreen, '#68CA99', '#86D5AD'],
				legend: {
					show: false
				},
				plotOptions: {
					pie: {
						size: '70%'
					}
				},
				tooltip: {
					theme: 'light'
				}
			};

			// Upload History Chart - Modified to match time series approach
			const uploadHistoryOptions = {
				chart: {
					type: 'area',
					height: 280,
					toolbar: {
						show: false
					},
					background: 'transparent'
				},
				stroke: {
					curve: 'smooth',
					width: 2
				},
				fill: {
					type: 'gradient',
					gradient: {
						shadeIntensity: 1,
						opacityFrom: 0.7,
						opacityTo: 0.2,
						stops: [0, 90, 100]
					}
				},
				colors: [primaryGreen, '#68CA99'],
				series: [
					{
						name: 'File Count',
						data: filledUploadHistory.map(item => ({
							x: new Date(item.date).getTime(),
							y: item.file_count
						}))
					},
					{
						name: 'Total Size (MB)',
						data: filledUploadHistory.map(item => ({
							x: new Date(item.date).getTime(),
							y: Math.round(item.total_size / (1024 * 1024) * 100) / 100
						}))
					}
				],
				dataLabels: {
					enabled: false
				},
				xaxis: {
					type: 'datetime',
					labels: {
						formatter: (val: string) => format(new Date(parseInt(val)), 'MMM d, yyyy')
					}
				},
				yaxis: [
					{
						title: {
							text: 'File Count'
						},
						min: 0,
						forceNiceScale: true,
						labels: {
							formatter: (val: number) => Math.round(val)
						}
					},
					{
						opposite: true,
						title: {
							text: 'Size (MB)'
						},
						min: 0,
						forceNiceScale: true,
						labels: {
							formatter: (val: number) => `${Math.round(val)}`
						}
					}
				],
				tooltip: {
					shared: true,
					intersect: false,
					theme: 'light',
					marker: {
						size: 6, // Smaller marker size
						strokeWidth: 0
					},
					x: {
						format: 'dd MMM yyyy'
					},
					y: [
						{
							formatter: function (val: number) {
								return val + " files";
							}
						},
						{
							formatter: function (val: number) {
								return formatBytes(val * 1024 * 1024);
							}
						}
					]
				},
				legend: {
					position: 'top',
					horizontalAlign: 'right'
				},
				grid: {
					padding: {
						left: 10,
						right: 10
					}
				}
			};

			timeSeriesChart = new ApexCharts(timeSeriesChartElement, timeSeriesOptions);
			statusChart = new ApexCharts(statusChartElement, statusDistributionOptions);
			pathDistributionChart = new ApexCharts(pathDistributionElement, pathDistributionOptions);
			storageDistributionChart = new ApexCharts(
				storageDistributionElement,
				storageDistributionOptions
			);
			uploadHistoryChart = new ApexCharts(
				uploadHistoryChartElement,
				uploadHistoryOptions
			);

			timeSeriesChart.render();
			statusChart.render();
			pathDistributionChart.render();
			storageDistributionChart.render();
			uploadHistoryChart.render();
		}

		return () => {
			timeSeriesChart?.destroy();
			statusChart?.destroy();
			pathDistributionChart?.destroy();
			storageDistributionChart?.destroy();
			uploadHistoryChart?.destroy();
		};
	});

	$: if (timeSeriesChart && data.activity) {
		timeSeriesChart.updateSeries([
			{
				name: 'Uploads',
				data: data.activity.map((d) => ({
					x: new Date(d.period).getTime(),
					y: d.uploads
				}))
			},
			{
				name: 'Downloads',
				data: data.activity.map((d) => ({
					x: new Date(d.period).getTime(),
					y: d.downloads
				}))
			},
			{
				name: 'Unique Visitors',
				data: data.activity.map((d) => ({
					x: new Date(d.period).getTime(),
					y: d.unique_visitors
				}))
			}
		]);
	}

	$: if (statusChart && data.requests?.status_distribution) {
		statusChart.updateOptions({
			colors: Object.keys(data.requests.status_distribution).map((code) =>
				getStatusCodeColor(code)
			),
			labels: Object.keys(data.requests.status_distribution).map((code) => `${code} Status`)
		});
		statusChart.updateSeries(Object.values(data.requests.status_distribution));
	}

	$: if (pathDistributionChart && data.requests?.path_distribution) {
		const sortedPaths = Object.entries(data.requests.path_distribution)
			.sort((a, b) => b[1] - a[1])
			.slice(0, 10);

		pathDistributionChart.updateSeries([
			{
				name: 'Requests',
				data: sortedPaths.map(([_, count]) => count)
			}
		]);

		pathDistributionChart.updateOptions({
			xaxis: {
				categories: sortedPaths.map(([path]) => path)
			}
		});
	}

	$: if (storageDistributionChart && data.storage?.file_size_distribution) {
		storageDistributionChart.updateSeries(Object.values(data.storage.file_size_distribution));
	}

	$: if (uploadHistoryChart && data.uploadHistory) {
    const filledData = fillMissingDates(data.uploadHistory, dateRange);

    uploadHistoryChart.updateSeries([
        {
            name: 'File Count',
            data: filledData.map(item => ({
                x: new Date(item.date).getTime(),
                y: item.file_count
            }))
        },
        {
            name: 'Total Size (MB)',
            data: filledData.map(item => ({
                x: new Date(item.date).getTime(),
                y: Math.round(item.total_size / (1024 * 1024) * 100) / 100
            }))
        }
    ]);
}
</script>

<div class="dashboard">
	<div class="header">
		<div class="header-content">
			<h1 class="title">System Metrics</h1>
			<div class="range-selector">
				{#each rangeOptions as option}
					<button
						class="range-btn {dateRange === option.value ? 'active' : ''}"
						on:click={() => { dateRange = option.value; handleRangeChange(); }}
					>
						{option.label}
					</button>
				{/each}
			</div>
		</div>
	</div>

	{#if data.error}
		<div class="error-container">
			<div class="error">
				<svg xmlns="http://www.w3.org/2000/svg" class="error-icon" viewBox="0 0 24 24" stroke="currentColor" fill="none">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<span>{data.error}</span>
			</div>
		</div>
	{:else}
		<div class="metrics-grid">
			<div class="metrics-card">
				<div class="metric">
					<h3>Files</h3>
					<div class="value">{data.storage?.current_files?.toLocaleString() || 0}</div>
				</div>
				<div class="growth">
					<span class="positive">+5.2%</span>
					<small>vs. previous period</small>
				</div>
			</div>
			<div class="metrics-card">
				<div class="metric">
					<h3>Storage</h3>
					<!-- <div class="value">{formatBytes(data.storage?.available_size_bytes || 0)} Available</div> -->
				</div>
				<div class="storage-bar">
					<div
						class="storage-segment files"
						style="width: {data.storage ? (data.storage.current_size_bytes / data.storage.system_total_size_bytes * 100) : 0}%"
						title="Files: {formatBytes(data.storage?.current_size_bytes || 0)}"
					></div>
					<div
						class="storage-segment other"
						style="width: {data.storage ? ((data.storage.used_size_bytes - data.storage.current_size_bytes) / data.storage.system_total_size_bytes * 100) : 0}%"
						title="System & Other: {formatBytes((data.storage?.used_size_bytes || 0) - (data.storage?.current_size_bytes || 0))}
App storage: {formatBytes(data.storage?.current_size_bytes || 0)}
Available space: {formatBytes(data.storage?.available_size_bytes || 0)}
Total space: {formatBytes(data.storage?.total_size_bytes || 0)}
Usage: {Math.round((data.storage?.used_size_bytes || 0) / (data.storage?.system_total_size_bytes || 1) * 100)}% of total capacity"					></div>
					<div
						class="storage-segment free"
						style="width: {data.storage ? (data.storage.available_size_bytes / data.storage.system_total_size_bytes * 100) : 0}%"
						title="Free Space: {formatBytes(data.storage?.available_size_bytes || 0)}"
					></div>
				</div>
				<div class="storage-legend">
					<div class="legend-item">
						<span class="legend-color files"></span>
						<span class="legend-label">Files</span>
					</div>
					<div class="legend-item">
						<span class="legend-color other"></span>
						<span class="legend-label">System</span>
					</div>
					<div class="legend-item">
						<span class="legend-color free"></span>
						<span class="legend-label">Free</span>
					</div>
				</div>
				<div class="storage-details">
					{formatBytes(data.storage?.system_total_size_bytes || 0)} total
				</div>
			</div>
			<div class="metrics-card">
				<div class="metric">
					<h3>Requests</h3>
					<div class="value">{data.requests?.total_requests?.toLocaleString() || 0}</div>
				</div>
				<div class="growth">
					<span class="positive">+12.8%</span>
					<small>vs. previous period</small>
				</div>
			</div>
			<div class="metrics-card">
				<div class="metric">
					<h3>Unique IPs</h3>
					<div class="value">{data.requests?.unique_ips?.toLocaleString() || 0}</div>
				</div>
				<div class="growth">
					<span class="positive">+8.3%</span>
					<small>vs. previous period</small>
				</div>
			</div>
			<div class="metrics-card">
				<div class="metric">
					<h3>App storage now</h3>
					<div class="value">{formatBytes(data.storage?.current_size_bytes)} </div>
				</div>
				<div class="growth">
					<span class="positive">+8.3%</span>
					<small>vs. previous period</small>
				</div>
			</div>
			<div class="metrics-card">
				<div class="metric">
					<h3>Total storage accumulated</h3>
					<div class="value">{formatBytes(data.storage?.total_size_bytes)} </div>
				</div>
				<div class="growth">
					<span class="positive">+8.3%</span>
					<small>vs. previous period</small>
				</div>
			</div>
		</div>

		<div class="charts-grid">
			<div class="chart-card full-width">
				<div class="chart-header">
					<h2>Activity Overview</h2>
				</div>
				<div class="chart-body">
					<div bind:this={timeSeriesChartElement} class="chart"></div>
				</div>
			</div>

			<!-- New Upload History Chart -->
			<div class="chart-card full-width">
				<div class="chart-header">
					<h2>Daily Upload Trends</h2>
				</div>
				<div class="chart-body">
					<div bind:this={uploadHistoryChartElement} class="chart"></div>
				</div>
			</div>

			<div class="chart-card">
				<div class="chart-header">
					<h2>Status Codes</h2>
				</div>
				<div class="chart-body">
					<div bind:this={statusChartElement} class="chart"></div>
				</div>
			</div>

			<div class="chart-card">
				<div class="chart-header">
					<h2>Top Requested Paths</h2>
				</div>
				<div class="chart-body">
					<div bind:this={pathDistributionElement} class="chart"></div>
				</div>
			</div>

			<div class="chart-card">
				<div class="chart-header">
					<h2>File Size Distribution</h2>
				</div>
				<div class="chart-body">
					<div bind:this={storageDistributionElement} class="chart"></div>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>

	.growth {
		display: none !important;
	}

	:global(body) {
		background-color: #f8fafc;
		margin: 0;
		font-family: 'Inter var', system-ui, -apple-system, sans-serif;
	}

	.dashboard {
		max-width: 1600px;
		margin: 0 auto;
		padding: 2rem;
	}

	.header {
		margin-bottom: 1.5rem;
	}

	.header-content {
		display: flex;
		justify-content: space-between;
		align-items: center;
		flex-wrap: wrap;
		gap: 1rem;
	}

	.title {
		font-size: 1.75rem;
		font-weight: 600;
		color: #0f172a;
		margin: 0;
	}

	.range-selector {
		display: flex;
		gap: 0.5rem;
		background: white;
		border-radius: 0.5rem;
		padding: 0.25rem;
		box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
	}

	.range-btn {
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		background: transparent;
		border: none;
		border-radius: 0.375rem;
		color: #64748b;
		cursor: pointer;
		transition: all 0.2s;
	}

	.range-btn.active {
		background-color: #f1f5f9;
		color: #0f172a;
		font-weight: 500;
	}

	.range-btn:hover:not(.active) {
		background-color: #f8fafc;
		color: #334155;
	}

	.error-container {
		display: flex;
		justify-content: center;
		margin: 2rem 0;
	}

	.error {
		background-color: #fee2e2;
		color: #b91c1c;
		border-radius: 0.5rem;
		padding: 1rem 1.5rem;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		max-width: 500px;
	}

	.error-icon {
		width: 1.5rem;
		height: 1.5rem;
	}

	.metrics-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	@media (max-width: 1200px) {
		.metrics-grid {
			grid-template-columns: repeat(2, 1fr);
		}
	}

	@media (max-width: 640px) {
		.metrics-grid {
			grid-template-columns: 1fr;
		}
	}

	.metrics-card {
		background: white;
		border-radius: 0.75rem;
		padding: 1.25rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
	}

	.metric {
		margin-bottom: 0.75rem;
	}

	.metric h3 {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 500;
		color: #64748b;
		margin-bottom: 0.5rem;
	}

	.value {
		font-size: 1.5rem;
		font-weight: 600;
		color: #0f172a;
	}

	.growth {
		display: flex;
		flex-direction: column;
		font-size: 0.875rem;
	}

	.positive {
		color: #16a34a;
	}

	.negative {
		color: #dc2626;
	}

	.growth small {
		color: #64748b;
		font-size: 0.75rem;
	}

	.progress-bar {
		width: 100%;
		height: 0.5rem;
		background: #e2e8f0;
		border-radius: 1rem;
		margin: 0.75rem 0 0.5rem;
		overflow: hidden;
	}

	.progress {
		height: 100%;
		background: #3b82f6;
		border-radius: 1rem;
	}

	.progress-label {
		font-size: 0.75rem;
		color: #64748b;
	}

	/* New macOS-style storage bar styles */
	.storage-bar {
		width: 100%;
		height: 0.75rem;
		border-radius: 0.5rem;
		margin: 0.75rem 0 0.5rem;
		overflow: hidden;
		display: flex;
		position: relative;
	}

	.storage-segment {
		height: 100%;
		transition: all 0.2s;
		position: relative;
		cursor: pointer;
	}

	.storage-segment:hover {
		opacity: 0.8;
	}

	.storage-segment.files {
		background: #40B87B;
	}

	.storage-segment.other {
		background: #F38D68;
	}

	.storage-segment.free {
		background: #e2e8f0;
	}

	.storage-legend {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		margin-top: 0.5rem;
		font-size: 0.75rem;
		color: #64748b;
	}

	.legend-item {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}

	.legend-color {
		width: 0.75rem;
		height: 0.75rem;
		border-radius: 50%;
	}

	.legend-color.files {
		background: #40B87B;
	}

	.legend-color.other {
		background: #F38D68;
	}

	.legend-color.free {
		background: #e2e8f0;
	}

	.storage-details {
		font-size: 0.75rem;
		color: #64748b;
		margin-top: 0.5rem;
		text-align: right;
	}

	.charts-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1.5rem;
	}

	@media (max-width: 1024px) {
		.charts-grid {
			grid-template-columns: 1fr;
		}
	}

	.chart-card {
		background: white;
		border-radius: 0.75rem;
		overflow: hidden;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
	}

	.full-width {
		grid-column: span 2;
	}

	@media (max-width: 1024px) {
		.full-width {
			grid-column: span 1;
		}
	}

	.chart-header {
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid #f1f5f9;
	}

	.chart-header h2 {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
		color: #0f172a;
	}

	.chart-body {
		padding: 1rem;
	}

	.chart {
		width: 100%;
		height: 100%;
	}
</style>