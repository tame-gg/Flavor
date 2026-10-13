import { describe, expect, it } from "vitest";
import { moveCursor } from "./nav";

describe("device list keyboard cursor", () => {
  it("moves one row at a time and stops at the ends", () => {
    expect(moveCursor("ArrowDown", 0, 3)).toBe(1);
    expect(moveCursor("ArrowDown", 2, 3)).toBe(2);
    expect(moveCursor("ArrowUp", 1, 3)).toBe(0);
    expect(moveCursor("ArrowUp", 0, 3)).toBe(0);
  });

  it("jumps to the first and last row", () => {
    expect(moveCursor("Home", 2, 3)).toBe(0);
    expect(moveCursor("End", 0, 3)).toBe(2);
  });

  it("ignores other keys and empty lists", () => {
    expect(moveCursor("Enter", 1, 3)).toBeNull();
    expect(moveCursor("ArrowDown", 0, 0)).toBeNull();
  });
});
