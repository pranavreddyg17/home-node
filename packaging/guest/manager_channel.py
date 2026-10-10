"""Bounded development Files round trip on an admitted manager channel."""
import base64
import contextlib
import hashlib
import json
import os
import re
import socket
import struct
import sys
import time
import uuid

import boot_image


def large_object_roundtrip(channel, reconnect):
    identifier = uuid.uuid4().hex
    chunk = bytes(range(256)) * 1024
    chunk_digest = hashlib.sha256(chunk).hexdigest()
    total = 1 << 30
    digest = hashlib.sha256()
    for offset in range(0, total, len(chunk)):
        fields = {"objectId": identifier, "offset": offset, "size": total,
                  "sha256": chunk_digest, "data": base64.b64encode(chunk).decode()}
        if offset == 2 * len(chunk):
            # Commit may happen before or after disconnect. Do not consume an
            # acknowledgment; replay the identical request on a new channel.
            peer = channel.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12)
            payload = json.dumps({"version": 1, "requestId": uuid.uuid4().hex,
                                  "operation": "upload", **fields}).encode()
            if len(payload) > 512 << 10:
                raise ValueError("fixture request exceeds frame bound")
            channel.sendall(struct.pack(">I", len(payload)) + payload)
            channel.close()
            channel = reconnect()
            if channel.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12) != peer:
                raise ValueError("guest peer changed across unacknowledged reconnect")
        try:
            uploaded = boot_image.request(channel, "upload", **fields)
        except ValueError as error:
            raise ValueError("large object upload refused at offset " + str(offset)) from error
        if uploaded.get("offset") != offset + len(chunk):
            raise ValueError("large object upload offset mismatch")
        digest.update(chunk)
        if offset == len(chunk):
            peer = channel.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12)
            channel.close()
            channel = reconnect()
            if channel.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12) != peer:
                raise ValueError("guest peer changed across transfer reconnect")
            if boot_image.request(channel, "upload", **fields).get("offset") != offset + len(chunk):
                raise ValueError("large object acknowledged replay mismatch")
    checksum = digest.hexdigest()
    finalized = boot_image.request(channel, "finalize", objectId=identifier, size=total, sha256=checksum)
    if finalized.get("size") != total or finalized.get("sha256") != checksum:
        raise ValueError("large object finalization mismatch")
    downloaded = hashlib.sha256()
    for offset in range(0, total, len(chunk)):
        response = boot_image.request(channel, "download", objectId=identifier, offset=offset)
        actual = base64.b64decode(response.get("data", ""), validate=True)
        if actual != chunk or response.get("offset") != offset + len(chunk) or response.get("sha256") != chunk_digest:
            raise ValueError("large object download integrity mismatch")
        downloaded.update(actual)
    if downloaded.hexdigest() != checksum:
        raise ValueError("large object complete digest mismatch")
    boot_image.request(channel, "delete", objectId=identifier)
    return channel


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
    if len(sys.argv) != 4 or not os.path.isabs(sys.argv[1]) or re.fullmatch(r"[A-Za-z0-9_-]{20,64}", sys.argv[2]) is None or sys.argv[3] not in ("0", "1"):
        raise ValueError("absolute admitted channel and bounded restart identity required")
    # QEMU socket availability precedes guest adapter readiness. Every reconnect
    # closes its predecessor first; never create competing channel consumers.
    with contextlib.ExitStack() as cleanup:
        def connect():
            connection = cleanup.enter_context(socket.socket(socket.AF_UNIX, socket.SOCK_STREAM))
            connection.settimeout(30)
            connection.connect(sys.argv[1])
            return connection
        channel = connect()
        channel.settimeout(120)
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
        if sys.argv[3] == "0":
            channel = large_object_roundtrip(channel, connect)
        persistent_object(channel, sys.argv[2], sys.argv[3])
    print("development Files channel round trip passed")


if __name__ == "__main__":
    main()
