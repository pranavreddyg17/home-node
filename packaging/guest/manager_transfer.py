"""Disposable native fixture client for the actual transfer-service binary."""
import http.client
import json
import os
import re
import socket
import sys
import time
import uuid

import boot_image
import overlay


class UnixHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("local", timeout=30)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def main():
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1":
        raise ValueError("explicit unprivileged disposable Linux client required")
    if len(sys.argv) != 4 or not os.path.isabs(sys.argv[1]) or re.fullmatch(r"[A-Za-z0-9_-]{20,64}", sys.argv[2]) is None or sys.argv[3] not in ("allow", "deny"):
        raise ValueError("bounded native transfer fixture inputs required")
    path, instance, mode = sys.argv[1:]

    def request(_channel, operation, allowed_error=None, **fields):
        identifier = uuid.uuid4().hex
        body = json.dumps({"instanceId": instance, "request": {
            "version": 1, "requestId": identifier, "operation": operation, **fields}}).encode()
        if len(body) > 512 << 10:
            raise ValueError("transfer fixture frame exceeds bound")
        with UnixHTTP(path) as connection:
            connection.request("POST", "/v1/guest", body, {"Content-Type": "application/json"})
            response = connection.getresponse()
            data = response.read((512 << 10) + 1)
            if len(data) > 512 << 10:
                raise ValueError("transfer fixture response exceeds bound")
            if mode == "deny":
                if response.status != 403:
                    raise RuntimeError("foreign controller UID was not denied")
                return {}
            if response.status != 200:
                if response.status == 403:
                    raise RuntimeError("authorized controller UID was denied")
                raise ValueError("transfer fixture HTTP refusal " + str(response.status))
        result = json.loads(data, object_pairs_hook=overlay.unique_object)
        if not isinstance(result, dict) or type(result.get("version")) is not int or result.get("version") != 1 or result.get("requestId") != identifier or result.get("error"):
            raise RuntimeError("transfer fixture guest refusal")
        return result

    deadline = time.monotonic() + 120
    while True:
        try:
            result = request(None, "health")
            if mode == "deny" or result.get("state") == "ready":
                break
            raise ValueError("transfer guest not ready")
        except (OSError, http.client.HTTPException, ValueError):
            if time.monotonic() >= deadline:
                raise
            time.sleep(0.2)
    if mode == "allow":
        original = boot_image.request
        try:
            boot_image.request = request
            boot_image.object_roundtrip(None)
        finally:
            boot_image.request = original
    print("development transfer " + mode + " passed")


if __name__ == "__main__":
    main()
