import { useEffect, useState, useSyncExternalStore } from "react";
import { Capability } from "@gen/flavor/v1/common_pb";
import { useDaemon } from "../../app/sync/useDaemon";
import { getTrafficStats } from "../../lib/api/daemon";
import { windowShown } from "../../lib/windowState";
import type { Sample } from "./chart";

type Traffic = ReadonlyMap<string, Sample[]>;

const onVisibility = (notify: () => void) => {
  document.addEventListener("visibilitychange", notify);
  return () => document.removeEventListener("visibilitychange", notify);
};

export function useTraffic(): Traffic | null {
  const { status, info } = useDaemon();
  const supported = info?.capabilities.includes(Capability.TRAFFIC_STATS) ?? false;
  const visible = useSyncExternalStore(onVisibility, () => document.visibilityState === "visible");
  const [traffic, setTraffic] = useState<Traffic>(new Map());

  useEffect(() => {
    if (status !== "ready" || !supported || !visible) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = () =>
      windowShown()
        .then((shown) => (shown ? getTrafficStats() : null))
        .then((r) => {
          if (cancelled) return;
          if (!r) {
            setTraffic(new Map());
            return;
          }
          setTraffic(
            new Map(
              r.networks.map((n) => [
                n.networkId,
                n.samples.map((s) => ({
                  at: Number(s.atUnixMs),
                  rx: Number(s.rxBytesPerSecond),
                  tx: Number(s.txBytesPerSecond),
                })),
              ]),
            ),
          );
        })
        .catch(() => {})
        .finally(() => {
          if (!cancelled) timer = setTimeout(poll, 1000);
        });
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
      setTraffic(new Map());
    };
  }, [status, supported, visible]);

  return supported ? traffic : null;
}
