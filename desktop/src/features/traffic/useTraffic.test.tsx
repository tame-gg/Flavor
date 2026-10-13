import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Capability } from "@gen/flavor/v1/common_pb";
import { useTraffic } from "./useTraffic";

const daemon = vi.hoisted(() => ({ status: "ready", info: { capabilities: [] as number[] } }));
const getTrafficStats = vi.hoisted(() => vi.fn());

vi.mock("../../app/sync/useDaemon", () => ({ useDaemon: () => daemon }));
vi.mock("../../lib/api/daemon", () => ({ getTrafficStats }));
const shown = vi.hoisted(() => ({ value: true }));
vi.mock("../../lib/windowState", () => ({ windowShown: () => Promise.resolve(shown.value) }));

let visibility = "visible";
let pending: ((r: unknown) => void)[] = [];

const response = {
  networks: [{ networkId: "n1", samples: [{ atUnixMs: 1000n, rxBytesPerSecond: 5n, txBytesPerSecond: 2n }] }],
};
const answer = async () => {
  await act(() => vi.advanceTimersByTimeAsync(0));
  pending.shift()?.(response);
  await act(() => vi.advanceTimersByTimeAsync(0));
};
const setVisibility = (v: string) =>
  act(() => {
    visibility = v;
    document.dispatchEvent(new Event("visibilitychange"));
  });

beforeEach(() => {
  vi.useFakeTimers();
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => visibility });
  visibility = "visible";
  shown.value = true;
  pending = [];
  daemon.status = "ready";
  daemon.info.capabilities = [Capability.TRAFFIC_STATS];
  getTrafficStats.mockReset().mockImplementation(() => new Promise((resolve) => pending.push(resolve)));
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useTraffic", () => {
  it("returns null and never polls without the capability", async () => {
    daemon.info.capabilities = [];
    const { result } = renderHook(() => useTraffic());
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(result.current).toBeNull();
    expect(getTrafficStats).not.toHaveBeenCalled();
  });

  it("keeps one request in flight and polls a second after each answer", async () => {
    const { result } = renderHook(() => useTraffic());
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(getTrafficStats).toHaveBeenCalledTimes(1);
    await answer();
    expect(result.current?.get("n1")).toEqual([{ at: 1000, rx: 5, tx: 2 }]);
    await act(() => vi.advanceTimersByTimeAsync(999));
    expect(getTrafficStats).toHaveBeenCalledTimes(1);
    await act(() => vi.advanceTimersByTimeAsync(1));
    expect(getTrafficStats).toHaveBeenCalledTimes(2);
  });

  it("stops and clears when the daemon leaves ready", async () => {
    const { result, rerender } = renderHook(() => useTraffic());
    await answer();
    await act(() => vi.advanceTimersByTimeAsync(1000));
    daemon.status = "unavailable";
    rerender();
    expect(result.current?.size).toBe(0);
    await answer();
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(result.current?.size).toBe(0);
    expect(getTrafficStats).toHaveBeenCalledTimes(2);
  });

  it("stops and clears while the window is hidden", async () => {
    const { result } = renderHook(() => useTraffic());
    await answer();
    expect(result.current?.size).toBe(1);
    setVisibility("hidden");
    expect(result.current?.size).toBe(0);
    await act(() => vi.advanceTimersByTimeAsync(5000));
    expect(getTrafficStats).toHaveBeenCalledTimes(1);
    setVisibility("visible");
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(getTrafficStats).toHaveBeenCalledTimes(2);
  });

  it("does not ask the daemon while the window is minimized or in the tray", async () => {
    shown.value = false;
    const { result } = renderHook(() => useTraffic());
    await act(() => vi.advanceTimersByTimeAsync(3500));
    expect(getTrafficStats).not.toHaveBeenCalled();
    expect(result.current?.size).toBe(0);
    shown.value = true;
    await act(() => vi.advanceTimersByTimeAsync(1000));
    expect(getTrafficStats).toHaveBeenCalledTimes(1);
  });
});
