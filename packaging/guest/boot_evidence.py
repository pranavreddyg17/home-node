"""Validate bounded development boot evidence; never authorize a release."""


def keys(value, expected):
    if type(value) is not dict or set(value) != set(expected):
        raise ValueError("development boot evidence fields refused")


def positive(value, maximum):
    if type(value) is not int or not 0 < value <= maximum:
        raise ValueError("development boot evidence number refused")


def cancellation(value):
    keys(value, {"cancelled", "taskDeletionAcknowledged"})
    if value["cancelled"] is not True or value["taskDeletionAcknowledged"] is not True:
        raise ValueError("development cancellation evidence incomplete")


def validate(value, manifest, accelerator):
    keys(value, {"schema", "profile", "imageSHA256", "accelerator", "tcgBootAndObjectRoundTrip",
                 "kvmBootAndObjectRoundTrip", "objectTransfer", "videoPresets", "videoCancellation",
                 "aiInference", "shutdown", "releaseQualified"})
    if type(manifest) is not dict or manifest.get("releaseQualified") is not False:
        raise ValueError("development manifest qualification refused")
    profile = manifest.get("profile")
    checksum = manifest.get("sha256")
    if type(profile) is not str or profile not in {"files", "video", "ai"} or type(checksum) is not str or len(checksum) != 64 or any(c not in "0123456789abcdef" for c in checksum):
        raise ValueError("development manifest identity refused")
    if type(value["schema"]) is not int or value["schema"] != 1 or value["profile"] != profile or value["imageSHA256"] != checksum or value["releaseQualified"] is not False:
        raise ValueError("development boot identity or qualification refused")
    if type(accelerator) is not str or accelerator not in {"tcg", "kvm"} or value["accelerator"] != accelerator or value["tcgBootAndObjectRoundTrip"] is not (accelerator == "tcg") or value["kvmBootAndObjectRoundTrip"] is not (accelerator == "kvm"):
        raise ValueError("development accelerator evidence refused")
    transfer = value["objectTransfer"]
    keys(transfer, {"bytes", "chunkBytes", "acknowledgedChunkReplay"})
    positive(transfer["bytes"], 128 << 20)
    if type(transfer["chunkBytes"]) is not int or transfer["chunkBytes"] != 256 << 10 or transfer["acknowledgedChunkReplay"] is not True:
        raise ValueError("development transfer evidence incomplete")
    shutdown = value["shutdown"]
    keys(shutdown, {"guestInitiated", "reason", "qemuExitCode", "readOnlyFilesystemCheck"})
    if shutdown["guestInitiated"] is not True or shutdown["reason"] != "guest-shutdown" or type(shutdown["qemuExitCode"]) is not int or shutdown["qemuExitCode"] != 0 or shutdown["readOnlyFilesystemCheck"] is not True:
        raise ValueError("development shutdown evidence incomplete")
    if profile == "video":
        presets = value["videoPresets"]
        if type(presets) is not list or len(presets) != 2:
            raise ValueError("development video evidence incomplete")
        for item, (preset, width, height) in zip(presets, [("mp4-720p", 1280, 720), ("mp4-1080p", 1920, 1080)]):
            keys(item, {"preset", "width", "height", "codec", "bytes"})
            if item["preset"] != preset or type(item["width"]) is not int or type(item["height"]) is not int or item["width"] != width or item["height"] != height or item["codec"] != "h264":
                raise ValueError("development video preset evidence refused")
            positive(item["bytes"], 2 << 20)
        cancellation(value["videoCancellation"])
    elif value["videoPresets"] != [] or value["videoCancellation"] is not None:
        raise ValueError("foreign video evidence refused")
    if profile == "ai":
        inference = value["aiInference"]
        keys(inference, {"generationSucceeded", "textBytes", "taskDeletionAcknowledged", "cancellation"})
        if inference["generationSucceeded"] is not True or inference["taskDeletionAcknowledged"] is not True:
            raise ValueError("development inference evidence incomplete")
        positive(inference["textBytes"], 32768)
        cancellation(inference["cancellation"])
    elif value["aiInference"] is not None:
        raise ValueError("foreign AI evidence refused")
