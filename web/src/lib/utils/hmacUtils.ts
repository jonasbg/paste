import { encode as base64Encode } from 'base64-arraybuffer';

export function generateHmacToken(fileId: string, key: string): string {
  // Generate HMAC using SubtleCrypto
  const encoder = new TextEncoder();
  const keyData = encoder.encode(key);
  const fileIdData = encoder.encode(fileId);

  return crypto.subtle.importKey(
    'raw',
    keyData,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign']
  ).then(cryptoKey =>
    crypto.subtle.sign(
      'HMAC',
      cryptoKey,
      fileIdData
    )
  ).then(signature => {
    // Convert to base64 and make filename safe
    return base64Encode(signature)
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=/g, '');
  });
}