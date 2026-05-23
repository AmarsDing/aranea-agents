import { parseA2UIJsonl, shouldUseA2UIView } from "./a2uiParse";
import {
  formatUserActionUserMarkdown,
  parseUserActionFromContent,
} from "./a2uiUserActionDisplay";
import { toolEventFromMessage } from "./envelopeToolCall";
import { isActivityMessage } from "./mergeSessionMessages";
import { isToolLinkedInReactIndex } from "./reactToolLinkIndex";
import {
  parseReactPlannerContent,
  reactDisplayMarkdown,
  shouldUseReactPlannerView,
} from "./reactPlannerParse";
import { reasoningMarkdown } from "./streamContentPatch";
import type { Message, ReactStepWithTools, ReactToolLinkIndex } from "./types";

export type AssistantPresentation = {
  reasoning: string;
  reactSteps: ReturnType<typeof parseReactPlannerContent>;
  a2uiLines: ReturnType<typeof parseA2UIJsonl> | null;
  bodyMarkdown: string;
  mode: "default" | "react" | "a2ui" | "userAction";
};

export type MessagePresentationBundle = {
  presentation: AssistantPresentation;
  reactStepsWithTools: ReactStepWithTools[];
  /** Hide standalone tool_call row when shown under ReAct ACTION. */
  suppressToolRow: boolean;
  structuredToolEvent: ReturnType<typeof toolEventFromMessage>;
};

export function resolveAssistantPresentation(
  plannerKind: string,
  message: Message
): AssistantPresentation {
  const raw = message.content_markdown ?? "";
  const reasoningRaw = reasoningMarkdown(message).trim();

  if (shouldUseA2UIView(plannerKind, raw)) {
    return {
      reasoning: reasoningRaw,
      reactSteps: null,
      a2uiLines: parseA2UIJsonl(raw),
      bodyMarkdown: "",
      mode: "a2ui",
    };
  }

  if (shouldUseReactPlannerView(plannerKind, raw)) {
    const reactSteps = parseReactPlannerContent(raw);
    return {
      reasoning: "",
      reactSteps,
      a2uiLines: null,
      bodyMarkdown: reactDisplayMarkdown(reactSteps, raw),
      mode: "react",
    };
  }

  return {
    reasoning: reasoningRaw,
    reactSteps: null,
    a2uiLines: null,
    bodyMarkdown: raw,
    mode: "default",
  };
}

function resolveUserPresentation(message: Message): AssistantPresentation {
  const raw = message.content_markdown ?? "";
  const userAction = parseUserActionFromContent(raw);
  if (userAction) {
    return {
      reasoning: "",
      reactSteps: null,
      a2uiLines: null,
      bodyMarkdown: formatUserActionUserMarkdown(userAction),
      mode: "userAction",
    };
  }
  return {
    reasoning: "",
    reactSteps: null,
    a2uiLines: null,
    bodyMarkdown: raw,
    mode: "default",
  };
}

/** Index-only: linked tools come from buildReactToolLinkIndex, never per-row enrich. */
function reactStepsForIndex(
  cached: ReactStepWithTools[] | undefined,
  presentation: AssistantPresentation
): ReactStepWithTools[] {
  if (cached !== undefined) return cached;
  const rawSteps = presentation.reactSteps?.steps;
  if (!rawSteps?.length) return [];
  return rawSteps.map((step) => ({ ...step, linkedTools: [] }));
}

/** Single entry for ChatMessageRow: presentation, ReAct tools, tool-row dedupe. */
export function buildMessagePresentation(
  plannerKind: string,
  message: Message,
  index: number,
  reactLinkIndex: ReactToolLinkIndex
): MessagePresentationBundle {
  if (message.role === "user") {
    return {
      presentation: resolveUserPresentation(message),
      reactStepsWithTools: [],
      suppressToolRow: false,
      structuredToolEvent: null,
    };
  }

  const toolEv = toolEventFromMessage(message);
  const suppressToolRow =
    Boolean(toolEv?.id) &&
    isActivityMessage(message) &&
    isToolLinkedInReactIndex(reactLinkIndex, toolEv?.id);

  if (suppressToolRow) {
    return {
      presentation: {
        reasoning: "",
        reactSteps: null,
        a2uiLines: null,
        bodyMarkdown: "",
        mode: "default",
      },
      reactStepsWithTools: [],
      suppressToolRow: true,
      structuredToolEvent: toolEv,
    };
  }

  const presentation = resolveAssistantPresentation(plannerKind, message);
  const reactStepsWithTools = reactStepsForIndex(
    reactLinkIndex.stepsByAssistantIndex.get(index),
    presentation
  );

  return {
    presentation,
    reactStepsWithTools,
    suppressToolRow: false,
    structuredToolEvent: toolEv,
  };
}
