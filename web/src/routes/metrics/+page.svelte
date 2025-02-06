<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { parse, format } from 'date-fns';
  import ApexCharts from 'apexcharts';
  import { onMount } from 'svelte';
  import type { ActivitySummary, SecurityMetrics, StorageSummary } from '$lib/types';

  export let data: {
    activity: ActivitySummary[];
    metrics: SecurityMetrics;
    storage: StorageSummary;
    range: string;
    error?: string;
  };

  let dateRange = data.range;
  let activityChartElement: HTMLElement;
  let statusChartElement: HTMLElement;
  let topIPsChartElement: HTMLElement;
  let activityChart: ApexCharts;
  let statusChart: ApexCharts;
  let topIPsChart: ApexCharts;

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
    if (num < 300) return '#00E396';
    if (num < 400) return '#FEB019';
    if (num < 500) return '#FF4560';
    return '#775DD0';
  }

  function getStatusCodeLabel(code: string) {
    const num = parseInt(code);
    if (num < 300) return 'Success';
    if (num < 400) return 'Redirect';
    if (num < 500) return 'Client Error';
    return 'Server Error';
  }

  onMount(() => {
    if (data.activity && data.metrics) {
      // Activity Timeline Chart
      const activityOptions = {
        chart: {
          type: 'line',
          height: 380,
          fontFamily: '-apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
          toolbar: {
            show: true
          }
        },
        colors: ['#2E93fA', '#66DA26', '#546E7A'],
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
            formatter: (val: string) => format(new Date(parseInt(val)), 'MMM d')
          }
        },
        stroke: {
          curve: 'smooth',
          width: 2
        }
      };

      // Status codes donut chart
      const statusOptions = {
        chart: {
          type: 'donut',
          height: 380
        },
        series: Object.values(data.metrics.status_codes),
        labels: Object.keys(data.metrics.status_codes).map(code => `${code} ${getStatusCodeLabel(code)}`),
        colors: Object.keys(data.metrics.status_codes).map(code => getStatusCodeColor(code)),
        legend: {
          position: 'bottom'
        }
      };

      // Top IPs bar chart
      const topIPsOptions = {
        chart: {
          type: 'bar',
          height: 380
        },
        series: [{
          name: 'Total Requests',
          data: data.metrics.top_ips.map(ip => ip.requests)
        }, {
          name: 'Failed Requests',
          data: data.metrics.top_ips.map(ip => ip.failures)
        }],
        xaxis: {
          categories: data.metrics.top_ips.map(ip => ip.ip),
          labels: {
            rotate: -45
          }
        },
        colors: ['#2E93fA', '#ff4560']
      };

      activityChart = new ApexCharts(activityChartElement, activityOptions);
      statusChart = new ApexCharts(statusChartElement, statusOptions);
      topIPsChart = new ApexCharts(topIPsChartElement, topIPsOptions);

      activityChart.render();
      statusChart.render();
      topIPsChart.render();
    }

    return () => {
      activityChart?.destroy();
      statusChart?.destroy();
      topIPsChart?.destroy();
    };
  });

  // Reactive statements for updating charts when data changes
  $: if (activityChart && data.activity) {
    activityChart.updateSeries([
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
    ]);
  }

  $: if (statusChart && data.metrics?.status_codes) {
    statusChart.updateSeries(
      [Object.values(data.metrics.status_codes)].flat()
    );
    statusChart.updateOptions({
      labels: Object.keys(data.metrics.status_codes).map(code => `${code} ${getStatusCodeLabel(code)}`),
      colors: Object.keys(data.metrics.status_codes).map(code => getStatusCodeColor(code))
    });
  }

  $: if (topIPsChart && data.metrics?.top_ips) {
    topIPsChart.updateSeries([
      {
        name: 'Total Requests',
        data: data.metrics.top_ips.map(ip => ip.requests)
      },
      {
        name: 'Failed Requests',
        data: data.metrics.top_ips.map(ip => ip.failures)
      }
    ]);
    topIPsChart.updateOptions({
      xaxis: {
        categories: data.metrics.top_ips.map(ip => ip.ip)
      }
    });
  }
