type FileMetadata = {
	filename?: string;
	contentType?: string;
};

export type RenderedTextPreview = {
	mode: 'pre' | 'table';
	html: string;
};

const CODE_EXTENSIONS = new Set([
	'js',
	'ts',
	'jsx',
	'tsx',
	'css',
	'scss',
	'html',
	'htm',
	'svelte',
	'py',
	'rb',
	'php',
	'java',
	'go',
	'rs',
	'c',
	'cc',
	'cpp',
	'h',
	'hpp',
	'sh',
	'bash',
	'zsh',
	'fish',
	'ps1',
	'sql'
]);

function escapeHtml(value: string): string {
	return value
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

function getFileExtension(filename: string | undefined): string {
	if (!filename || !filename.includes('.')) return '';
	return filename.split('.').pop()?.toLowerCase() || '';
}

function detectPreviewKind(fileMetadata: FileMetadata): string {
	const ext = getFileExtension(fileMetadata.filename);
	const contentType = fileMetadata.contentType?.toLowerCase() || '';

	if (ext === 'csv' || contentType.includes('csv')) return 'csv';
	if (ext === 'tsv' || contentType.includes('tab-separated-values')) return 'tsv';
	if (ext === 'json' || ext === 'jsonl' || contentType.includes('json')) return 'json';
	if (
		ext === 'xml' ||
		ext === 'html' ||
		ext === 'htm' ||
		ext === 'svg' ||
		contentType.includes('xml') ||
		contentType.includes('html')
	)
		return 'markup';
	if (ext === 'yaml' || ext === 'yml' || contentType.includes('yaml')) return 'yaml';
	if (ext === 'toml') return 'toml';
	if (ext === 'ini' || ext === 'conf' || ext === 'cfg' || ext === 'env') return 'ini';
	if (CODE_EXTENSIONS.has(ext) || contentType.includes('javascript')) return 'code';

	return 'text';
}

function tokenizeSource(
	source: string,
	pattern: RegExp,
	classify: (token: string, offset: number, source: string) => string | null
): string {
	let lastIndex = 0;
	let html = '';

	source.replace(pattern, (match: string, ...args: unknown[]) => {
		const offset = args[args.length - 2] as number;
		const className = classify(match, offset, source);
		html += escapeHtml(source.slice(lastIndex, offset));
		html += className
			? `<span class="${className}">${escapeHtml(match)}</span>`
			: escapeHtml(match);
		lastIndex = offset + match.length;
		return match;
	});

	html += escapeHtml(source.slice(lastIndex));
	return html;
}

function renderPlainPre(source: string): RenderedTextPreview {
	return {
		mode: 'pre',
		html: escapeHtml(source)
	};
}

function renderJson(source: string): RenderedTextPreview {
	let normalized = source;

	try {
		normalized = JSON.stringify(JSON.parse(source), null, 2);
	} catch {
		// Keep original text when it is truncated or otherwise invalid JSON.
	}

	const tokenized = tokenizeSource(
		normalized,
		/"(?:\\.|[^"\\])*"|\btrue\b|\bfalse\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|[{}\[\],:]/g,
		(token, offset, input) => {
			if (token.startsWith('"')) {
				return /^\s*:/.test(input.slice(offset + token.length)) ? 'tok-key' : 'tok-string';
			}

			if (token === 'true' || token === 'false') {
				return 'tok-bool';
			}

			if (token === 'null') {
				return 'tok-null';
			}

			if (/^-?\d/.test(token)) {
				return 'tok-number';
			}

			return 'tok-punct';
		}
	);

	return { mode: 'pre', html: tokenized };
}

