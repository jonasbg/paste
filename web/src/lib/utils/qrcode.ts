/**
 * Minimal, zero-dependency ISO/IEC 18004 QR Code generator (Byte mode, EC Level M, Versions 1-15).
 * Runs 100% locally in the browser so secret share URLs (#key= / #passphrase=) never leave the device.
 */

export interface QrMatrix {
	size: number;
	version: number;
	modules: boolean[][];
}

// EC Level M parameters for Versions 1..15 from ISO/IEC 18004 Table 9:
// [ecCodewordsPerBlock, group1Blocks, group1DataCodewords, group2Blocks, group2DataCodewords]
const EC_M_TABLE: ReadonlyArray<readonly [number, number, number, number, number]> = [
	[0, 0, 0, 0, 0], // placeholder for index 0
	[10, 1, 16, 0, 0], // v1  (21x21, 14 bytes)
	[16, 1, 28, 0, 0], // v2  (25x25, 26 bytes)
	[26, 1, 44, 0, 0], // v3  (29x29, 42 bytes)
	[18, 2, 32, 0, 0], // v4  (33x33, 62 bytes)
	[24, 2, 43, 0, 0], // v5  (37x37, 84 bytes)
	[16, 4, 27, 0, 0], // v6  (41x41, 106 bytes)
	[18, 4, 31, 0, 0], // v7  (45x45, 122 bytes)
	[22, 2, 38, 2, 39], // v8  (49x49, 152 bytes)
	[22, 3, 36, 2, 37], // v9  (53x53, 180 bytes)
	[26, 4, 43, 1, 44], // v10 (57x57, 213 bytes)
	[30, 1, 50, 4, 51], // v11 (61x61, 251 bytes)
	[22, 6, 36, 2, 37], // v12 (65x65, 287 bytes)
	[22, 8, 37, 1, 38], // v13 (69x69, 331 bytes)
	[24, 4, 40, 5, 41], // v14 (73x73, 362 bytes)
	[24, 5, 41, 5, 42] // v15 (77x77, 412 bytes)
];

// Alignment pattern center coordinates for Versions 1..15 (ISO/IEC 18004 Table E.1).
const ALIGNMENT_PATTERNS: ReadonlyArray<readonly number[]> = [
	[], // 0
	[], // 1
	[6, 18], // 2
	[6, 22], // 3
	[6, 26], // 4
	[6, 30], // 5
	[6, 34], // 6
	[6, 22, 38], // 7
	[6, 24, 42], // 8
	[6, 26, 46], // 9
	[6, 28, 50], // 10
	[6, 30, 54], // 11
	[6, 32, 58], // 12
	[6, 34, 62], // 13
	[6, 26, 46, 66], // 14
	[6, 26, 48, 70] // 15
];

// GF(256) exp/log tables with primitive polynomial 0x11D.
const GF_EXP = new Uint8Array(512);
const GF_LOG = new Uint8Array(256);
(() => {
	let x = 1;
	for (let i = 0; i < 255; i++) {
		GF_EXP[i] = x;
		GF_LOG[x] = i;
		x <<= 1;
		if (x & 0x100) x ^= 0x11d;
	}
	for (let i = 255; i < 512; i++) {
		GF_EXP[i] = GF_EXP[i - 255];
	}
})();

function gfMul(a: number, b: number): number {
	if (a === 0 || b === 0) return 0;
	return GF_EXP[GF_LOG[a] + GF_LOG[b]];
}

function rsGeneratorPoly(degree: number): Uint8Array {
	let poly = new Uint8Array([1]);
	for (let i = 0; i < degree; i++) {
		const next = new Uint8Array(poly.length + 1);
		const factor = GF_EXP[i];
		for (let j = 0; j < poly.length; j++) {
			next[j] ^= poly[j];
			next[j + 1] ^= gfMul(poly[j], factor);
		}
		poly = next;
	}
	return poly;
}

function rsComputeRemainder(data: Uint8Array, ecLen: number): Uint8Array {
	const gen = rsGeneratorPoly(ecLen);
	const rem = new Uint8Array(ecLen);
	for (let i = 0; i < data.length; i++) {
		const factor = data[i] ^ rem[0];
		for (let j = 0; j < ecLen - 1; j++) {
			rem[j] = rem[j + 1] ^ gfMul(gen[j + 1], factor);
		}
		rem[ecLen - 1] = gfMul(gen[ecLen], factor);
	}
	return rem;
}

