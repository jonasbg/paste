import { generateHmacToken } from '$lib/utils/hmacUtils';
import { getWasmInstance } from '$lib/utils/wasm-loader';
import { normalizeMimeType } from '$lib/utils/mimeType';
import { FileProcessor } from './fileProcessor';
import type { ProgressCallback } from './fileProcessor';
import { configStore } from '$lib/stores/config';
import { get } from 'svelte/store';
import { tr } from '$lib/i18n';

function requireWasmMethod<T>(method: T | undefined, name: string): NonNullable<T> {
    if (!method) {
        throw new Error(`${name} is unavailable`);
    }

    return method as NonNullable<T>;
}

export function generateKey(): string | null {
    const wasmInstance = getWasmInstance();
    if (!wasmInstance) return null;
    const generateKeyMethod = requireWasmMethod(wasmInstance.generateKey, 'generateKey');

    const config = get(configStore);
    if (!config.data) {
        console.warn('Config not loaded, using default key size of 128');
        return generateKeyMethod(128);
    }

    const keySize = parseInt(config.data.key_size);
    if (isNaN(keySize)) {
        console.warn('Invalid key size in config, using default of 128');
        return generateKeyMethod(128);
    }

    return generateKeyMethod(keySize);
}

export async function uploadEncryptedFile(
    file: File,
    key: string,
    onProgress: ProgressCallback,
    customFileId?: string
): Promise<{ fileId: string; token: string }> {
    const fileProcessor = new FileProcessor();
    const config = get(configStore);
    if (config.loading) {
        await new Promise<void>((resolve) => {
            const unsubscribe = configStore.subscribe((state) => {
                if (!state.loading) {
                    unsubscribe();
                    resolve();
                }
            });
        });
    }

    if (config.error) throw new Error(`Failed to load configuration: ${config.error}`);
    if (file.size > fileProcessor.getMaxFileSize()) {
        throw new Error(
            `File size exceeds maximum allowed size of ${FileProcessor.formatFileSize(fileProcessor.getMaxFileSize())}`
        );
    }

    const wasmInstance = getWasmInstance();
    if (!wasmInstance) throw new Error('WASM not initialized');
    const encrypt = requireWasmMethod(wasmInstance.encrypt, 'encrypt');
    const createEncryptionStream = requireWasmMethod(
        wasmInstance.createEncryptionStream,
        'createEncryptionStream'
    );
    const encryptChunk = requireWasmMethod(wasmInstance.encryptChunk, 'encryptChunk');
    const disposeCipher = wasmInstance.disposeCipher;

    return new Promise((resolve, reject) => {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/upload`);
        const chunkSize = config.chunkSize * 1024 * 1024;
        let fileOffset = 0;
        let currentFileId: string | null = null;
        let cachedToken: string | null = null;
        let cipherId: number | null = null;
        let settled = false; // guard: resolve/reject only once

        // Prefetch state: the next chunk's file read + WASM encryption is kicked
        // off immediately after the current chunk is sent, so it overlaps with the
        // network round-trip for the current chunk's ACK.
        type PrefetchResult = { encrypted: Uint8Array; plaintextSize: number };
        let prefetchForOffset = -1;
        let prefetchPromise: Promise<PrefetchResult | null> | null = null;

        // Smooth intra-chunk progress interpolation
        let lastChunkDuration = 0; // ms it took to upload+ack the previous chunk
        let chunkSendTime = 0;     // when we sent the current chunk
        let chunkStartBytes = 0;   // fileOffset value at chunk send time (plaintext)
        let chunkByteSize = 0;     // plaintext bytes in the current chunk
        let progressTimer: ReturnType<typeof setInterval> | null = null;

        ws.binaryType = 'arraybuffer';

        const stopProgressTimer = () => {
            if (progressTimer !== null) {
                clearInterval(progressTimer);
                progressTimer = null;
            }
        };

        const cleanup = () => {
            stopProgressTimer();
            if (cipherId !== null && disposeCipher) {
                disposeCipher(cipherId);
                cipherId = null;
            }
        };

        const settle = (fn: () => void) => {
            if (settled) return;
            settled = true;
            cleanup();
            fn();
        };

        // ── Serial message queue ─────────────────────────────────────────────
        // ws.onmessage is NOT declared async so we never have two concurrent
        // handlers running. All async work goes through the queue below, which
        // processes messages one at a time in arrival order.
        const msgQueue: Array<Record<string, unknown>> = [];
        let queueRunning = false;

        async function drainQueue() {
            if (queueRunning) return;
            queueRunning = true;
            while (msgQueue.length > 0) {
                const msg = msgQueue.shift()!;
                await handleMessage(msg);
            }
            queueRunning = false;
        }

        ws.onmessage = (event: MessageEvent) => {
            if (typeof event.data !== 'string') return;
            try {
                msgQueue.push(JSON.parse(event.data) as Record<string, unknown>);
            } catch {
                settle(() => reject(new Error('Invalid server message')));
                ws.close();
                return;
            }
            void drainQueue();
        };

        ws.onopen = () => {
            const initMsg: Record<string, unknown> = { type: 'init', size: file.size };
            if (customFileId) initMsg.fileId = customFileId;
            ws.send(JSON.stringify(initMsg));
        };

        ws.onerror = () => {
            settle(() => reject(new Error(tr('service.uploadNetworkError'))));
        };

        ws.onclose = (event: CloseEvent) => {
            // Only surface an error if we haven't already resolved/rejected.
            // A non-clean close (wasClean=false) means the connection was terminated
            // without a proper WebSocket close handshake — typically a proxy timeout
            // or network drop.
            settle(() => {
                if (!event.wasClean) {
                    reject(new Error(tr('service.connectionAborted')));
                } else {
                    // Clean close without a prior resolve is also an error (e.g. server
                    // closed the connection after sending an error frame).
                    reject(new Error(tr('service.connectionClosed')));
                }
            });
        };

        // ── Server message dispatcher ────────────────────────────────────────
        async function handleMessage(response: Record<string, unknown>) {
            // All server messages now carry a "type" field. Fall back to
            // property-based detection for any legacy path.
            const msgType = response.type as string | undefined;

            if (msgType === 'error' || response.error) {
                settle(() =>
                    reject(
                        new Error(
                            typeof response.error === 'string'
                                ? response.error
                                : tr('service.uploadUnknownError')
                        )
                    )
                );
                ws.close();
                return;
            }

            // Step 1 → server assigned an ID, send HMAC token
            if (msgType === 'id' && !currentFileId) {
                currentFileId = response.id as string;
                cachedToken = await generateHmacToken(currentFileId, key);
                ws.send(JSON.stringify({ type: 'token', token: cachedToken }));
                return;
            }

            // Step 2 → token accepted, send encrypted metadata header
            if (msgType === 'token_accepted') {
                const metadata = {
                    filename: file.name,
                    contentType: normalizeMimeType(file),
                    size: file.size
                };
                const metadataBytes = new TextEncoder().encode(JSON.stringify(metadata));
                const encryptedMetadata = encrypt(key, metadataBytes);
                const header = new Uint8Array(16 + encryptedMetadata.length - 12);
                header.set(encryptedMetadata.slice(0, 12), 0);
                new DataView(header.buffer).setUint32(12, encryptedMetadata.length - 12, true);
                header.set(encryptedMetadata.slice(12), 16);
                ws.send(header);
                return;
            }

            // Step 3 → server ready, initialise encryption stream and send first chunk
            if (msgType === 'ready') {
                const streamResult = createEncryptionStream(key);
                if (!streamResult || typeof streamResult.id !== 'number' || !streamResult.iv) {
                    const errorMsg =
                        streamResult instanceof Error ? streamResult.message : 'Invalid result';
                    settle(() =>
                        reject(
                            new Error('Failed to initialize encryption stream: ' + errorMsg)
                        )
                    );
                    ws.close();
                    return;
                }
                cipherId = streamResult.id;
                ws.send(streamResult.iv);
                await sendNextChunk();
                return;
            }

            // Step 4 → chunk acknowledged, send next chunk
            if (msgType === 'ack') {
                stopProgressTimer();
                if (chunkSendTime > 0) {
                    lastChunkDuration = Date.now() - chunkSendTime;
                }
                // Progress is based on plaintext fileOffset, not the encrypted ack byte
                // count (which is plaintext + 16 bytes per chunk and would drift > 100%).
                const progress = Math.min(Math.round((fileOffset / file.size) * 100), 99);
                await onProgress(progress, tr('service.uploading'));
                await sendNextChunk();
                return;
            }

            // Step 5 → upload complete
            if (msgType === 'complete' && currentFileId) {
                // Drive progress to 100 before resolving so the ProgressBar reactive
                // loop sees progress===100 at the same time isComplete becomes true.
                // Without this, isComplete sets displayProgress=100 while progress=99,
                // which triggers the animation loop to run backwards back to 99.
                await onProgress(100, tr('service.done'));
                settle(() => resolve({ fileId: currentFileId!, token: cachedToken! }));
                ws.close();
                return;
            }
        }

        // Start reading + encrypting the chunk at `forOffset` in the background.
        // Called immediately after ws.send() so the work overlaps with the ACK RTT.
        // Returns null on failure; sendNextChunk falls back to a synchronous path.
        async function prefetchNextChunk(forOffset: number): Promise<PrefetchResult | null> {
            if (cipherId === null || forOffset >= file.size) return null;
            try {
                const slice = await file.slice(forOffset, forOffset + chunkSize).arrayBuffer();
                // Re-check after the async file read — cleanup may have run.
                if (cipherId === null) return null;
                const isLast = forOffset + chunkSize >= file.size;
                const encrypted = encryptChunk(cipherId, new Uint8Array(slice), isLast);
                if (!encrypted || encrypted instanceof Error) return null;
                return { encrypted: encrypted as Uint8Array, plaintextSize: slice.byteLength };
            } catch {
                return null;
            }
        }

        async function sendNextChunk() {
            if (cipherId === null) {
                settle(() => reject(new Error('Cipher not initialized')));
                ws.close();
                return;
            }

            if (fileOffset < file.size) {
                let encryptedChunk: Uint8Array;
                let plaintextSize: number;

                if (prefetchPromise !== null && prefetchForOffset === fileOffset) {
                    // A prefetch was started for exactly this offset — await it.
                    // If it was already resolved this is effectively free; if it's
                    // still in flight we overlap the wait with nothing useful anyway.
                    const result = await prefetchPromise;
                    prefetchPromise = null;
                    if (result !== null) {
                        encryptedChunk = result.encrypted;
                        plaintextSize = result.plaintextSize;
                    } else {
                        // Prefetch failed — read and encrypt now.
                        const slice = await file.slice(fileOffset, fileOffset + chunkSize).arrayBuffer();
                        const isLast = fileOffset + chunkSize >= file.size;
                        encryptedChunk = encryptChunk(cipherId, new Uint8Array(slice), isLast) as Uint8Array;
                        plaintextSize = slice.byteLength;
                    }
                } else {
                    // No prefetch in flight for this offset (first chunk, or prefetch
                    // was not started because the previous chunk was the last).
                    const slice = await file.slice(fileOffset, fileOffset + chunkSize).arrayBuffer();
                    const isLast = fileOffset + chunkSize >= file.size;
                    encryptedChunk = encryptChunk(cipherId, new Uint8Array(slice), isLast) as Uint8Array;
                    plaintextSize = slice.byteLength;
                }

                chunkSendTime = Date.now();
                chunkStartBytes = fileOffset;       // plaintext bytes before this chunk
                chunkByteSize = plaintextSize;      // plaintext size of this chunk
                fileOffset += plaintextSize;

                const isLastChunk = fileOffset >= file.size;
                ws.send(encryptedChunk);

                // Immediately kick off the next chunk's file read + encryption so it
                // is ready (or nearly ready) when the server ACK arrives.
                if (!isLastChunk) {
                    prefetchForOffset = fileOffset;
                    prefetchPromise = prefetchNextChunk(fileOffset);
                }

                // Interpolate progress within this chunk every 100ms using the previous
                // chunk's round-trip duration as a speed estimate.
                if (!isLastChunk && lastChunkDuration > 0) {
                    progressTimer = setInterval(async () => {
                        const elapsed = Date.now() - chunkSendTime;
                        const fraction = Math.min(elapsed / lastChunkDuration, 0.95);
                        const estimatedBytes = chunkStartBytes + chunkByteSize * fraction;
                        const progress = Math.min(
                            Math.round((estimatedBytes / file.size) * 100),
                            99
                        );
                        await onProgress(progress, tr('service.uploading'));
                    }, 100);
                }
            } else {
                // Signal end of file
                ws.send(new Uint8Array([0]));
            }
        }
    });
}
