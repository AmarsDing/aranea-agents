import { beforeEach, describe, expect, it } from 'vitest';
import { bearerAuthHeader, clearAuthToken, getAuthToken, setAuthToken } from '../authToken';

describe('authToken storage', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('returns empty string when no token stored', () => {
    expect(getAuthToken()).toBe('');
    expect(bearerAuthHeader()).toEqual({});
  });

  it('stores and reads back the token, trimming whitespace', () => {
    setAuthToken('  jwt.abc.def  ');
    expect(getAuthToken()).toBe('jwt.abc.def');
    expect(bearerAuthHeader()).toEqual({ Authorization: 'Bearer jwt.abc.def' });
  });

  it('ignores empty token on set', () => {
    setAuthToken('first');
    setAuthToken('   ');
    expect(getAuthToken()).toBe('first');
  });

  it('clears the token', () => {
    setAuthToken('jwt.abc.def');
    clearAuthToken();
    expect(getAuthToken()).toBe('');
    expect(bearerAuthHeader()).toEqual({});
  });
});
