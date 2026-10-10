"""Bounded development Files round trip on an admitted manager channel."""
import os
import socket
import sys
import time

import boot_image


def main():
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1":
        raise ValueError("explicit unprivileged disposable Linux client required")
    if len(sys.argv) != 2 or not os.path.isabs(sys.argv[1]):
        raise ValueError("absolute admitted channel required")
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
    print("development Files channel round trip passed")


if __name__ == "__main__":
    main()
