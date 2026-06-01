import { createEcosystemService } from "../../services";
import type { Product } from "../../services/kratos/ecosystem/v1/index";

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

function mapProduct(raw: Product): EcosystemProduct {
  return {
    id: String(raw.id ?? ""),
    name: String(raw.name ?? ""),
    display_name: String(raw.displayName ?? ""),
    description: String(raw.description ?? ""),
    type: String(raw.type ?? ""),
    version: String(raw.version ?? ""),
    install_count: Number(raw.installCount ?? 0),
    installed: Boolean(raw.installed ?? false)
  };
}

export async function listEcosystemProducts(search = ""): Promise<EcosystemProduct[]> {
  const svc = createEcosystemService();
  const res = await svc.ListProducts({ search: search || undefined, limit: 100, type: undefined, offset: undefined });
  const items = res.items ?? [];
  return items.map(mapProduct);
}

export async function installEcosystemProduct(id: string): Promise<void> {
  const svc = createEcosystemService();
  await svc.InstallProduct({ id });
}

export async function publishEcosystemProduct(input: {
  name: string;
  display_name: string;
  description: string;
  type: string;
}): Promise<EcosystemProduct> {
  const svc = createEcosystemService();
  const res = await svc.PublishProduct({
    name: input.name,
    displayName: input.display_name,
    description: input.description,
    type: input.type,
    version: undefined,
    priceModel: undefined,
    priceCents: undefined,
    configJson: undefined,
  });
  return mapProduct(res);
}