</script>

<style>
  .dashboard {
    padding: 2rem;
    max-width: 1400px;
    margin: 0 auto;
    background-color: #f5f5f5;
    min-height: 100vh;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 2rem;
  }

  .title {
    font-size: 1.5rem;
    font-weight: 600;
    color: #333;
  }

  select {
    padding: 0.5rem 1rem;
    border: 1px solid #ddd;
    border-radius: 4px;
    background-color: white;
    font-size: 0.9rem;
  }

  .metrics-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin-bottom: 2rem;
  }

  .metric-card {
    background: white;
    border-radius: 8px;
    padding: 1.5rem;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  }

  .metric-card-header {
    display: flex;
    align-items: center;
    margin-bottom: 1rem;
  }

  .metric-card-icon {
    width: 40px;
    height: 40px;
    background: #f0f0f0;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-right: 1rem;
  }

  .metric-card-title {
    font-size: 0.875rem;
    color: #666;
    margin: 0;
  }

  .metric-card-value {
    font-size: 1.5rem;
    font-weight: 600;
    color: #333;
  }

  .metric-card-subvalue {
    font-size: 0.875rem;
    color: #666;
    margin-left: 0.5rem;
  }

  .charts-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 1rem;
    margin-bottom: 2rem;
  }

  .chart-container {
    background: white;
    border-radius: 8px;
    padding: 1.5rem;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  }

  .chart-title {
    font-size: 1.125rem;
    color: #333;
    margin: 0 0 1rem 0;
  }

  .error {
    background-color: #fee2e2;
    border: 1px solid #fecaca;
    padding: 1rem;
    border-radius: 4px;
    color: #dc2626;
  }

  @media (min-width: 1024px) {
    .charts-grid {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>

<div class="dashboard">
  <div class="header">
    <h1 class="title">Unified Metrics Dashboard</h1>
    <select bind:value={dateRange} on:change={handleRangeChange}>
      {#each rangeOptions as option}
        <option value={option.value}>{option.label}</option>
      {/each}
    </select>
  </div>

  {#if data.error}
    <div class="error">{data.error}</div>
  {:else}
    <div class="metrics-grid">
      <!-- Security Metrics -->
      <div class="metric-card">
        <div class="metric-card-header">
          <div class="metric-card-icon">🔒</div>
          <h3 class="metric-card-title">Total Requests</h3>
        </div>
        <span class="metric-card-value">{data.metrics?.total_requests || 0}</span>
      </div>

      <div class="metric-card">
        <div class="metric-card-header">
          <div class="metric-card-icon">⚠️</div>
          <h3 class="metric-card-title">Failed Requests</h3>
        </div>
        <span class="metric-card-value">{data.metrics?.failed_requests || 0}</span>
      </div>

      <!-- Storage Metrics -->
      <div class="metric-card">
        <div class="metric-card-header">
          <div class="metric-card-icon">📁</div>
          <h3 class="metric-card-title">Current Files</h3>
        </div>
        <div>
          <span class="metric-card-value">{data.storage?.current_files || 0}</span>
          <span class="metric-card-subvalue">({formatBytes(data.storage?.current_size || 0)})</span>
        </div>
      </div>

      <div class="metric-card">
        <div class="metric-card-header">
          <div class="metric-card-icon">👥</div>
          <h3 class="metric-card-title">Unique Visitors</h3>
        </div>
        <span class="metric-card-value">{data.storage?.total_unique_visitors || 0}</span>
      </div>
    </div>

    <div class="charts-grid">
      <div class="chart-container">
        <h2 class="chart-title">Activity Timeline</h2>
        <div bind:this={activityChartElement}></div>
      </div>

      <div class="chart-container">
        <h2 class="chart-title">Status Code Distribution</h2>
        <div bind:this={statusChartElement}></div>
      </div>

      <div class="chart-container">
        <h2 class="chart-title">Top IP Addresses</h2>
        <div bind:this={topIPsChartElement}></div>
      </div>
    </div>
  {/if}
</div>