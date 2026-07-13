package collab

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsio "github.com/nats-io/nats.go"
)

// TestCollab_LeafNodeE2E is the edge-sync e2e guard: a real central NATS
// (embedded, JetStream, with a Leaf Node listen port) runs the SyncWorker;
// a SEPARATE leaf-node server (the desktop edge) connects to it and
// publishes a Loro update on app.sync.<docID>. The leaf node replicates the
// message to central; the worker applies it and persists. This exercises
// the exact production path (Phase B transport + Phase C merge/persist)
// without mocking the leaf attachment.
func TestCollab_LeafNodeE2E(t *testing.T) {
	// 1) Central server with JetStream + a Leaf Node listen port.
	centralOpts := &server.Options{
		Port:      -1,
		NoLog:     true,
		NoSigs:    true,
		StoreDir:  t.TempDir(),
		JetStream: true,
		LeafNode: server.LeafNodeOpts{
			Host: "127.0.0.1",
			Port: 17422, // fixed so the leaf remote can target it
		},
	}
	central, err := server.NewServer(centralOpts)
	if err != nil {
		t.Fatalf("central server: %v", err)
	}
	central.Start()
	defer central.Shutdown()
	if !central.ReadyForConnections(10 * time.Second) {
		t.Fatal("central never ready")
	}
	centralNC, err := natsio.Connect(central.ClientURL())
	if err != nil {
		t.Fatalf("central connect: %v", err)
	}
	defer centralNC.Close()

	// 2) Central SyncWorker with a fake persister.
	fp := &fakePersister{snapshots: make(map[string][]byte)}
	docs := NewDocStore()
	worker := NewSyncWorker(centralNC, fp, docs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = worker.Run(ctx) }()

	// 3) Leaf node server attached to the central leaf port.
	leafURL := &url.URL{Scheme: "nats", Host: "127.0.0.1:17422"}
	leafOpts := &server.Options{
		Port:      -1,
		NoLog:     true,
		NoSigs:    true,
		StoreDir:  t.TempDir(),
		JetStream: true,
		LeafNode: server.LeafNodeOpts{
			Remotes: []*server.RemoteLeafOpts{{URLs: []*url.URL{leafURL}}},
		},
	}
	leaf, err := server.NewServer(leafOpts)
	if err != nil {
		t.Fatalf("leaf server: %v", err)
	}
	leaf.Start()
	defer leaf.Shutdown()
	if !leaf.ReadyForConnections(10 * time.Second) {
		t.Fatal("leaf never ready")
	}
	leafNC, err := natsio.Connect(leaf.ClientURL())
	if err != nil {
		t.Fatalf("leaf connect: %v", err)
	}
	defer leafNC.Close()

	// Wait for the leaf to attach to central.
	attached := false
	for i := 0; i < 50; i++ {
		if central.NumLeafNodes() > 0 {
			attached = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !attached {
		t.Fatal("leaf node never attached to central")
	}

	// 4) Edge publishes a Loro update on the leaf connection.
	edgeDoc := NewDoc("e2e-doc")
	update, err := edgeDoc.EncodeUpdate(nil)
	if err != nil {
		t.Fatalf("encode update: %v", err)
	}
	if err := leafNC.Publish("app.sync.e2e-doc", update); err != nil {
		t.Fatalf("leaf publish: %v", err)
	}

	// 5) Central worker must persist it (leaf replicated to central).
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if len(fp.get("e2e-doc")) > 0 {
			centralDoc := NewDoc("e2e-doc")
			if err := centralDoc.ApplyUpdate(fp.get("e2e-doc")); err != nil {
				t.Fatalf("persisted snapshot invalid Loro doc: %v", err)
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("leaf-node update was not replicated to central + persisted")
}
