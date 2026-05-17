package golang

import (
	"path/filepath"
	"testing"

	"costEngine/internal/entity/node"
)

// TestExtractCalls_F019Subkinds confirma os novos pares reconhecidos:
// gRPC, SQS/SNS/Kinesis e schedulers. Detecção é sintática — apenas
// confirma kind/framework/operation/topic capturados, não a resolução
// fina do alvo.
func TestExtractCalls_F019Subkinds(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	mustWrite(t, filepath.Join(root, "userservice", "svc.go"), `package userservice

import (
	"context"
)

type Svc struct {
	GRPC   grpcConn
	SQS    sqsClient
	SNS    snsClient
	KIN    kinesisClient
	Cron   cronScheduler
}

type grpcConn struct{}
func (g grpcConn) Invoke(ctx context.Context, method string, req, resp any) error { return nil }

type sqsClient struct{}
func (s sqsClient) SendMessage(ctx context.Context, in any) (any, error) { return nil, nil }
func (s sqsClient) ReceiveMessage(ctx context.Context, in any) (any, error) { return nil, nil }

type snsClient struct{}
func (s snsClient) Publish(ctx context.Context, in any) (any, error) { return nil, nil }

type kinesisClient struct{}
func (k kinesisClient) PutRecord(ctx context.Context, in any) (any, error) { return nil, nil }

type cronScheduler struct{}
func (c cronScheduler) AddFunc(spec string, fn func()) (int, error) { return 0, nil }

func tick() {}

func (s *Svc) Run(ctx context.Context) error {
	_ = s.GRPC.Invoke(ctx, "/payments.PaymentService/Charge", nil, nil)
	_, _ = s.SQS.SendMessage(ctx, nil)
	_, _ = s.SQS.ReceiveMessage(ctx, nil)
	_, _ = s.SNS.Publish(ctx, nil)
	_, _ = s.KIN.PutRecord(ctx, nil)
	_, _ = s.Cron.AddFunc("@every 5m", tick)
	return nil
}
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	byKind := map[node.CallKind][]node.Call{}
	for _, c := range res.Calls {
		byKind[c.Kind_] = append(byKind[c.Kind_], c)
	}

	// gRPC: TargetURL capturada do method path.
	rpc := byKind[node.CallRpcCall]
	if len(rpc) != 1 {
		t.Fatalf("RpcCall=%d want 1", len(rpc))
	}
	if rpc[0].TargetURL != "/payments.PaymentService/Charge" {
		t.Errorf("Rpc.TargetURL=%q", rpc[0].TargetURL)
	}

	// SQS: 1 send + 1 receive.
	if n := len(byKind[node.CallQueueSend]); n != 1 {
		t.Errorf("QueueSend=%d want 1", n)
	}
	if n := len(byKind[node.CallQueueReceive]); n != 1 {
		t.Errorf("QueueReceive=%d want 1", n)
	}

	// SNS Publish + Kinesis PutRecord → ambos EventPublish.
	ev := byKind[node.CallEventPublish]
	if len(ev) != 2 {
		t.Errorf("EventPublish=%d want 2 (SNS+Kinesis)", len(ev))
	}

	// Cron: 1 JobSchedule com spec capturada em Topic.
	js := byKind[node.CallJobSchedule]
	if len(js) != 1 {
		t.Fatalf("JobSchedule=%d want 1", len(js))
	}
	if js[0].Topic != "@every 5m" {
		t.Errorf("JobSchedule.Topic=%q want @every 5m", js[0].Topic)
	}
}