function renderMarkup(source: string): RenderedTextPreview {
	const escaped = escapeHtml(source);
	const highlighted = escaped.replace(
		/(&lt;!--[\s\S]*?--&gt;)|(&lt;\/?)([A-Za-z0-9:_-]+)([^&]*?)(\/?&gt;)/g,
		(match, comment, open, tagName, attrs, close) => {
			if (comment) {
				return `<span class="tok-comment">${comment}</span>`;
			}

			const highlightedAttrs = attrs.replace(
				/([A-Za-z_:][-A-Za-z0-9_:.]*)(=)(&quot;.*?&quot;|&#39;.*?&#39;)/g,
				(
					_match: string,
					name: string,
					eq: string,
					value: string
				) =>
					`<span class="tok-attr">${name}</span><span class="tok-punct">${eq}</span><span class="tok-string">${value}</span>`
			);

			return `${open}<span class="tok-tag">${tagName}</span>${highlightedAttrs}${close}`;
		}
	);

	return { mode: 'pre', html: highlighted };
}

function renderConfig(source: string): RenderedTextPreview {
	const html = tokenizeSource(
		source,
		/(^\s*[#;].*$|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|^\s*[\w.-]+(?=\s*[:=])|\btrue\b|\bfalse\b|\bnull\b|-?\b\d+(?:\.\d+)?\b|[:=])/gm,
		(token) => {
			if (/^\s*[#;]/.test(token)) return 'tok-comment';
			if (token === ':' || token === '=') return 'tok-punct';
			if (token === 'true' || token === 'false') return 'tok-bool';
			if (token === 'null') return 'tok-null';
			if (/^-?\d/.test(token)) return 'tok-number';
			if (token.startsWith('"') || token.startsWith("'")) return 'tok-string';
			return 'tok-key';
		}
	);

	return { mode: 'pre', html };
}

function renderCode(source: string): RenderedTextPreview {
	const html = tokenizeSource(
		source,
		/(\/\/.*$|#.*$|\/\*[\s\S]*?\*\/|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`])*`|\b(function|return|const|let|var|if|else|for|while|class|import|export|from|async|await|try|catch|switch|case|break|continue|new|public|private|protected|package|func|struct|enum|interface|type|extends|implements|SELECT|FROM|WHERE|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP)\b|\btrue\b|\bfalse\b|\bnull\b|\bundefined\b|-?\b\d+(?:\.\d+)?\b)/gm,
		(token) => {
			if (token.startsWith('//') || token.startsWith('#') || token.startsWith('/*'))
				return 'tok-comment';
			if (token.startsWith('"') || token.startsWith("'") || token.startsWith('`'))
				return 'tok-string';
			if (token === 'true' || token === 'false') return 'tok-bool';
			if (token === 'null' || token === 'undefined') return 'tok-null';
			if (/^-?\d/.test(token)) return 'tok-number';
			return 'tok-keyword';
		}
	);

	return { mode: 'pre', html };
}

function parseDelimitedLine(line: string, delimiter: string): string[] {
	const cells: string[] = [];
	let current = '';
	let inQuotes = false;

	for (let i = 0; i < line.length; i += 1) {
		const char = line[i];
		const nextChar = line[i + 1];

		if (char === '"') {
			if (inQuotes && nextChar === '"') {
				current += '"';
				i += 1;
			} else {
				inQuotes = !inQuotes;
			}
			continue;
		}

		if (char === delimiter && !inQuotes) {
			cells.push(current);
			current = '';
			continue;
		}

		current += char;
	}

	cells.push(current);
	return cells;
}

function renderTable(source: string, delimiter: ',' | '\t'): RenderedTextPreview {
	const lines = source
		.split('\n')
		.filter((line) => line.length > 0)
		.slice(0, 50);
	if (lines.length === 0) {
		return renderPlainPre(source);
	}

	const rows = lines.map((line) => parseDelimitedLine(line, delimiter).slice(0, 20));
	const header = rows[0] || [];
	const body = rows.slice(1);

	const headHtml = header.map((cell) => `<th>${escapeHtml(cell)}</th>`).join('');
	const bodyHtml = body
		.map((row) => `<tr>${row.map((cell) => `<td>${escapeHtml(cell)}</td>`).join('')}</tr>`)
		.join('');

	return {
		mode: 'table',
		html: `<table class="preview-table"><thead><tr>${headHtml}</tr></thead><tbody>${bodyHtml}</tbody></table>`
	};
}

export function renderTextPreview(source: string, fileMetadata: FileMetadata): RenderedTextPreview {
	const kind = detectPreviewKind(fileMetadata);

	switch (kind) {
		case 'json':
			return renderJson(source);
		case 'csv':
			return renderTable(source, ',');
		case 'tsv':
			return renderTable(source, '\t');
		case 'markup':
			return renderMarkup(source);
		case 'yaml':
		case 'toml':
		case 'ini':
			return renderConfig(source);
		case 'code':
			return renderCode(source);
		default:
			return renderPlainPre(source);
	}
}
