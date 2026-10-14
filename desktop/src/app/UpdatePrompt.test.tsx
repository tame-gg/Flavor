import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { toast, pendingUpdate } = vi.hoisted(() => ({
  toast: Object.assign(vi.fn(), { loading: vi.fn(), error: vi.fn() }),
  pendingUpdate: vi.fn(),
}));
vi.mock("sonner", () => ({ toast }));

vi.mock("../lib/api/daemon", () => ({
  installUpdate: vi.fn(),
  onUpdate: vi.fn(() => Promise.resolve(() => {})),
  pendingUpdate: () => pendingUpdate(),
}));

import { UpdatePrompt } from "./UpdatePrompt";

async function shown(flavordUpdate: string | null) {
  pendingUpdate.mockResolvedValue({ version: "1.2.0", flavordUpdate });
  render(<UpdatePrompt />);
  await vi.waitFor(() => expect(toast).toHaveBeenCalled());
  return toast.mock.calls[0][1];
}

describe("UpdatePrompt", () => {
  beforeEach(() => toast.mockClear());

  it("offers a restart when flavord is current", async () => {
    const opts = await shown(null);
    expect(opts.action.label).toBe("Restart to update");
    expect(opts.cancel.label).toBe("Later");
  });

  it("shows only instructions when flavord needs updating", async () => {
    const opts = await shown("Run brew upgrade flavor first.");
    expect(opts.action).toBeUndefined();
    expect(opts.cancel.label).toBe("Later");
    expect(opts.description).toBe("Run brew upgrade flavor first.");
  });
});
