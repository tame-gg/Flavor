import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CopyButton } from "./CopyButton";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("CopyButton", () => {
  it("shows the result and resets", async () => {
    vi.useFakeTimers();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    render(<CopyButton value="100.64.0.13" label="Copy" />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(writeText).toHaveBeenCalledWith("100.64.0.13");
    expect(screen.getByRole("button").textContent).toBe("Copied");
    act(() => {
      vi.advanceTimersByTime(1500);
    });
    expect(screen.getByRole("button").textContent).toBe("Copy");
  });
});
