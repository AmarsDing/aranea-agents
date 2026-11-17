export type UiLayout = 'fluid' | 'measure' | 'canvas' | 'immersive';

export function resolveUiLayout(value: unknown): UiLayout {
  return value === 'measure' || value === 'canvas' || value === 'immersive' ? value : 'fluid';
}
