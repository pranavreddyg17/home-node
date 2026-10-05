package backup

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
)

// serviceRequest keeps metadata responses outside the maintenance schemas.
// A per-connection response slot is populated only by successful listing.
type serviceRequest struct {
	maintenance *WorkerRequest
	snapshot    *SnapshotPageRequest
	page        *SnapshotPage
	preview     *SnapshotPreviewRequest
	inspection  *SnapshotPreview
}

func decodeServiceRequest(raw []byte) (serviceRequest, error) {
	if request, err := DecodeWorkerRequest(raw); err == nil {
		return serviceRequest{maintenance: &request}, nil
	}
	if request, err := DecodeSnapshotPageRequest(raw); err == nil {
		return serviceRequest{snapshot: &request, page: new(SnapshotPage)}, nil
	}
	if request, err := DecodeSnapshotPreviewRequest(raw); err == nil {
		return serviceRequest{preview: &request, inspection: new(SnapshotPreview)}, nil
	}
	return serviceRequest{}, ErrManifest
}

// ServeCredentialService serializes all operations on the installed worker
// listener. Metadata callbacks have no maintenance request or root token.
// Responses are sent after callback completion and credential closure.
func ServeCredentialService(ctx context.Context, listener net.Listener, controllerUID uint32, maintenance func(context.Context, WorkerRequest, *os.File) error, snapshot func(context.Context, SnapshotPageRequest, *os.File) (SnapshotPage, error), previews ...func(context.Context, SnapshotPreviewRequest, *os.File) (SnapshotPreview, error)) error {
	if maintenance == nil || snapshot == nil || len(previews) > 1 || (len(previews) == 1 && previews[0] == nil) {
		if listener != nil {
			listener.Close()
		}
		return ErrManifest
	}
	receive := func(ctx context.Context, connection *net.UnixConn, uid uint32) (serviceRequest, *os.File, error) {
		return receiveCredentialPayload(ctx, connection, uid, decodeServiceRequest)
	}
	work := func(ctx context.Context, request serviceRequest, credential *os.File) error {
		if request.maintenance != nil && request.snapshot == nil && request.page == nil && request.preview == nil && request.inspection == nil {
			return maintenance(ctx, *request.maintenance, credential)
		}
		if request.maintenance == nil && request.snapshot != nil && request.page != nil && request.preview == nil && request.inspection == nil {
			bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			page, err := snapshot(bounded, *request.snapshot, credential)
			if err != nil {
				return err
			}
			if err := bounded.Err(); err != nil {
				return err
			}
			if _, err := EncodeSnapshotPageResponse(request.snapshot.RequestID, page); err != nil {
				return err
			}
			*request.page = page
			return nil
		}
		if request.maintenance == nil && request.snapshot == nil && request.page == nil && request.preview != nil && request.inspection != nil && len(previews) == 1 {
			bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			inspection, err := previews[0](bounded, *request.preview, credential)
			if err != nil {
				return err
			}
			if err := bounded.Err(); err != nil {
				return err
			}
			if inspection.SnapshotID != request.preview.SnapshotID {
				return ErrManifest
			}
			if _, err := EncodeSnapshotPreviewResponse(request.preview.RequestID, inspection); err != nil {
				return err
			}
			*request.inspection = inspection
			return nil
		}
		return ErrManifest
	}
	complete := func(ctx context.Context, connection *net.UnixConn, request serviceRequest) error {
		if request.maintenance != nil && request.snapshot == nil && request.page == nil && request.preview == nil && request.inspection == nil {
			return sendWorkerRequestCompletion(ctx, connection, *request.maintenance)
		}
		if request.maintenance == nil && request.snapshot != nil && request.page != nil && request.preview == nil && request.inspection == nil {
			raw, err := EncodeSnapshotPageResponse(request.snapshot.RequestID, *request.page)
			if err != nil {
				return err
			}
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return disktransport.SendPacket(bounded, connection, raw)
		}
		if request.maintenance == nil && request.snapshot == nil && request.page == nil && request.preview != nil && request.inspection != nil {
			raw, err := EncodeSnapshotPreviewResponse(request.preview.RequestID, *request.inspection)
			if err != nil {
				return err
			}
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return disktransport.SendPacket(bounded, connection, raw)
		}
		return ErrManifest
	}
	refuse := func(ctx context.Context, connection *net.UnixConn, request serviceRequest, cause error) error {
		if request.maintenance == nil || request.snapshot != nil || request.page != nil || request.preview != nil || request.inspection != nil {
			return ErrManifest
		}
		return sendWorkerRequestRefusal(ctx, connection, *request.maintenance, cause)
	}
	return serveDispatch(ctx, listener, controllerUID, receive, work, complete, refuse)
}
