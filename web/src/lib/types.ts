export interface ActivitySummary {
	period: string;
	uploads: number;
	downloads: number;
	unique_visitors: number;
}

export interface TopIPMetrics {
	ip: string;
	request_count: number;
	error_count: number;
}

export interface TimeDistributionData {
	date: string;
	count: number;
}

export interface SecurityMetrics {
	period: string;
	status_codes: Record<number, number>;
	total_requests: number;
	failed_requests: number;
	unique_ips: number;
	top_ips: TopIPMetrics[];
	average_latency: number;
}

export interface StorageSummary {
	system_total_size_bytes: number;
	total_files: number;
	total_size_bytes: number;
	current_files: number;
	current_size_bytes: number;
	file_size_distribution: Record<string, number>;
}

export interface RequestMetrics {
	total_requests: number;
	unique_ips: number;
	average_latency_ms: number;
	status_distribution: Record<number, number>;
	top_ips: TopIPMetrics[];
	time_distribution: TimeDistributionData[];
}

export interface UploadHistoryItem {
	date: string;
	file_count: number;
	total_size: number;
}

export interface PageData {
	activity: ActivitySummary[];
	metrics: SecurityMetrics;
	storage: StorageSummary;
	requests: RequestMetrics;
	uploadHistory: UploadHistoryItem[];
	range: string;
	error?: string;
}