function selectVersion(byteLen: number): number {
	for (let v = 1; v < EC_M_TABLE.length; v++) {
		const [, g1, g1Data, g2, g2Data] = EC_M_TABLE[v];
		const totalDataCodewords = g1 * g1Data + g2 * g2Data;
		const countBits = v <= 9 ? 8 : 16;
		const neededBits = 4 + countBits + byteLen * 8;
		if (neededBits <= totalDataCodewords * 8) {
			return v;
		}
	}
	throw new Error('QR payload too long');
}

function encodeDataCodewords(bytes: Uint8Array, version: number): Uint8Array {
	const [, g1, g1Data, g2, g2Data] = EC_M_TABLE[version];
	const totalDataCodewords = g1 * g1Data + g2 * g2Data;
	const capacityBits = totalDataCodewords * 8;

	const bits: number[] = [];
	const pushBits = (val: number, len: number) => {
		for (let i = len - 1; i >= 0; i--) {
			bits.push((val >>> i) & 1);
		}
	};

	// Byte mode indicator = 0100
	pushBits(0b0100, 4);
	// Character count indicator
	pushBits(bytes.length, version <= 9 ? 8 : 16);
	for (let i = 0; i < bytes.length; i++) {
		pushBits(bytes[i], 8);
	}

	// Terminator (up to 4 zero bits)
	const termLen = Math.min(4, capacityBits - bits.length);
	for (let i = 0; i < termLen; i++) bits.push(0);

	// Pad to byte boundary
	while (bits.length % 8 !== 0) bits.push(0);

	// Pack into bytes and append alternating 0xEC / 0x11 pad codewords
	const out = new Uint8Array(totalDataCodewords);
	const byteCount = bits.length / 8;
	for (let i = 0; i < byteCount; i++) {
		let b = 0;
		for (let j = 0; j < 8; j++) {
			b = (b << 1) | bits[i * 8 + j];
		}
		out[i] = b;
	}
	for (let i = byteCount, padIdx = 0; i < totalDataCodewords; i++, padIdx++) {
		out[i] = padIdx % 2 === 0 ? 0xec : 0x11;
	}
	return out;
}

function interleaveBlocks(data: Uint8Array, version: number): Uint8Array {
	const [ecLen, g1, g1Data, g2, g2Data] = EC_M_TABLE[version];
	const dataBlocks: Uint8Array[] = [];
	const ecBlocks: Uint8Array[] = [];

	let offset = 0;
	for (let i = 0; i < g1; i++) {
		const block = data.slice(offset, offset + g1Data);
		offset += g1Data;
		dataBlocks.push(block);
		ecBlocks.push(rsComputeRemainder(block, ecLen));
	}
	for (let i = 0; i < g2; i++) {
		const block = data.slice(offset, offset + g2Data);
		offset += g2Data;
		dataBlocks.push(block);
		ecBlocks.push(rsComputeRemainder(block, ecLen));
	}

	const totalLen = data.length + (g1 + g2) * ecLen;
	const result = new Uint8Array(totalLen);
	let pos = 0;

	const maxDataLen = Math.max(g1Data, g2Data);
	for (let i = 0; i < maxDataLen; i++) {
		for (const block of dataBlocks) {
			if (i < block.length) result[pos++] = block[i];
		}
	}
	for (let i = 0; i < ecLen; i++) {
		for (const block of ecBlocks) {
			result[pos++] = block[i];
		}
	}
	return result;
}

function computeFormatBits(mask: number): number {
	// EC Level M = 00 (2 bits) || mask (3 bits)
	const data = (0b00 << 3) | (mask & 0x7);
	let rem = data << 10;
	for (let i = 14; i >= 10; i--) {
		if ((rem >>> i) & 1) {
			rem ^= 0x537 << (i - 10);
		}
	}
	return ((data << 10) | rem) ^ 0x5412;
}

function computeVersionBits(version: number): number {
	let rem = version << 12;
	for (let i = 17; i >= 12; i--) {
		if ((rem >>> i) & 1) {
			rem ^= 0x1f25 << (i - 12);
		}
	}
	return (version << 12) | rem;
}

