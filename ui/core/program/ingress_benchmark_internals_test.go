package program

import (
	"bytes"
	"fmt"
	"testing"
)

// BenchmarkByteIngressProducerBurst measures the boundary where a producer can run
// ahead of the interface owner. The producer writes one MiB in small chunks while
// the owner drains only posted work. The pending-byte limit, rather than the amount
// produced, is the maximum lead the producer can acquire.
func BenchmarkByteIngressProducerBurst(b *testing.B) {
	const total = 1 << 20
	payload := bytes.Repeat([]byte{'x'}, total)

	for _, limit := range []int{4 << 10, 64 << 10} {
		b.Run(fmt.Sprintf("limit=%dKiB", limit>>10), func(b *testing.B) {
			var batches int64
			b.SetBytes(total)
			b.ReportAllocs()
			for b.Loop() {
				queue := newTaskQueue()
				consumed := 0
				final := false
				ingress, err := NewByteIngress(ByteIngressConfig{Dispatcher: Dispatcher{tasks: queue}, Limit: limit, Consume: func(batch ByteBatch) {
					consumed += len(batch.Data)
					final = batch.Final
					batches++
				}})
				if err != nil {
					b.Fatal(err)
				}

				produced := make(chan error, 1)
				go func() {
					const chunk = 256
					for from := 0; from < len(payload); from += chunk {
						to := min(from+chunk, len(payload))
						if _, err := ingress.Write(payload[from:to]); err != nil {
							produced <- err
							return
						}
					}
					produced <- ingress.Close()
				}()

				for !final {
					<-queue.wake
					for _, task := range queue.take() {
						if task != nil {
							task()
						}
					}
				}
				if err := <-produced; err != nil {
					b.Fatal(err)
				}
				queue.stop()
				if consumed != total {
					b.Fatalf("consumed %d bytes, want %d", consumed, total)
				}
			}
			b.ReportMetric(float64(batches)/float64(b.N), "batches/op")
		})
	}
}
