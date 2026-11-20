package sysproxyapi

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/xishang0128/sysproxy-go/sysproxy"
)

type retryTestGuardRunner struct {
	sysproxyGuardRunner
	attempts int
	change   chan struct{}
}

func (r *retryTestGuardRunner) Query(*sysproxy.Options) (*sysproxy.ProxyConfig, error) {
	return &sysproxy.ProxyConfig{}, nil
}

func (r *retryTestGuardRunner) WaitChangeReady(ctx context.Context, _ *sysproxy.Options, ready func()) error {
	r.attempts++
	switch r.attempts {
	case 1, 2, 4:
		return errors.New("watch unavailable")
	default:
		ready()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.change:
			return nil
		}
	}
}

func (r *retryTestGuardRunner) Close() error { return nil }

func TestSysproxyGuardRetriesWatchErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events, unsubscribe := subscribeSysproxyEvents(16)
		defer unsubscribe()
		<-events // Discard the initial snapshot.

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runner := &retryTestGuardRunner{change: make(chan struct{})}
		done := make(chan struct{})
		go func() {
			defer close(done)
			(&sysproxyGuardState{}).run(ctx, 0, &sysproxyGuardConfig{
				mode:     sysproxyGuardModeProxy,
				expected: newSysproxyGuardSnapshot(sysproxyGuardModeProxy, &sysproxy.ProxyConfig{}),
				runner:   runner,
			})
		}()

		synctest.Wait()
		if runner.attempts != 1 || len(events) != 1 {
			t.Fatalf("before retry: got %d attempts and %d events, want 1 of each", runner.attempts, len(events))
		}
		if event := <-events; event.Type != sysproxyEventGuardWatchFailed || event.Error != "watch unavailable" {
			t.Fatalf("unexpected first watch failure event: %+v", event)
		}

		time.Sleep(time.Second / 2)
		synctest.Wait()
		if runner.attempts != 1 {
			t.Fatalf("retried before one second: got %d attempts", runner.attempts)
		}
		time.Sleep(time.Second / 2)
		synctest.Wait()
		if runner.attempts != 2 || len(events) != 0 {
			t.Fatalf("after retry: got %d attempts and %d duplicate events, want 2 and 0", runner.attempts, len(events))
		}

		time.Sleep(time.Second)
		synctest.Wait()
		if runner.attempts != 3 {
			t.Fatalf("recovery: got %d attempts, want 3", runner.attempts)
		}
		runner.change <- struct{}{}
		synctest.Wait()
		if runner.attempts != 4 || len(events) != 1 {
			t.Fatalf("after a change: got %d attempts and %d events, want 4 and 1", runner.attempts, len(events))
		}
		if event := <-events; event.Type != sysproxyEventGuardWatchFailed || event.Error != "watch unavailable" {
			t.Fatalf("unexpected watch failure event after recovery: %+v", event)
		}

		stoppedAt := time.Now()
		cancel()
		<-done
		if elapsed := time.Since(stoppedAt); elapsed != 0 {
			t.Fatalf("stopping the guard waited %v during retry delay", elapsed)
		}
	})
}
