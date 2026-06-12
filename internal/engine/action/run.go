// Package action implements Kubernetes resource operations (Ensure, Delete, Wait, Patch, Assert).
package action

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svckube "github.com/ipaqsa/kube2e/internal/kube"
)

const (
	// defaultWaitInterval is the polling cadence for Wait.
	defaultWaitInterval = 2 * time.Second
	// defaultWaitTimeout is the hard deadline for Wait and delete-wait.
	defaultWaitTimeout = 2 * time.Minute
)

// Config is the shared configuration passed to all Run* functions.
type Config struct {
	Kube        *svckube.Service
	Object      client.Object
	Annotations map[string]string
	Logger      *slog.Logger
}

// RunEnsure creates or updates the object on the cluster using Server-Side Apply.
func RunEnsure(ctx context.Context, conf *Config, act *Ensure) error {
	log := conf.Logger.With("action", "ensure", "name", conf.Object.GetName())

	applyDelay(log, act.Delay)

	log.Info("ensure object")

	start := time.Now()
	defer func() { log.Info("action finished", "duration", time.Since(start)) }()

	return conf.Kube.Ensure(ctx, conf.Object,
		svckube.WithEnsureAnnotations(conf.Annotations),
		svckube.WithEnsureRetry(act.Retry.attempts(), act.Retry.backoff()))
}

// RunDelete removes the object from the cluster. When act.Wait is true it
// blocks until the object disappears or act.Time elapses.
func RunDelete(ctx context.Context, conf *Config, act *Delete) error {
	log := conf.Logger.With("action", "delete", "name", conf.Object.GetName())

	applyDelay(log, act.Delay)

	log.Info("delete object")

	start := time.Now()
	defer func() { log.Info("action finished", "duration", time.Since(start)) }()

	return conf.Kube.Delete(ctx, conf.Object,
		svckube.WithDeleteWait(act.Wait, act.TimeoutOrDefault(defaultWaitTimeout)),
		svckube.WithDeleteInterval(act.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithDeleteRetry(act.Retry.attempts(), act.Retry.backoff()))
}

// RunWait polls the object until all JQ conditions pass or the timeout expires.
func RunWait(ctx context.Context, conf *Config, act *Wait) error {
	log := conf.Logger.With("action", "wait", "name", conf.Object.GetName())

	applyDelay(log, act.Delay)

	log.Info("wait for conditions")

	start := time.Now()
	defer func() { log.Info("action finished", "duration", time.Since(start)) }()

	return conf.Kube.Wait(ctx, conf.Object,
		svckube.WithWaitConditions(act.Conditions),
		svckube.WithWaitInterval(act.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithWaitTimeout(act.TimeoutOrDefault(defaultWaitTimeout)))
}

// RunPatch applies RFC 6902 JSON patches to the rendered object and re-ensures it.
func RunPatch(ctx context.Context, conf *Config, act *Patch) error {
	log := conf.Logger.With("action", "patch", "name", conf.Object.GetName())

	applyDelay(log, act.Delay)

	if len(act.Patches) == 0 {
		return nil
	}

	log.Info("patch object")

	start := time.Now()
	defer func() { log.Info("action finished", "duration", time.Since(start)) }()

	opts := jsonpatch.NewApplyOptions()
	opts.EnsurePathExistsOnAdd = true

	patched, err := act.Patches.Apply(conf.Object, opts)
	if err != nil {
		return fmt.Errorf("apply patch to object '%s': %w", conf.Object.GetName(), err)
	}

	return conf.Kube.Ensure(ctx, patched,
		svckube.WithEnsureRetry(act.Retry.attempts(), act.Retry.backoff()))
}

// RunAssert fetches the object once and checks that all JQ conditions evaluate
// to true. When act.Retry is set the check is repeated up to Retry.Attempts
// times with Retry.Backoff between each attempt.
func RunAssert(ctx context.Context, conf *Config, act *Assert) error {
	log := conf.Logger.With("action", "assert", "name", conf.Object.GetName())

	applyDelay(log, act.Delay)

	log.Info("assert conditions")

	start := time.Now()
	defer func() { log.Info("action finished", "duration", time.Since(start)) }()

	return conf.Kube.Check(ctx, conf.Object, act.Conditions,
		svckube.WithCheckRetry(act.Retry.attempts(), act.Retry.backoff()))
}

// applyDelay sleeps for the configured duration if set.
func applyDelay(log *slog.Logger, delay *metav1.Duration) {
	if delay == nil || delay.Duration <= 0 {
		return
	}

	log.Info("action delay", "duration", delay.Duration)
	time.Sleep(delay.Duration)
}
