"""Bounded development Files round trip on an admitted manager channel."""
import base64
import hashlib
import os
import socket
import sys
import time

import boot_image


def persistent_object(channel, identifier, attempt):
    content = b"HomeNode development restart persistence\n" * 128
    checksum = hashlib.sha256(content).hexdigest()
    if attempt == "0":
        uploaded = boot_image.request(channel, "upload", objectId=identifier, offset=0,
                                      size=len(content), sha256=checksum,
                                      data=base64.b64encode(content).decode())
        if uploaded.get("offset") != len(content):
            raise ValueError("persistent object upload mismatch")
        finalized = boot_image.request(channel, "finalize", objectId=identifier,
                                       size=len(content), sha256=checksum)
        if finalized.get("size") != len(content) or finalized.get("sha256") != checksum:
            raise ValueError("persistent object finalization mismatch")
    elif attempt == "1":
        downloaded = boot_image.request(channel, "download", objectId=identifier, offset=0)
        actual = base64.b64decode(downloaded.get("data", ""), validate=True)
        if actual != content or downloaded.get("offset") != len(content) or downloaded.get("sha256") != checksum:
            raise ValueError("object did not survive manager restart")
        boot_image.request(channel, "delete", objectId=identifier)
    else:
        raise ValueError("invalid restart attempt")


def main():
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1":
        raise ValueError("explicit unprivileged disposable Linux client required")
    if len(sys.argv) != 4 or not os.path.isabs(sys.argv[1]) or len(sys.argv[2]) != 32 or any(char not in "0123456789abcdef" for char in sys.argv[2]) or sys.argv[3] not in ("0", "1"):
        raise ValueError("absolute admitted channel and bounded restart identity required")
    # QEMU socket availability precedes guest adapter readiness. Keep one
    # connection: reconnecting may create a competing virtio channel consumer.
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as channel:
        channel.settimeout(120)
        channel.connect(sys.argv[1])
        deadline = time.monotonic() + 120
        while True:
            channel.settimeout(max(1, deadline - time.monotonic()))
            response = boot_image.request(channel, "health")
            if response.get("state") == "ready":
                break
            if response.get("state") != "starting" or time.monotonic() >= deadline:
                raise ValueError("guest adapter readiness deadline exceeded")
            time.sleep(0.25)
        channel.settimeout(30)
        result = boot_image.object_roundtrip(channel)
        if result != {"bytes": 1048579, "chunkBytes": 262144, "acknowledgedChunkReplay": True}:
            raise ValueError("unexpected Files round trip evidence")
        persistent_object(channel, sys.argv[2], sys.argv[3])
    print("development Files channel round trip passed")


if __name__ == "__main__":
    main()
