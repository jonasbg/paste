export const formatBytes = (bytes: number): string => {
	const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
	let value = bytes;
	let unit = 0;

	while (value >= 1024 && unit < units.length - 1) {
		value /= 1024;
		unit++;
	}

	return `${value.toFixed(2)} ${units[unit]}`;
};

export const formatNumber = (num: number): string => {
	return new Intl.NumberFormat().format(num);
};
