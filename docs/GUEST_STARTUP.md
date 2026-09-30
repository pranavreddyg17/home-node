# Guest startup and readiness

The immutable guest release must provide a bootloader/kernel, read-only system
root, writable ephemeral OS directories, a separately mounted writable data
volume and `/dev/virtio-ports/org.homenode.adapter`. The current host domain
uses a read-only raw system disk and a separate raw data disk; the guest agent
alone does not format, mount or qualify those disks. A maintained guest image
builder, qualified bootable release artifacts and actual boot/VM isolation
acceptance are still pending.

`homenode-guest` uses a fixed workload profile and opens its object directory
before serving the constrained virtio-serial protocol. Files/video currently
report agent readiness; those responses are not a boot/image qualification
certificate or proof that FFmpeg/media capability tests have passed.

For AI, the agent now requires the model server's fixed loopback
`GET http://127.0.0.1:8080/health` endpoint to return HTTP 200 and canonical
`status: ok` before reporting `ready`. Loading, connection errors, redirects,
malformed/trailing/duplicate JSON, unexpected fields and responses exceeding
4 KiB report `starting`; model/server error text is not sent to the controller.
The probe has a two-second deadline, no proxy and a separate connection pool
from generation, so a generation stream cannot hold the probe's only connection.
Optional nonnegative `slots_idle`/`slots_processing` fields are accepted for the
compatible server health contract. A pinned guest release must test that
contract against its actual model server version.

The contract follows the upstream [llama.cpp server health endpoint](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md#get-health-returns-health-check-result).
The image must run the reviewed model server on loopback only, with tools and
external integrations disabled and an independently licensed/hash-pinned CPU
model. These probes do not prove model licensing, model identity, acceptable
inference latency or resource isolation; release qualification must prove those.
The controller's existing bounded app-start loop waits for guest readiness and
reports failure if it never arrives.
