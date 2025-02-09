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
		RequestMetrics
	} from '$lib/types';

	export let data: {
		activity: ActivitySummary[];
		metrics: SecurityMetrics;
		storage: StorageSummary;
		requests: RequestMetrics;
		range: string;
		error?: string;
	};

	let dateRange = data.range;
	let timeSeriesChartElement: HTMLElement;
	let statusChartElement: HTMLElement;
	let pathDistributionElement: HTMLElement;
	let requestsTimelineElement: HTMLElement;
	let latencyTimelineElement: HTMLElement;
	let storageDistributionElement: HTMLElement;

	let timeSeriesChart: ApexCharts;
	let statusChart: ApexCharts;
	let pathDistributionChart: ApexCharts;
	let requestsTimelineChart: ApexCharts;
	let latencyTimelineChart: ApexCharts;
	let storageDistributionChart: ApexCharts;

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

	async function handleRangeChange() {
		const searchParams = new URLSearchParams($page.url.searchParams);
		searchParams.set('range', dateRange);
		goto(`?${searchParams.toString()}`, { replaceState: true });
	}

	function getStatusCodeColor(code: string) {
		const num = parseInt(code);
		if (num < 300) return '#10B981'; // Green
		if (num < 400) return '#F59E0B'; // Yellow
		if (num < 500) return '#EF4444'; // Red
		return '#8B5CF6'; // Purple
	}

	onMount(() => {
		if (data.activity && data.metrics && data.requests) {
			// Main Time Series Chart (Unified view)
			const timeSeriesOptions = {
				chart: {
					type: 'line',
					height: 400,
					fontFamily: 'Inter var, system-ui, -apple-system, sans-serif',
					animations: {
						enabled: false
					},
					toolbar: {
						show: true,
						tools: {
							download: true,
							selection: true,
							zoom: true,
							zoomin: true,
							zoomout: true,
							pan: true,
							reset: true
						}
					}
				},
				stroke: {
					curve: 'smooth',
					width: 2
				},
				colors: ['#10B981', '#3B82F6', '#8B5CF6'],
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
					position: 'top'
				}
			};

			// Status Distribution Chart
			const statusDistributionOptions = {
				chart: {
					type: 'donut',
					height: 300
				},
				colors: Object.keys(data.requests.status_distribution).map((code) =>
					getStatusCodeColor(code)
				),
				series: Object.values(data.requests.status_distribution),
				labels: Object.keys(data.requests.status_distribution).map((code) => `${code} Status`),
				legend: {
					position: 'bottom',
					formatter: function (label: string, opts) {
						return `${label} (${opts.w.globals.series[opts.seriesIndex]})`;
					}
				},
				plotOptions: {
					pie: {
						donut: {
							labels: {
								show: true,
								total: {
									show: true,
									label: 'Total Requests',
									formatter: function (w) {
										return w.globals.seriesTotals.reduce((a, b) => a + b, 0);
									}
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
					height: 300
				},
				plotOptions: {
					bar: {
						horizontal: true,
						borderRadius: 4
					}
				},
				colors: ['#3B82F6'],
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
				}
			};

			// Latency Timeline Chart
			const latencyTimelineOptions = {
				chart: {
					type: 'line',
					height: 300,
					animations: {
						enabled: false
					}
				},
				series: [
					{
						name: 'Average Latency (ms)',
						data: data.requests.time_distribution.map((d) => ({
							x: d.date,
							y: data.requests.average_latency_ms
						}))
					}
				],
				stroke: {
					curve: 'smooth',
					width: 2
				},
				colors: ['#F59E0B'],
				xaxis: {
					type: 'datetime'
				},
				yaxis: {
					labels: {
						formatter: (val: number) => `${Math.round(val)}ms`
					}
				}
			};

			// Storage Distribution Chart
			const storageDistributionOptions = {
				chart: {
					type: 'pie',
					height: 300
				},
				series: Object.values(data.storage.file_size_distribution),
				labels: Object.keys(data.storage.file_size_distribution),
				colors: ['#93C5FD', '#60A5FA', '#3B82F6', '#2563EB'],
				legend: {
					position: 'bottom',
					formatter: function (label: string, opts) {
						const value = opts.w.globals.series[opts.seriesIndex];
						return `${label}: ${value} files`;
					}
				}
			};

			timeSeriesChart = new ApexCharts(timeSeriesChartElement, timeSeriesOptions);
			statusChart = new ApexCharts(statusChartElement, statusDistributionOptions);
			pathDistributionChart = new ApexCharts(pathDistributionElement, pathDistributionOptions);
			latencyTimelineChart = new ApexCharts(latencyTimelineElement, latencyTimelineOptions);
			storageDistributionChart = new ApexCharts(
				storageDistributionElement,
				storageDistributionOptions
			);

			timeSeriesChart.render();
			statusChart.render();
			pathDistributionChart.render();
			latencyTimelineChart.render();
			storageDistributionChart.render();
		}

		return () => {
			timeSeriesChart?.destroy();
			statusChart?.destroy();
			pathDistributionChart?.destroy();
			latencyTimelineChart?.destroy();
			storageDistributionChart?.destroy();
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

	$: if (latencyTimelineChart && data.requests?.time_distribution) {
		latencyTimelineChart.updateSeries([
			{
				name: 'Average Latency (ms)',
				data: data.requests.time_distribution.map((d) => ({
					x: d.date,
					y: data.requests.average_latency_ms
				}))
			}
		]);
	}

	$: if (storageDistributionChart && data.storage?.file_size_distribution) {
		storageDistributionChart.updateSeries(Object.values(data.storage.file_size_distribution));
	}
</script>

<div class="dashboard">
	<div class="header">
		<div class="header-content">
			<h1 class="title">System Metrics Dashboard</h1>
			<select bind:value={dateRange} on:change={handleRangeChange}>
				{#each rangeOptions as option}
					<option value={option.value}>{option.label}</option>
				{/each}
			</select>
		</div>
	</div>

	{#if data.error}
		<div class="error">{data.error}</div>
	{:else}
		<div class="metrics-summary">
			<div class="metric-card">
				<h3>Current Files</h3>
				<div class="value">{data.storage?.current_files?.toLocaleString() || 0}</div>
			</div>
			<div class="metric-card">
				<h3>Current Storage</h3>
				<div class="value">{formatBytes(data.storage?.current_size_bytes || 0)}</div>
			</div>
			<div class="metric-card">
				<h3>Average File Size</h3>
				<div class="value">
					{data.storage?.current_files
						? formatBytes(data.storage.current_size_bytes / data.storage.current_files)
						: '0 B'}
				</div>
			</div>
			<div class="metric-card">
				<h3>Largest Size Bucket</h3>
				<div class="value">
					{#if data.storage?.file_size_distribution}
						{Object.entries(data.storage.file_size_distribution).reduce((a, b) =>
							a[1] > b[1] ? a : b
						)[0]}
					{:else}
						'N/A'
					{/if}
				</div>
			</div>

			<div class="metric-card">
				<h3>Total Requests</h3>
				<div class="value">{data.requests?.total_requests?.toLocaleString() || 0}</div>
			</div>
			<div class="metric-card">
				<h3>Unique IPs</h3>
				<div class="value">{data.requests?.unique_ips?.toLocaleString() || 0}</div>
			</div>
			<div class="metric-card">
				<h3>Avg Latency</h3>
				<div class="value">{Math.round(data.requests?.average_latency_ms || 0)}ms</div>
			</div>
			<div class="metric-card">
				<h3>Total Storage</h3>
				<div class="value">{formatBytes(data.storage?.total_size_bytes || 0)}</div>
			</div>
		</div>

		<div class="charts-grid">
			<div class="chart-container full-width">
				<h2>Activity Overview</h2>
				<div bind:this={timeSeriesChartElement}></div>
			</div>

			<div class="chart-container">
				<h2>Request Status Distribution</h2>
				<div bind:this={statusChartElement}></div>
			</div>

			<div class="chart-container">
				<h2>Top Requested Paths</h2>
				<div bind:this={pathDistributionElement}></div>
			</div>

			<div class="chart-container">
				<h2>Response Latency Trend</h2>
				<div bind:this={latencyTimelineElement}></div>
			</div>

			<div class="chart-container">
				<h2>File Size Distribution</h2>
				<div bind:this={storageDistributionElement}></div>
			</div>
		</div>
	{/if}
</div>

<style>
	.dashboard {
		@apply p-6 max-w-[1600px] mx-auto bg-gray-50 min-h-screen;
	}

	.header {
		@apply mb-6 bg-white rounded-lg shadow-sm p-4;
	}

	.header-content {
		@apply flex justify-between items-center;
	}

	.title {
		@apply text-2xl font-semibold text-gray-900;
	}

	select {
		@apply px-4 py-2 border border-gray-200 rounded-lg bg-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500;
	}

	.metrics-summary {
		@apply grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6;
	}

	.metric-card {
		@apply bg-white p-6 rounded-lg shadow-sm;
	}

	.metric-card h3 {
		@apply text-sm font-medium text-gray-500 mb-2;
	}

	.metric-card .value {
		@apply text-2xl font-semibold text-gray-900;
	}

	.charts-grid {
		@apply grid grid-cols-1 lg:grid-cols-2 gap-6;
	}

	.chart-container {
		@apply bg-white p-6 rounded-lg shadow-sm;
	}

	.chart-container h2 {
		@apply text-lg font-medium text-gray-900 mb-4;
	}

	.full-width {
		@apply lg:col-span-2;
	}

	.error {
		@apply bg-red-50 border border-red-200 text-red-700 p-4 rounded-lg mb-6;
	}
</style>
