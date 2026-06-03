/** ReAct planner shared types (parse / tool-link / presentation; no parsing logic). */

export type ReactStepKind = 'planning' | 'reasoning' | 'action' | 'replanning';

export type ReactStep = {
  kind: ReactStepKind;
  title: string;
  body: string;
};

export type ReactParsedContent = {
  steps: ReactStep[];
  finalAnswer: string;
  /** Body when no structured tags matched */
  fallbackMarkdown: string;
};
