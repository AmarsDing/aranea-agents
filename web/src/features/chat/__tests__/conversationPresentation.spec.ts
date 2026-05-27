import { describe, expect, it } from "vitest";
import {
  presentConversationSource,
  presentDeliveryStatus,
  presentRunStatus,
} from "../../../domain/conversationPresentation";

describe("conversationPresentation", () => {
  it("maps technical run states to user-facing labels", () => {
    expect(presentRunStatus("running")).toMatchObject({ label: "正在生成", tone: "info" });
    expect(presentRunStatus("awaiting_user")).toMatchObject({ label: "等你确认", tone: "warning" });
    expect(presentRunStatus("failed")).toMatchObject({ label: "失败可重试", tone: "danger" });
  });

  it("maps channel delivery states to user-facing labels", () => {
    expect(presentDeliveryStatus("delivered")).toMatchObject({ label: "已送达", tone: "success" });
    expect(presentDeliveryStatus("failed")).toMatchObject({ label: "发送失败", tone: "danger" });
  });

  it("names conversation sources", () => {
    expect(presentConversationSource("channel")).toBe("外部 Channel");
    expect(presentConversationSource("durable")).toBe("后台任务");
  });
});
