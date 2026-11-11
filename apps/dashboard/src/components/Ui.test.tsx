import { render, screen } from "@testing-library/react";
import { Empty, StatusBadge } from "./Ui";
import { fmtSeconds, validatePolicyInput } from "../services/format";

describe("StatusBadge", () => {
  it("renders the status text with underscores replaced", () => {
    render(<StatusBadge value="awaiting_approval" />);
    expect(screen.getByText("awaiting approval")).toBeInTheDocument();
  });
  it("renders unknown statuses with a neutral style", () => {
    render(<StatusBadge value="mystery" />);
    expect(screen.getByText("mystery").className).toContain("bg-ink-600");
  });
});

describe("Empty", () => {
  it("shows an empty state", () => {
    render(<Empty title="No agents yet">Register one</Empty>);
    expect(screen.getByText("No agents yet")).toBeInTheDocument();
  });
});

describe("format helpers", () => {
  it("formats durations", () => {
    expect(fmtSeconds(null)).toBe("—");
    expect(fmtSeconds(4.25)).toBe("4.3s");
    expect(fmtSeconds(125)).toBe("2m 5s");
  });
  it("validates policy forms", () => {
    const ok = { name: "p", retry_limit: 3, cooldown_s: 20, verify_timeout_s: 30, min_failures: 3, priority: 10 };
    expect(validatePolicyInput(ok)).toBeNull();
    expect(validatePolicyInput({ ...ok, name: " " })).toMatch(/Name/);
    expect(validatePolicyInput({ ...ok, retry_limit: 99 })).toMatch(/Retry/);
    expect(validatePolicyInput({ ...ok, verify_timeout_s: 1 })).toMatch(/Verification/);
  });
});
