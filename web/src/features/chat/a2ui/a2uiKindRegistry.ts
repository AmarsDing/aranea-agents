/** Table-driven A2UI StandardCatalog kind → presentation route (SRP: routing only). */

export type A2UIKindRoute = 'primitive' | 'form' | 'layout' | 'container' | 'unknown';

const KIND_ROUTE: Record<string, A2UIKindRoute> = {
  Text: 'primitive',
  Divider: 'primitive',
  Image: 'primitive',
  Icon: 'primitive',
  Video: 'primitive',
  Button: 'form',
  TextField: 'form',
  CheckBox: 'form',
  List: 'layout',
  Row: 'layout',
  Column: 'layout',
  Card: 'container',
  Modal: 'container',
  Tabs: 'container',
};

export function resolveA2UIKindRoute(kind: string): A2UIKindRoute {
  const k = (kind || '').trim();
  return KIND_ROUTE[k] ?? 'unknown';
}
