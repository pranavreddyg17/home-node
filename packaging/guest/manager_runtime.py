"""Explicit disposable controller client for the actual supervisor executable."""
import json
import os
import re
import sys
import uuid

import manager_transfer
import overlay


def main():
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1":
        raise ValueError("explicit disposable Linux controller UID required")
    if len(sys.argv) != 6:
        raise ValueError("bounded runtime fixture inputs required")
    path, instance, action, uid_text, controller_text = sys.argv[1:]
    if re.fullmatch(r"[1-9][0-9]{0,9}", controller_text) is None or not 0 < int(controller_text) <= 2147483647 or os.geteuid() != int(controller_text):
        raise ValueError("accepted controller policy identity required")
    if not os.path.isabs(path) or re.fullmatch(r"[A-Za-z0-9_-]{20,64}", instance) is None or action not in ("start", "shutdown") or re.fullmatch(r"[1-9][0-9]{4,9}", uid_text) is None:
        raise ValueError("bounded runtime fixture inputs required")
    uid = int(uid_text)
    if not 65536 <= uid <= 2147483647:
        raise ValueError("reserved guest UID required")
    revision, state = (1, "running") if action == "start" else (2, "stopped")
    body = json.dumps({"version": 1, "operationId": uuid.uuid4().hex, "action": action,
                       "workload": "files", "instanceId": instance, "policyGeneration": 1,
                       "revision": revision}).encode()
    # One operation ID and one request: ambiguous mutations are never retried
    # with a different identity by this fixture.
    with manager_transfer.UnixHTTP(path) as connection:
        connection.timeout = 95
        connection.request("POST", "/v1/runtime", body, {"Content-Type": "application/json"})
        response = connection.getresponse()
        data = response.read(4097)
        if response.status != 200 or len(data) > 4096:
            raise RuntimeError("runtime operation refused or response exceeds bound")
    result = json.loads(data, object_pairs_hook=overlay.unique_object)
    if not isinstance(result, dict) or result.get("id") != instance or result.get("workload") != "files" or result.get("state") != state or type(result.get("revision")) is not int or result["revision"] != revision or type(result.get("guestUid")) is not int or result["guestUid"] != uid:
        raise RuntimeError("runtime operation identity or state mismatch")
    print("development runtime " + action + " passed")


if __name__ == "__main__":
    main()
