import { registryCol, REGISTRY_COL_W } from '../../features/ui/registryTableColumns';

export function buildProductColumns(t: (key: string) => string) {
  return [
    registryCol('name', t('ecommerceLoop.traffic.product'), 'nameKey', 'left', REGISTRY_COL_W.nameWide),
    registryCol('visitors', t('ecommerceLoop.traffic.visitors'), 'visitors', 'right', REGISTRY_COL_W.metric),
    registryCol('cart', t('ecommerceLoop.traffic.cart'), 'addToCart', 'right', REGISTRY_COL_W.metric),
    registryCol('rate', t('ecommerceLoop.traffic.rate'), 'addToCart', 'right', REGISTRY_COL_W.content),
    registryCol('orders', t('ecommerceLoop.traffic.orders'), 'orders', 'right', REGISTRY_COL_W.metric),
  ];
}

export function buildWorkbenchProductColumns(t: (key: string) => string) {
  return buildProductColumns(t).filter((column) => column.name !== 'cart');
}
