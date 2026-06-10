import { browser } from '$lib/env';
import { derived, get, writable } from 'svelte/store';
import en from './en';
import no from './no';

export type Locale = 'en' | 'no';

const dictionaries: Record<Locale, Record<string, unknown>> = { en, no };

/**
 * Detect the locale automatically from the browser language: Norwegian for
 * nb/nn/no, English for everything else. There is no manual switch — the
 * language follows the visitor's browser.
 */
function detectLocale(): Locale {
	if (!browser) return 'en';
	const lang = (navigator.language || '').toLowerCase();
	return lang.startsWith('nb') || lang.startsWith('nn') || lang.startsWith('no') ? 'no' : 'en';
}

const initial = detectLocale();

export const locale = writable<Locale>(initial);

if (browser) {
	document.documentElement.lang = initial;
}

/** Resolve a dotted key (e.g. "preview.title") against a dictionary. */
function resolve(dict: Record<string, unknown>, key: string): string | undefined {
	const value = key
		.split('.')
		.reduce<unknown>(
			(acc, part) =>
				acc && typeof acc === 'object' ? (acc as Record<string, unknown>)[part] : undefined,
			dict
		);
	return typeof value === 'string' ? value : undefined;
}

/** Replace {name} placeholders with values from params. */
function interpolate(str: string, params?: Record<string, string | number>): string {
	if (!params) return str;
	return str.replace(/\{(\w+)\}/g, (match, name) => (name in params ? String(params[name]) : match));
}

export function translate(
	loc: Locale,
	key: string,
	params?: Record<string, string | number>
): string {
	const str = resolve(dictionaries[loc], key) ?? resolve(dictionaries.en, key) ?? key;
	return interpolate(str, params);
}

/** Reactive translator for components: `$t('key', params)`. */
export const t = derived(
	locale,
	($locale) =>
		(key: string, params?: Record<string, string | number>): string =>
			translate($locale, key, params)
);

/** Imperative translator for use outside components (services, plain modules). */
export function tr(key: string, params?: Record<string, string | number>): string {
	return translate(get(locale), key, params);
}
