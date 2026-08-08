import { describe, expect, it } from 'vitest';
import { buildPairingQrPayload, parsePairingQr } from '../pairingQr';

describe('buildPairingQrPayload', () => {
  it('emits canonical marker JSON with trimmed url', () => {
    const payload = buildPairingQrPayload('  https://aranea.example.com  ');
    expect(JSON.parse(payload)).toEqual({
      aranea: 'mobile-setup',
      v: 1,
      url: 'https://aranea.example.com',
    });
  });
});

describe('parsePairingQr', () => {
  it('round-trips a payload produced by buildPairingQrPayload', () => {
    const text = buildPairingQrPayload('https://aranea.example.com');
    expect(parsePairingQr(text)).toEqual({ url: 'https://aranea.example.com' });
  });

  it('accepts a plain plausible server URL (hand-rolled QR)', () => {
    expect(parsePairingQr('https://aranea.example.com')).toEqual({
      url: 'https://aranea.example.com',
    });
    expect(parsePairingQr('  http://192.168.1.10:8000  ')).toEqual({
      url: 'http://192.168.1.10:8000',
    });
  });

  it('rejects non-JSON garbage that is not a URL', () => {
    expect(parsePairingQr('')).toBeNull();
    expect(parsePairingQr('   ')).toBeNull();
    expect(parsePairingQr('hello world')).toBeNull();
    expect(parsePairingQr('WIFI:S:home;T:WPA;P:secret;;')).toBeNull();
  });

  it('rejects marker JSON with wrong marker / version / url types', () => {
    expect(parsePairingQr(JSON.stringify({ aranea: 'other', v: 1, url: 'https://a.com' }))).toBeNull();
    expect(parsePairingQr(JSON.stringify({ aranea: 'mobile-setup', v: 2, url: 'https://a.com' }))).toBeNull();
    expect(parsePairingQr(JSON.stringify({ aranea: 'mobile-setup', v: 1, url: 42 }))).toBeNull();
    expect(parsePairingQr(JSON.stringify({ aranea: 'mobile-setup', v: 1 }))).toBeNull();
    expect(parsePairingQr(JSON.stringify(['mobile-setup', 'https://a.com']))).toBeNull();
    expect(parsePairingQr('"https://a.com"')).toBeNull();
  });

  it('rejects marker JSON whose url fails server-url validation', () => {
    expect(parsePairingQr(JSON.stringify({ aranea: 'mobile-setup', v: 1, url: 'ftp://a.com' }))).toBeNull();
    expect(parsePairingQr(JSON.stringify({ aranea: 'mobile-setup', v: 1, url: 'https://a.com/api' }))).toBeNull();
    expect(parsePairingQr(JSON.stringify({ aranea: 'mobile-setup', v: 1, url: 'not-a-url' }))).toBeNull();
  });

  it('rejects plain text that is not a plausible server URL', () => {
    expect(parsePairingQr('ftp://example.com')).toBeNull();
    expect(parsePairingQr('https://example.com/some/path')).toBeNull();
    expect(parsePairingQr('https://example.com?x=1')).toBeNull();
  });
});
