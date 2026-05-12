/**
 * Normalize a file's MIME type based on its extension.
 *
 * Browsers are inconsistent with `File.type` — they often return
 * `application/octet-stream` or `text/plain` for files they don't
 * explicitly know (e.g. `.md`, `.rs`, `.zig`). This function ensures
 * the stored `contentType` metadata is always accurate.
 *
 * Prefers `file.type` when it is already a well-known non-generic type,
 * otherwise resolves from extension.
 */

const EXTENSION_MIME: Record<string, string> = {
	// Text / markup
	txt: 'text/plain',
	md: 'text/markdown',
	markdown: 'text/markdown',
	rst: 'text/prs.fallenstein.rst',
	rtf: 'application/rtf',

	// Data / config
	json: 'application/json',
	jsonl: 'application/jsonl',
	xml: 'application/xml',
	yaml: 'text/yaml',
	yml: 'text/yaml',
	toml: 'application/toml',
	csv: 'text/csv',
	tsv: 'text/tab-separated-values',
	ini: 'text/plain',
	conf: 'text/plain',
	cfg: 'text/plain',
	env: 'text/plain',
	gradle: 'text/plain',
	properties: 'text/plain',
	lock: 'text/plain',

	// Shell / scripting
	sh: 'application/x-sh',
	bash: 'application/x-sh',
	zsh: 'application/x-zsh',
	fish: 'application/x-fish',
	ps1: 'application/x-powershell',

	// Code
	js: 'text/javascript',
	mjs: 'text/javascript',
	ts: 'text/typescript',
	jsx: 'text/jsx',
	tsx: 'text/tsx',
	lua: 'text/x-lua',
	r: 'text/x-r',
	rmd: 'text/x-rmarkdown',

	// Web
	html: 'text/html',
	htm: 'text/html',
	css: 'text/css',
	scss: 'text/x-scss',

	// Component frameworks
	svelte: 'text/x-svelte',
	vue: 'text/x-vue',
	astro: 'text/x-astro',

	// Compiled / other languages
	py: 'text/x-python',
	rb: 'text/x-ruby',
	php: 'text/x-php',
	java: 'text/x-java-source',
	go: 'text/x-go',
	rs: 'text/x-rust',
	c: 'text/x-c',
	cc: 'text/x-c++',
	cpp: 'text/x-c++',
	h: 'text/x-c',
	hpp: 'text/x-c++',
	kt: 'text/x-kotlin',
	kts: 'text/x-kotlin',
	swift: 'text/x-swift',
	dart: 'text/x-dart',
	sql: 'text/x-sql',
	scala: 'text/x-scala',
	ex: 'text/x-elixir',
	exs: 'text/x-elixir',
	hs: 'text/x-haskell',
	zig: 'text/x-zig',
	pl: 'text/x-perl',
	pm: 'text/x-perl',

	// Patch / diff
	diff: 'text/x-patch',
	patch: 'text/x-patch',

	// Log
	log: 'text/plain'
};

/**
 * MIME types that are clearly "generic" and carry no useful type information.
 * When we see these we always fall back to extension-based detection.
 */
const GENERIC_MIME_TYPES = new Set(['application/octet-stream', 'text/plain']);

/**
 * Check if a MIME type is a well-known text-based type.
 */
function isTextMime(type: string): boolean {
	const lower = type.toLowerCase().split(';', 1)[0].trim();
	if (lower.startsWith('text/')) return true;
	const subtype = lower.includes('/') ? lower.split('/')[1] : lower;

	const textSubtypes = new Set([
		'json',
		'xml',
		'yaml',
		'javascript',
		'ecmascript',
		'toml',
		'markdown',
		'rtf',
		'x-sh',
		'x-shellscript',
		'x-httpd-php',
		'x-python',
		'x-sql',
		'x-perl',
		'x-ruby',
		'x-java',
		'x-c',
		'x-c++',
		'x-go',
		'x-rust',
		'x-lua',
		'x-r',
		'x-scss',
		'x-svelte',
		'x-vue',
		'x-astro',
		'x-powershell',
		'x-fish',
		'x-zsh',
		'x-scala',
		'x-elixir',
		'x-haskell',
		'x-zig',
		'x-kotlin',
		'x-swift',
		'x-dart',
		'x-patch'
	]);

	if (textSubtypes.has(subtype)) return true;
	return ['json', 'xml', 'yaml'].some((suffix) => subtype.endsWith(`+${suffix}`));
}

/**
 * Resolve the file extension (lowercased) from a filename.
 */
function getExtension(filename: string): string {
	const dotIndex = filename.lastIndexOf('.');
	if (dotIndex < 0) return '';
	return filename.slice(dotIndex + 1).toLowerCase();
}

/**
 * Normalize the MIME type for a file so the stored metadata is reliable.
 *
 * Strategy:
 * 1. If `file.type` is already specific (not generic), use it.
 * 2. Otherwise, look up the correct MIME type from the extension map.
 * 3. Fall back to `file.type` (could be a correct but unknown type).
 * 4. Final fallback: `application/octet-stream`.
 */
export function normalizeMimeType(file: File | { name: string; type: string }): string {
	const browserType = file.type?.toLowerCase().split(';', 1)[0].trim() || '';
	const ext = getExtension(file.name);

	// If browser already gave us a specific, non-generic type, trust it
	if (browserType && !GENERIC_MIME_TYPES.has(browserType)) {
		return browserType;
	}

	// Extension-based lookup
	const resolved = EXTENSION_MIME[ext];
	if (resolved) return resolved;

	// Fallback to whatever the browser said
	if (browserType) return browserType;

	return 'application/octet-stream';
}

/**
 * Check if a MIME type indicates a text-based file (for preview decisions).
 */
export function isTextBased(type: string): boolean {
	return isTextMime(type.toLowerCase());
}
