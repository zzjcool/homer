package web

import (
	"context"
	"testing"
	"time"
)

func TestProbeWriteLockReportsBusy(t *testing.T) {
	// 占用写锁
	writeMutex <- struct{}{}
	defer func() { <-writeMutex }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if ProbeWriteLock(ctx) {
		t.Fatal("ProbeWriteLock reported free while the write lock was held")
	}
}

func TestProbeWriteLockReportsFree(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !ProbeWriteLock(ctx) {
		t.Fatal("ProbeWriteLock reported busy while the write lock was free")
	}
	// 探针必须释放锁：再探一次仍应成功
	if !ProbeWriteLock(ctx) {
		t.Fatal("ProbeWriteLock did not release the lock after probing")
	}
}
