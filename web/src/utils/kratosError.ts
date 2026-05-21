import axios from "axios";

export type KratosApiError = {
  status?: number;
  reason?: string;
  message: string;
};

export type AgentCreateFieldErrors = {
  agent_key?: string;
  display_name?: string;
  provider?: string;
  model?: string;
  remote_url?: string;
  form?: string;
};

export function parseKratosApiError(err: unknown): KratosApiError {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data as Record<string, unknown> | undefined;
    const message =
      typeof data?.message === "string"
        ? data.message
        : err.message || "请求失败";
    return {
      status: err.response?.status,
      reason: typeof data?.reason === "string" ? data.reason : undefined,
      message
    };
  }
  if (err instanceof Error) {
    return { message: err.message };
  }
  return { message: "请求失败" };
}

export function mapAgentCreateFieldErrors(err: KratosApiError): AgentCreateFieldErrors {
  const msg = err.message;
  switch (err.reason) {
    case "AGENT_KEY_INVALID":
    case "AGENT_KEY_CONFLICT":
      return { agent_key: msg };
    case "AGENT":
      if (/agent_key/i.test(msg)) return { agent_key: msg };
      if (/display_name/i.test(msg)) return { display_name: msg };
      if (/remote_url|a2a_proxy/i.test(msg)) return { remote_url: msg };
      if (/provider|model/i.test(msg)) return { provider: msg, model: msg };
      return { form: msg };
    default:
      return { form: msg };
  }
}
