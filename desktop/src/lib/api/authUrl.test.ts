import { describe, expect, it } from "vitest";
import { authDestination } from "./authUrl";

describe("authDestination", () => {
  it("accepts http and https links and reports the destination host", () => {
    expect(authDestination("https://login.tailscale.com/a/abc")).toBe("login.tailscale.com");
    expect(authDestination("http://127.0.0.1:8080/register/x")).toBe("127.0.0.1:8080");
  });

  it("allows an identity provider host that differs from the control server", () => {
    expect(authDestination("https://accounts.example-idp.org/oauth?state=1")).toBe("accounts.example-idp.org");
  });

  it("rejects every other scheme and credentials in the link", () => {
    for (const bad of ["javascript:alert(1)", "file:///etc/passwd", "data:text/html,x", "ftp://a.example/", "https://u:p@a.example/", "", "not a url"]) {
      expect(authDestination(bad)).toBeNull();
    }
  });
});