function drawFunctionPatterns(version: number): {
	modules: boolean[][];
	isFunction: boolean[][];
	size: number;
} {
	const size = version * 4 + 17;
	const modules: boolean[][] = Array.from({ length: size }, () => Array(size).fill(false));
	const isFunction: boolean[][] = Array.from({ length: size }, () => Array(size).fill(false));

	const setFn = (r: number, c: number, dark: boolean) => {
		if (r < 0 || r >= size || c < 0 || c >= size) return;
		modules[r][c] = dark;
		isFunction[r][c] = true;
	};

	// Timing patterns
	for (let i = 0; i < size; i++) {
		setFn(6, i, i % 2 === 0);
		setFn(i, 6, i % 2 === 0);
	}

	// Finder patterns + separators
	const drawFinder = (top: number, left: number) => {
		for (let dr = -1; dr <= 7; dr++) {
			for (let dc = -1; dc <= 7; dc++) {
				const r = top + dr;
				const c = left + dc;
				if (r < 0 || r >= size || c < 0 || c >= size) continue;
				const inOuter = dr >= 0 && dr <= 6 && dc >= 0 && dc <= 6;
				const onBorder = dr === 0 || dr === 6 || dc === 0 || dc === 6;
				const inCenter = dr >= 2 && dr <= 4 && dc >= 2 && dc <= 4;
				setFn(r, c, inOuter && (onBorder || inCenter));
			}
		}
	};
	drawFinder(0, 0);
	drawFinder(0, size - 7);
	drawFinder(size - 7, 0);

	// Alignment patterns
	const coords = ALIGNMENT_PATTERNS[version];
	for (let i = 0; i < coords.length; i++) {
		for (let j = 0; j < coords.length; j++) {
			const isCorner =
				(i === 0 && j === 0) ||
				(i === 0 && j === coords.length - 1) ||
				(i === coords.length - 1 && j === 0);
			if (isCorner) continue;
			const cr = coords[i];
			const cc = coords[j];
			for (let dr = -2; dr <= 2; dr++) {
				for (let dc = -2; dc <= 2; dc++) {
					const dist = Math.max(Math.abs(dr), Math.abs(dc));
					setFn(cr + dr, cc + dc, dist === 0 || dist === 2);
				}
			}
		}
	}

	// Reserve format info areas + dark module
	for (let i = 0; i <= 8; i++) {
		if (i !== 6) {
			setFn(8, i, false);
			setFn(i, 8, false);
		}
	}
	for (let i = size - 8; i < size; i++) {
		setFn(8, i, false);
		setFn(i, 8, false);
	}
	setFn(size - 8, 8, true); // always dark module

	// Version info (Versions 7+)
	if (version >= 7) {
		const vBits = computeVersionBits(version);
		for (let i = 0; i < 18; i++) {
			const bit = ((vBits >>> i) & 1) !== 0;
			const a = size - 11 + (i % 3);
			const b = Math.floor(i / 3);
			setFn(a, b, bit);
			setFn(b, a, bit);
		}
	}

	return { modules, isFunction, size };
}

function drawFormatBits(modules: boolean[][], size: number, mask: number) {
	const bits = computeFormatBits(mask);

	// First copy (around top-left finder)
	for (let i = 0; i <= 5; i++) modules[8][i] = ((bits >>> (14 - i)) & 1) !== 0;
	modules[8][7] = ((bits >>> 8) & 1) !== 0;
	modules[8][8] = ((bits >>> 7) & 1) !== 0;
	modules[7][8] = ((bits >>> 6) & 1) !== 0;
	for (let i = 9; i < 15; i++) modules[14 - i][8] = ((bits >>> (14 - i)) & 1) !== 0;

	// Second copy (bottom-left and top-right)
	for (let i = 0; i < 8; i++) modules[8][size - 1 - i] = ((bits >>> i) & 1) !== 0;
	for (let i = 8; i < 15; i++) modules[size - 15 + i][8] = ((bits >>> i) & 1) !== 0;
	modules[size - 8][8] = true;
}

function placeCodewords(
	modules: boolean[][],
	isFunction: boolean[][],
	size: number,
	codewords: Uint8Array
) {
	let bitIdx = 0;
	const totalBits = codewords.length * 8;
	let upward = true;

	for (let right = size - 1; right >= 1; right -= 2) {
		if (right === 6) right = 5; // skip vertical timing column
		for (let vert = 0; vert < size; vert++) {
			const r = upward ? size - 1 - vert : vert;
			for (let j = 0; j < 2; j++) {
				const c = right - j;
				if (!isFunction[r][c]) {
					let dark = false;
					if (bitIdx < totalBits) {
						dark = ((codewords[bitIdx >>> 3] >>> (7 - (bitIdx & 7))) & 1) !== 0;
						bitIdx++;
					}
					modules[r][c] = dark;
				}
			}
		}
		upward = !upward;
	}
}

