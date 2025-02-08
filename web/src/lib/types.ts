export interface StorageSummary {
  total_files: number;
  total_size_bytes: number;
  current_files: number;
  current_size_bytes: number;
  file_size_distribution: Record<string, number>;
}