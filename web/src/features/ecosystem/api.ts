import { requestHandler } from "../../services/axiosHandler";

export type EcosystemProduct = {
  id: string;
  name: string;
  display_name: string;
  description: string;
  type: string;
  version: string;
  install_count: number;
  installed: boolean;
};

function mapProduct(raw: unknown): EcosystemProduct {
  const r = raw as Record<string, unknown>;
  return {
    id: String(r.id ?? ""),
    name: String(r.name ?? ""),
    display_name: String(r.displayName ?? r.display_name ?? ""),
    description: String(r.description ?? ""),
    type: String(r.type ?? ""),
    version: String(r.version ?? ""),
    install_count: Number(r.installCount ?? r.install_count ?? 0),
    installed: Boolean(r.installed ?? false)
  };
}

export async function listEcosystemProducts(search = ""): Promise<EcosystemProduct[]> {
  const q = new URLSearchParams({ limit: "100" });
  if (search) q.set("search", search);
  const res = await requestHandler({
    path: `v1/ecosystem/products?${q.toString()}`,
    method: "GET",
    body: null
  });
  const items = (res as { items?: unknown[] }).items ?? [];
  return items.map(mapProduct);
}

export async function installEcosystemProduct(id: string): Promise<void> {
  await requestHandler({
    path: `v1/ecosystem/products/${encodeURIComponent(id)}/install`,
    method: "POST",
    body: "{}"
  });
}

export async function publishEcosystemProduct(input: {
  name: string;
  display_name: string;
  description: string;
  type: string;
}): Promise<EcosystemProduct> {
  const res = await requestHandler({
    path: "v1/ecosystem/products",
    method: "POST",
    body: JSON.stringify({
      name: input.name,
      display_name: input.display_name,
      description: input.description,
      type: input.type
    })
  });
  return mapProduct(res);
}
