package main

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessRecordsConcurrencyAndOrder(t *testing.T) {
	records := [][]string{{"domain"}, {""}}
	for i := 0; i < 30; i++ {
		records = append(records, []string{fmt.Sprint(i)})
	}
	var active, peak atomic.Int32
	started := make(chan struct{}, 30)
	release := make(chan struct{})
	done := make(chan []hashResult, 1)
	go func() {
		done <- processRecords(records, 10, func(domain string) (string, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			started <- struct{}{}
			<-release
			if domain == "7" {
				return "", fmt.Errorf("fetch failed")
			}
			return "hash-" + domain, nil
		})
	}()
	for i := 0; i < 10; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("workers did not run concurrently")
		}
	}
	close(release)
	var results []hashResult
	select {
	case results = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not finish")
	}
	if peak.Load() != 10 {
		t.Fatalf("peak concurrency = %d", peak.Load())
	}
	if results[0].row != nil || results[0].err != nil {
		t.Fatal("header not skipped")
	}
	if results[1].err == nil {
		t.Fatal("empty domain accepted")
	}
	for i := 0; i < 30; i++ {
		r := results[i+2]
		if i == 7 {
			if r.err == nil {
				t.Fatal("fetch failure lost")
			}
			continue
		}
		if r.err != nil || len(r.row) != 2 || r.row[0] != fmt.Sprint(i) || r.row[1] != fmt.Sprintf("hash-%d", i) {
			t.Fatalf("incorrect result at %d: %+v", i, r)
		}
	}
}
