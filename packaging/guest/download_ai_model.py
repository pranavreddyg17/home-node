#!/usr/bin/env python3
"""Fetch only a pinned development model in an explicit disposable builder."""
import hashlib
import json
import os
from pathlib import Path
import sys
import time
import urllib.parse
import urllib.request
import build_ai_runtime

REPOSITORY = "bartowski/SmolLM2-135M-Instruct-GGUF"
REVISION = "09816acd5d99df7be770d85ea30822623dab342c"
FILENAME = "SmolLM2-135M-Instruct-Q8_0.gguf"
SIZE = 144811360
SHA256 = "5a1395716f7913741cc51d98581b9b1228d80987a9f7d3664106742eb06bba83"
URL = "https://huggingface.co/" + REPOSITORY + "/resolve/" + REVISION + "/" + FILENAME


def allowed_url(url):
    try:
        parsed = urllib.parse.urlsplit(url)
        port = parsed.port
    except ValueError:
        return False
    host = parsed.hostname or ""
    return parsed.scheme == "https" and parsed.username is None and parsed.password is None and port in (None, 443) and (
        host == "huggingface.co" or host.endswith(".huggingface.co") or host.endswith(".hf.co"))


class ModelRedirects(urllib.request.HTTPRedirectHandler):
    max_redirections = 3
    max_repeats = 1

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        if not allowed_url(newurl):
            raise ValueError("model redirect is not admitted")
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def copy_verified(response, output, size, checksum):
    deadline = time.monotonic() + 180
    copied, digest = 0, hashlib.sha256()
    while True:
        if time.monotonic() >= deadline:
            raise ValueError("model transfer timed out")
        chunk = response.read(min(1 << 20, size - copied + 1))
        if not chunk:
            break
        copied += len(chunk)
        if copied > size:
            raise ValueError("model transfer exceeds size")
        output.write(chunk)
        digest.update(chunk)
    if copied != size or digest.hexdigest() != checksum:
        raise ValueError("model transfer integrity mismatch")


def download(output):
    if sys.platform != "linux" or os.geteuid() == 0 or os.getenv("HOMENODE_AI_MODEL_BUILD") != "1":
        raise ValueError("requires explicit unprivileged disposable Linux builder")
    build_ai_runtime.private_directory(output.parent)
    if not output.is_absolute() or output != Path(os.path.abspath(output)):
        raise ValueError("expected canonical output")
    os.mkdir(output, 0o700)
    partial = output / "model.gguf.part"
    fd = os.open(partial, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "w+b") as file:
        os.posix_fallocate(file.fileno(), 0, SIZE)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), ModelRedirects())
        request = urllib.request.Request(URL, headers={"Accept-Encoding": "identity"})
        with opener.open(request, timeout=30) as response:
            if response.status != 200 or not allowed_url(response.url) or response.headers.get("Content-Encoding", "identity") != "identity":
                raise ValueError("model response is not admitted")
            copy_verified(response, file, SIZE, SHA256)
        file.flush()
        os.fsync(file.fileno())
    os.rename(partial, output / "model.gguf")
    record = {"schema": 1, "repository": REPOSITORY, "sourceRevision": REVISION,
              "sourceFilename": FILENAME, "sha256": SHA256, "bytes": SIZE,
              "publisherLicense": "Apache-2.0", "baseModel": "HuggingFaceTB/SmolLM2-135M-Instruct",
              "developmentOnly": True, "releaseQualified": False}
    with (output / "development-model.json").open("x") as file:
        json.dump(record, file, indent=2)
        file.write("\n")
    return record


if __name__ == "__main__":
    try:
        print(json.dumps(download(Path(sys.argv[1]))))
    except (OSError, ValueError, IndexError) as error:
        reason = str(error)[:240] if type(error) is ValueError else type(error).__name__
        sys.exit("Development model fetch failed: " + reason)
