import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { NetworkSchema } from "@gen/flavor/v1/network_pb";
import { describe, expect, it } from "vitest";
import { networkColorIndex, networkColorVar } from "./networkColor";

const net = (id: string, ms: number) => create(NetworkSchema, { id, createdAt: timestampFromMs(ms) });

describe("networkColorIndex", () => {
  it("assigns colours in creation order and wraps after eight", () => {
    const nets = Array.from({ length: 9 }, (_, i) => net(`n${i}`, 1000 - i));
    const idx = networkColorIndex(nets);
    expect(idx.get("n8")).toBe(0);
    expect(idx.get("n1")).toBe(7);
    expect(idx.get("n0")).toBe(0);
  });

  it("keeps a network's colour when another is added later", () => {
    const before = networkColorIndex([net("a", 1), net("b", 2)]);
    const after = networkColorIndex([net("c", 3), net("b", 2), net("a", 1)]);
    expect(after.get("a")).toBe(before.get("a"));
    expect(after.get("b")).toBe(before.get("b"));
  });

  it("falls back to the neutral colour for unknown networks", () => {
    expect(networkColorVar(undefined)).toBe("var(--ui-net-8)");
    expect(networkColorVar(2)).toBe("var(--ui-net-3)");
  });
});
