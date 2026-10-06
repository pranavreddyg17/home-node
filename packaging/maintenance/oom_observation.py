"""Bounded fixture observation; never infer OOM from service state alone."""
import time


def wait_for_oom(query, *, monotonic=time.monotonic, sleep=time.sleep, timeout=60):
    deadline = monotonic() + timeout
    terminal_seen = False
    while monotonic() < deadline:
        result = query("Result")
        if result == "oom-kill":
            return
        phase = query("ActiveState")
        if phase in ("failed", "inactive"):
            # The manager may have processed OOM after the first Result query.
            # Reobserve Result after the terminal state; never accept phase alone.
            if query("Result") == "oom-kill":
                return
            terminal_seen = True
        sleep(0.2)
    if terminal_seen:
        raise RuntimeError("fixture stopped without an OOM result")
    raise RuntimeError("bounded service did not report OOM")
