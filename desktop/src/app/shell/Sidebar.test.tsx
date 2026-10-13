import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Sidebar } from "./Sidebar";

afterEach(cleanup);

describe("Sidebar", () => {
  it("marks the current page, shows counts and navigates", () => {
    const onPage = vi.fn();
    const onSearch = vi.fn();
    render(
      <Sidebar
        pages={[
          ["overview", "Overview"],
          ["devices", "Devices"],
          ["settings", "Settings"],
        ]}
        page="overview"
        onPage={onPage}
        counts={{ devices: 14 }}
        alerts={{ overview: 2 }}
        workspace="Work"
        status="unavailable"
        onSearch={onSearch}
      />,
    );
    expect(screen.getByRole("button", { name: /Overview/ }).getAttribute("aria-current")).toBe("page");
    expect(screen.getByRole("button", { name: /Devices/ }).textContent).toContain("14");
    expect(screen.getByRole("status").textContent).toContain("not reachable");
    fireEvent.click(screen.getByRole("button", { name: /Devices/ }));
    expect(onPage).toHaveBeenCalledWith("devices");
    fireEvent.click(screen.getByRole("button", { name: /Search/ }));
    expect(onSearch).toHaveBeenCalled();
  });
});
