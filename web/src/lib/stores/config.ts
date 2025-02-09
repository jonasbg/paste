import { writable } from 'svelte/store';

interface Config {
    max_file_size: string;
    id_size: string;
    key_size: string;
}

interface ConfigStore {
    loading: boolean;
    error: string | null;
    data: Config | null;
}

function createConfigStore() {
    const { subscribe, set, update } = writable<ConfigStore>({
        loading: false,
        error: null,
        data: null
    });

    async function fetchConfig() {
        update(state => ({ ...state, loading: true, error: null }));

        try {
            const response = await fetch('/api/config');

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            const config: Config = await response.json();
            update(state => ({
                loading: false,
                error: null,
                data: config
            }));
        } catch (error) {
            update(state => ({
                loading: false,
                error: error instanceof Error ? error.message : 'Failed to fetch config',
                data: null
            }));
        }
    }

    return {
        subscribe,
        fetch: fetchConfig
    };
}

export const configStore = createConfigStore();

// Initialize the store when the app starts
if (typeof window !== 'undefined') {
    configStore.fetch();
}