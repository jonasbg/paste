import { configStore } from '$lib/stores/config';
import { get } from 'svelte/store';

export interface ProgressCallback {
    (progress: number, message: string): Promise<void>;
}

export class FileProcessor {
    static formatFileSize(bytes: number): string {
        const units = ['B', 'KB', 'MB', 'GB'];
        let size = bytes;
        let unitIndex = 0;
        while (size >= 1024 && unitIndex < units.length - 1) {
            size /= 1024;
            unitIndex++;
        }
        return `${size.toFixed(2)} ${units[unitIndex]}`;
    }

    private parseFileSize(sizeStr: string): number {
        const units: { [key: string]: number } = {
            'B': 1,
            'KB': 1024,
            'MB': 1024 * 1024,
            'GB': 1024 * 1024 * 1024,
            'TB': 1024 * 1024 * 1024 * 1024,
        };
        const match = sizeStr.match(/^([\d.]+)\s*([KMGT]?B)$/i);
        if (!match) {
            throw new Error(`Invalid file size format: ${sizeStr}`);
        }
        const [, size, unit] = match;
        return parseFloat(size) * (units[unit.toUpperCase()] || 1);
    }

    getMaxFileSize(): number {
        const config = get(configStore);
        if (!config.data) {
            throw new Error('Config has not been loaded. Please wait for config to load before processing files.');
        }
        return this.parseFileSize(config.data.max_file_size);
    }

}