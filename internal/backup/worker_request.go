package backup

import (
	"context"
	"net"
	"os"
)

// WorkerRequest admits exactly the reviewed launch and cleanup schemas. Legacy
// acquired dispatches are not accepted by this worker entry point.
type WorkerRequest struct {
	Launch  *Launch
	Cleanup *Cleanup
}

func DecodeWorkerRequest(raw []byte) (WorkerRequest, error) {
	if launch, err := DecodeLaunch(raw); err == nil {
		return WorkerRequest{Launch: &launch}, nil
	}
	if cleanup, err := DecodeCleanup(raw); err == nil {
		return WorkerRequest{Cleanup: &cleanup}, nil
	}
	return WorkerRequest{}, ErrManifest
}
func receiveCredentialWorkerRequest(ctx context.Context, connection *net.UnixConn, uid uint32) (WorkerRequest, *os.File, error) {
	return receiveCredentialPayload(ctx, connection, uid, DecodeWorkerRequest)
}
func sendWorkerRequestCompletion(ctx context.Context, connection *net.UnixConn, request WorkerRequest) error {
	if request.Launch != nil && request.Cleanup == nil {
		return sendLaunchCompletion(ctx, connection, *request.Launch)
	}
	if request.Cleanup != nil && request.Launch == nil {
		return sendCleanupCompletion(ctx, connection, *request.Cleanup)
	}
	return ErrManifest
}
func ServeAcknowledgedCredentialWorker(ctx context.Context, listener net.Listener, controllerUID uint32, work func(context.Context, WorkerRequest, *os.File) error) error {
	return serveDispatch(ctx, listener, controllerUID, receiveCredentialWorkerRequest, work, sendWorkerRequestCompletion, sendWorkerRequestRefusal)
}

// Cleanup failures cannot be represented as pre-acquisition launch refusals.
func sendWorkerRequestRefusal(ctx context.Context, connection *net.UnixConn, request WorkerRequest, cause error) error {
	if request.Launch == nil || request.Cleanup != nil {
		return ErrManifest
	}
	return sendLaunchRepositoryRefusal(ctx, connection, *request.Launch, cause)
}
