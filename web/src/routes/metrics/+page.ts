import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const range = url.searchParams.get('range') || '7d';

	try {
		const [securityRes, activityRes, storageRes] = await Promise.all([
			fetch(`/api/metrics/security?range=${range}`),
			fetch(`/api/metrics/activity?range=${range}`),
			fetch(`/api/metrics/storage?range=${range}`)
		]);

		if (!securityRes.ok || !activityRes.ok || !storageRes.ok) {
			throw new Error('Failed to fetch metrics');
		}

		return {
			metrics: await securityRes.json(),
			activity: await activityRes.json(),
			storage: await storageRes.json(),
			range
		};
	} catch (e) {
		return {
			metrics: null,
			activity: [],
			storage: null,
			range,
			error: 'Failed to load metrics data'
		};
	}
};
