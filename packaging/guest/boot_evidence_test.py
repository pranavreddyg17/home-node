import copy
import sys
import unittest
sys.dont_write_bytecode = True
import boot_evidence


def fixture(profile="files", accelerator="tcg"):
    manifest = {"profile": profile, "sha256": "a" * 64, "releaseQualified": False}
    report = {"schema": 1, "profile": profile, "imageSHA256": "a" * 64, "accelerator": accelerator,
              "tcgBootAndObjectRoundTrip": accelerator == "tcg", "kvmBootAndObjectRoundTrip": accelerator == "kvm",
              "objectTransfer": {"bytes": 1048579, "chunkBytes": 262144, "acknowledgedChunkReplay": True},
              "videoPresets": [], "videoCancellation": None, "aiInference": None,
              "shutdown": {"guestInitiated": True, "reason": "guest-shutdown", "qemuExitCode": 0, "readOnlyFilesystemCheck": True},
              "releaseQualified": False}
    cancellation = {"cancelled": True, "taskDeletionAcknowledged": True}
    if profile == "video":
        report["videoPresets"] = [{"preset": p, "width": w, "height": h, "codec": "h264", "bytes": 1024}
                                  for p, w, h in [("mp4-720p", 1280, 720), ("mp4-1080p", 1920, 1080)]]
        report["videoCancellation"] = cancellation
    if profile == "ai":
        report["aiInference"] = {"generationSucceeded": True, "textBytes": 20, "taskDeletionAcknowledged": True, "cancellation": cancellation}
    return manifest, report


class BootEvidenceTests(unittest.TestCase):
    def test_each_profile_and_accelerator_stays_development_only(self):
        for profile in ("files", "video", "ai"):
            for accelerator in ("tcg", "kvm"):
                manifest, report = fixture(profile, accelerator)
                boot_evidence.validate(report, manifest, accelerator)
                self.assertIs(report["releaseQualified"], False)

    def test_incomplete_or_misclassified_reports_refused(self):
        mutations = [lambda r: r.update(schema=True), lambda r: r.update(imageSHA256="b" * 64),
                     lambda r: r.update(releaseQualified=True), lambda r: r.update(unreviewed=True),
                     lambda r: r.update(kvmBootAndObjectRoundTrip=True), lambda r: r.update(accelerator="kvm"),
                     lambda r: r["shutdown"].update(qemuExitCode=False),
                     lambda r: r["shutdown"].update(reason="host-signal"),
                     lambda r: r["shutdown"].update(readOnlyFilesystemCheck=False),
                     lambda r: r["objectTransfer"].update(bytes=True),
                     lambda r: r["objectTransfer"].update(acknowledgedChunkReplay=False)]
        for mutate in mutations:
            manifest, report = fixture()
            mutate(report)
            with self.assertRaises(ValueError):
                boot_evidence.validate(report, manifest, "tcg")

    def test_video_requires_both_presets_and_cancellation(self):
        manifest, original = fixture("video")
        for mutate in (lambda r: r["videoPresets"].pop(), lambda r: r["videoPresets"][0].update(width=True),
                       lambda r: r["videoPresets"][1].update(bytes=3 << 20),
                       lambda r: r["videoCancellation"].update(cancelled=False)):
            report = copy.deepcopy(original)
            mutate(report)
            with self.assertRaises(ValueError):
                boot_evidence.validate(report, manifest, "tcg")

    def test_ai_requires_generation_deletion_and_cancellation(self):
        manifest, original = fixture("ai", "kvm")
        for mutate in (lambda r: r["aiInference"].update(generationSucceeded=False),
                       lambda r: r["aiInference"].update(textBytes=32769),
                       lambda r: r["aiInference"].update(taskDeletionAcknowledged=False),
                       lambda r: r["aiInference"]["cancellation"].update(cancelled=1)):
            report = copy.deepcopy(original)
            mutate(report)
            with self.assertRaises(ValueError):
                boot_evidence.validate(report, manifest, "kvm")
