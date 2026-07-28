export type AdminSession = {
  id: number;
  name: string;
  email: string;
  access: string;
  avatar: string;
  /** P2 (mobile): JWT returned by Login only; persisted via services/authToken for Bearer/WS auth. */
  token?: string;
};
