# Recovery engine
Allowlisted actions only; each needs a permission and an authorized target. Modes: simulate (default), approval, auto.
Guards: global kill switch, per-target cooldown, retry limit (escalates), execution timeout.
Success is reported only if `Verify` passes; otherwise rollback (if defined) or failure.
