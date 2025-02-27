export interface StorageSummary {
	total_files: number;
	total_size_bytes: number;
	current_files: number;
	current_size_bytes: number;
	file_size_distribution: Record<string, number>;
}

export interface UploadHistoryItem {
  date: string;
  file_count: number;
  total_size: number;
}

// Update your page data interface to include uploadHistory
export interface PageData {
  activity: ActivitySummary[];
  metrics: SecurityMetrics;
  storage: StorageSummary;
  requests: RequestMetrics;
  uploadHistory: UploadHistoryItem[];
  range: string;
  error?: string;
}