// web/src/features/session/__tests__/v2Api.spec.ts
import { describe, it, expect } from 'vitest';
import { decodeBytesJson } from '../v2Api';

describe('decodeBytesJson', () => {
  it('returns null for null/undefined/empty', () => {
    expect(decodeBytesJson(null)).toBeNull();
    expect(decodeBytesJson(undefined)).toBeNull();
    expect(decodeBytesJson('')).toBeNull();
  });

  it('returns object as-is when already decoded', () => {
    const obj = { foo: 'bar' };
    expect(decodeBytesJson(obj as never)).toBe(obj);
  });

  it('decodes literal JSON string', () => {
    expect(decodeBytesJson('{"a":1}')).toEqual({ a: 1 });
    expect(decodeBytesJson('[1,2,3]')).toEqual([1, 2, 3]);
  });

  it('decodes base64-encoded ASCII JSON', () => {
    // base64('{"path":"/tmp/file.txt"}') = eyJwYXRoIjoiL3RtcC9maWxlLnR4dCJ9
    expect(decodeBytesJson('eyJwYXRoIjoiL3RtcC9maWxlLnR4dCJ9')).toEqual({
      path: '/tmp/file.txt',
    });
  });

  it('decodes base64-encoded UTF-8 JSON with Chinese characters without mojibake', () => {
    // The original bug: atob() returns Latin-1 binary string, Chinese UTF-8
    // bytes were misinterpreted as Latin-1, causing mojibake like
    // "ç»å»ºä¸ä¸ªå¢é" instead of "组建三个团队".
    // base64('{"label":"组建三个团队"}')
    const b64 = btoa(unescape(encodeURIComponent('{"label":"组建三个团队"}')));
    const result = decodeBytesJson(b64);
    expect(result).toEqual({ label: '组建三个团队' });
  });

  it('decodes base64-encoded UTF-8 JSON with emoji', () => {
    // base64('{"msg":"hello 🚀 world"}')
    const b64 = btoa(unescape(encodeURIComponent('{"msg":"hello 🚀 world"}')));
    const result = decodeBytesJson(b64);
    expect(result).toEqual({ msg: 'hello 🚀 world' });
  });

  it('returns null for invalid base64 and invalid JSON', () => {
    expect(decodeBytesJson('!!!not-base64!!!')).toBeNull();
    expect(decodeBytesJson('{invalid json}')).toBeNull();
  });
});
