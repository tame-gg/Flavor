import { useCallback, useState } from "react";
import { errorMessage } from "../lib/api/errors";

export function useAction<A extends unknown[], R>(fn: (...args: A) => Promise<R>) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const run = useCallback(
    async (...args: A): Promise<{ ok: true; value: R } | { ok: false }> => {
      setPending(true);
      setError(null);
      try {
        return { ok: true, value: await fn(...args) };
      } catch (e) {
        setError(errorMessage(e));
        return { ok: false };
      } finally {
        setPending(false);
      }
    },
    [fn],
  );
  return { run, pending, error, clearError: () => setError(null) };
}
