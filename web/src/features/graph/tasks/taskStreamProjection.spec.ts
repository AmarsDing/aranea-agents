import { describe, expect, it } from "vitest";
import {
  applyTaskStatusMetadata,
  seedTaskMap,
  tasksToList,
  wsTaskStatusToTaskStatus,
} from "./taskStreamProjection";
import type { Task } from "../types";

describe("taskStreamProjection", () => {
  it("maps ws task status strings", () => {
    expect(wsTaskStatusToTaskStatus("claimed")).toBe("TASK_CLAIMED");
    expect(wsTaskStatusToTaskStatus("review_required")).toBe("TASK_REVIEW_REQUIRED");
  });

  it("merges ws metadata into task", () => {
    const base: Task = {
      taskId: "t1",
      nodeId: "n1",
      executionId: "e1",
      assignee: "",
      status: "TASK_PENDING",
      context: "",
      input: "",
      output: "",
      summary: "",
      metadata: "",
      requiredRole: "",
      assignmentMode: "",
      createdAt: "",
      claimedAt: "",
      completedAt: "",
    };
    const updated = applyTaskStatusMetadata(base, {
      task_status: "claimed",
      assignee: "agent-a",
      summary: "done chunk",
    });
    expect(updated.status).toBe("TASK_CLAIMED");
    expect(updated.assignee).toBe("agent-a");
    expect(updated.summary).toBe("done chunk");
  });

  it("sorts tasks by node id", () => {
    const map = seedTaskMap([
      { ...emptyTask("t2", "e1", "b") },
      { ...emptyTask("t1", "e1", "a") },
    ]);
    expect(tasksToList(map).map((t) => t.nodeId)).toEqual(["a", "b"]);
  });
});

function emptyTask(taskId: string, executionId: string, nodeId: string): Task {
  return {
    taskId,
    nodeId,
    executionId,
    assignee: "",
    status: "TASK_PENDING",
    context: "",
    input: "",
    output: "",
    summary: "",
    metadata: "",
    requiredRole: "",
    assignmentMode: "",
    createdAt: "",
    claimedAt: "",
    completedAt: "",
  };
}