function maskBit(mask: number, r: number, c: number): boolean {
	switch (mask) {
		case 0:
			return (r + c) % 2 === 0;
		case 1:
			return r % 2 === 0;
		case 2:
			return c % 3 === 0;
		case 3:
			return (r + c) % 3 === 0;
		case 4:
			return (Math.floor(r / 2) + Math.floor(c / 3)) % 2 === 0;
		case 5:
			return ((r * c) % 2) + ((r * c) % 3) === 0;
		case 6:
			return (((r * c) % 2) + ((r * c) % 3)) % 2 === 0;
		case 7:
			return (((r + c) % 2) + ((r * c) % 3)) % 2 === 0;
		default:
			return false;
	}
}

function applyMask(
	base: boolean[][],
	isFunction: boolean[][],
	size: number,
	mask: number
): boolean[][] {
	const out: boolean[][] = Array.from({ length: size }, (_, r) => {
		const row = base[r].slice();
		for (let c = 0; c < size; c++) {
			if (!isFunction[r][c] && maskBit(mask, r, c)) {
				row[c] = !row[c];
			}
		}
		return row;
	});
	drawFormatBits(out, size, mask);
	return out;
}

function penaltyScore(modules: boolean[][], size: number): number {
	let score = 0;

	// Rule 1: 5+ same-colored modules in a row/column
	for (let r = 0; r < size; r++) {
		let runColor = modules[r][0];
		let runLen = 1;
		for (let c = 1; c < size; c++) {
			if (modules[r][c] === runColor) {
				runLen++;
			} else {
				if (runLen >= 5) score += 3 + (runLen - 5);
				runColor = modules[r][c];
				runLen = 1;
			}
		}
		if (runLen >= 5) score += 3 + (runLen - 5);
	}
	for (let c = 0; c < size; c++) {
		let runColor = modules[0][c];
		let runLen = 1;
		for (let r = 1; r < size; r++) {
			if (modules[r][c] === runColor) {
				runLen++;
			} else {
				if (runLen >= 5) score += 3 + (runLen - 5);
				runColor = modules[r][c];
				runLen = 1;
			}
		}
		if (runLen >= 5) score += 3 + (runLen - 5);
	}

	// Rule 2: 2x2 blocks of same color
	for (let r = 0; r < size - 1; r++) {
		for (let c = 0; c < size - 1; c++) {
			const v = modules[r][c];
			if (v === modules[r][c + 1] && v === modules[r + 1][c] && v === modules[r + 1][c + 1]) {
				score += 3;
			}
		}
	}

	// Rule 3: finder-like 1:1:3:1:1 patterns
	const patA = [true, false, true, true, true, false, true, false, false, false, false];
	const patB = [false, false, false, false, true, false, true, true, true, false, true];
	for (let r = 0; r < size; r++) {
		for (let c = 0; c <= size - 11; c++) {
			let mA = true;
			let mB = true;
			for (let k = 0; k < 11; k++) {
				if (modules[r][c + k] !== patA[k]) mA = false;
				if (modules[r][c + k] !== patB[k]) mB = false;
			}
			if (mA || mB) score += 40;
		}
	}
	for (let c = 0; c < size; c++) {
		for (let r = 0; r <= size - 11; r++) {
			let mA = true;
			let mB = true;
			for (let k = 0; k < 11; k++) {
				if (modules[r + k][c] !== patA[k]) mA = false;
				if (modules[r + k][c] !== patB[k]) mB = false;
			}
			if (mA || mB) score += 40;
		}
	}

	// Rule 4: dark/light balance
	let darkCount = 0;
	for (let r = 0; r < size; r++) {
		for (let c = 0; c < size; c++) {
			if (modules[r][c]) darkCount++;
		}
	}
	const pct = (darkCount * 100) / (size * size);
	const k = Math.floor(Math.abs(pct - 50) / 5);
	score += k * 10;

	return score;
}

/** Generate a QR code boolean matrix for the given UTF-8 string. */
export function generateQrMatrix(text: string): QrMatrix {
	const bytes = new TextEncoder().encode(text);
	const version = selectVersion(bytes.length);
	const dataCodewords = encodeDataCodewords(bytes, version);
	const finalCodewords = interleaveBlocks(dataCodewords, version);

	const { modules: base, isFunction, size } = drawFunctionPatterns(version);
	placeCodewords(base, isFunction, size, finalCodewords);

	let bestMask = 0;
	let bestPenalty = Infinity;
	let bestModules = base;

	for (let mask = 0; mask < 8; mask++) {
		const candidate = applyMask(base, isFunction, size, mask);
		const p = penaltyScore(candidate, size);
		if (p < bestPenalty) {
			bestPenalty = p;
			bestMask = mask;
			bestModules = candidate;
		}
	}
	void bestMask;

	return { size, version, modules: bestModules };
}
