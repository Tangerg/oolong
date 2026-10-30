package content

import "testing"

func TestCancelledSourceSubscriptionsDoNotAccumulateWithoutSourceChanges(t *testing.T) {
	document, err := NewStreamDocument(StreamConfig{Format: "text"})
	if err != nil {
		t.Fatal(err)
	}
	for range 10_000 {
		cancel := document.Subscribe(func(DocumentChange) { t.Fatal("cancelled observer was called") })
		cancel()
		cancel()
	}
	if len(document.listeners) != 0 {
		t.Fatalf("retained %d cancelled subscriptions on an idle source", len(document.listeners))
	}
	if err := document.Append([]byte("later")); err != nil {
		t.Fatal(err)
	}
}
