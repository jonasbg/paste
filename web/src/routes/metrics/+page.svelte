<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { parse, format } from 'date-fns';
	import ApexCharts from 'apexcharts';
	import { onMount } from 'svelte';
	import type { ActivitySummary, SecurityMetrics, StorageSummary, RequestMetrics } from '$lib/types';

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

	let timeSeriesChart: ApexCharts;
	let statusChart: ApexCharts;
	let pathDistributionChart: ApexCharts;
	let requestsTimelineChart: ApexCharts;
	let latencyTimelineChart: ApexCharts;

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
											data: data.activity.map(d => ({
													x: new Date(d.period).getTime(),
													y: d.uploads
											}))
									},
									{
											name: 'Downloads',
											data: data.activity.map(d => ({
													x: new Date(d.period).getTime(),
													y: d.downloads
											}))
									},
									{
											name: 'Unique Visitors',
											data: data.activity.map(d => ({
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

					// Requests Timeline Chart
					const requestsTimelineOptions = {
							chart: {
									type: 'area',
									height: 300,
									stacked: true,
									animations: {
											enabled: false
									}
							},
							colors: Object.keys(data.requests.status_distribution).map(code => getStatusCodeColor(code)),
							series: Object.entries(data.requests.status_distribution).map(([code, count]) => ({
									name: `${code} Status`,
									data: data.requests.time_distribution.map(d => ({
											x: d.date,
											y: count
									}))
							})),
							xaxis: {
									type: 'datetime'
							},
							yaxis: {
									labels: {
											formatter: (val: number) => Math.round(val)
									}
							},
							fill: {
									type: 'gradient',
									gradient: {
											opacityFrom: 0.6,
											opacityTo: 0.1
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
							series: [{
									name: 'Requests',
									data: Object.entries(data.requests.path_distribution)
											.sort((a, b) => b[1] - a[1])
											.slice(0, 10)
											.map(([path, count]) => count)
							}],
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
							series: [{
									name: 'Average Latency (ms)',
									data: data.requests.time_distribution.map(d => ({
											x: d.date,
											y: data.requests.average_latency_ms
									}))
							}],
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

					timeSeriesChart = new ApexCharts(timeSeriesChartElement, timeSeriesOptions);
					requestsTimelineChart = new ApexCharts(requestsTimelineElement, requestsTimelineOptions);
					pathDistributionChart = new ApexCharts(pathDistributionElement, pathDistributionOptions);
					latencyTimelineChart = new ApexCharts(latencyTimelineElement, latencyTimelineOptions);

					timeSeriesChart.render();
					requestsTimelineChart.render();
					pathDistributionChart.render();
					latencyTimelineChart.render();
			}

			return () => {
					timeSeriesChart?.destroy();
					requestsTimelineChart?.destroy();
					pathDistributionChart?.destroy();
					latencyTimelineChart?.destroy();
			};
	});
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
									<h3>Current Storage</h3>
									<div class="value">{formatBytes(data.storage?.current_size || 0)}</div>
							</div>
					</div>

					<div class="charts-grid">
							<div class="chart-container full-width">
									<h2>Activity Overview</h2>
									<div bind:this={timeSeriesChartElement}></div>
							</div>

							<div class="chart-container">
									<h2>Request Status Distribution</h2>
									<div bind:this={requestsTimelineElement}></div>
							</div>

							<div class="chart-container">
									<h2>Top Requested Paths</h2>
									<div bind:this={pathDistributionElement}></div>
							</div>

							<div class="chart-container">
									<h2>Response Latency Trend</h2>
									<div bind:this={latencyTimelineElement}></div>
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