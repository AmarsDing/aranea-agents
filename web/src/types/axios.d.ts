import 'axios';

declare module 'axios' {
  export interface AxiosRequestConfig {
    /** When true, 4xx responses do not trigger the global Quasar notify (caller shows inline errors). */
    skipErrorNotify?: boolean;
  }
}
