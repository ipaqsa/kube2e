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
	// defaultExecTimeout is the hard deadline for a single Exec command.
	defaultExecTimeout = 30 * time.Second
)

// Config is the shared configuration passed to all Run* functions.
type Config struct {
	Kube        *svckube.Service
	Object      client.Object
	Annotations map[string]string
	Logger      *slog.Logger
}

// RunEnsure creates or updates the object on the cluster using Server-Side Apply.
func RunEnsure(ctx context.Context, conf *Config, act *Ensure) (*Report, error) {
	report := newReport("ensure", Target{Object: act.Object})
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "ensure", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("ensure object")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.Ensure(ctx, conf.Object,
		svckube.WithEnsureAnnotations(conf.Annotations),
		svckube.WithEnsureRetry(act.Retry.attempts(), act.Retry.backoff()))

	return finishReport(report, err)
}

// RunDelete removes the object from the cluster. When act.Wait is true it
// blocks until the object disappears or act.Time elapses.
func RunDelete(ctx context.Context, conf *Config, act *Delete) (*Report, error) {
	report := newReport("delete", act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "delete", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("delete object")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.Delete(ctx, conf.Object,
		svckube.WithDeleteWait(act.Wait, act.TimeoutOrDefault(defaultWaitTimeout)),
		svckube.WithDeleteInterval(act.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithDeleteRetry(act.Retry.attempts(), act.Retry.backoff()))

	return finishReport(report, err)
}

// RunWait polls the object until all JQ conditions pass or the timeout expires.
func RunWait(ctx context.Context, conf *Config, act *Wait) (*Report, error) {
	report := newReport("wait", act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "wait", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("wait for conditions")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.Wait(ctx, conf.Object,
		svckube.WithWaitConditions(act.Conditions),
		svckube.WithWaitInterval(act.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithWaitTimeout(act.TimeoutOrDefault(defaultWaitTimeout)))

	return finishReport(report, err)
}

// RunPatch applies RFC 6902 JSON patches to the rendered object and re-ensures it.
func RunPatch(ctx context.Context, conf *Config, act *Patch) (*Report, error) {
	report := newReport("patch", act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "patch", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	if len(act.Patches) == 0 {
		return finishReport(report, nil)
	}

	log.Info("patch object")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	opts := jsonpatch.NewApplyOptions()
	opts.EnsurePathExistsOnAdd = true

	patched, err := act.Patches.Apply(conf.Object, opts)
	if err != nil {
		return finishReport(report, fmt.Errorf("apply patch to object '%s': %w", conf.Object.GetName(), err))
	}

	err = conf.Kube.Ensure(ctx, patched,
		svckube.WithEnsureRetry(act.Retry.attempts(), act.Retry.backoff()))

	return finishReport(report, err)
}

// RunAssert fetches the object once and checks that all JQ conditions evaluate
// to true. When act.Retry is set the check is repeated up to Retry.Attempts
// times with Retry.Backoff between each attempt.
func RunAssert(ctx context.Context, conf *Config, act *Assert) (*Report, error) {
	report := newReport("assert", act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "assert", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("assert conditions")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.Check(ctx, conf.Object, act.Conditions,
		svckube.WithCheckRetry(act.Retry.attempts(), act.Retry.backoff()))

	return finishReport(report, err)
}

// RunLogs polls the logs of the named Pod until they contain act.Contains or
// the timeout expires.
func RunLogs(ctx context.Context, conf *Config, act *Logs) (*Report, error) {
	report := newReport("logs", act.Target)
	log := conf.Logger.With("action", "logs", "name", conf.Object.GetName())

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("wait for log output", "contains", act.Contains)

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.LogsContains(ctx, conf.Object, act.Contains,
		svckube.WithLogsContainer(act.Container),
		svckube.WithLogsMatch(svckube.LogsMatch(act.Match)),
		svckube.WithLogsInterval(act.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithLogsTimeout(act.TimeoutOrDefault(defaultWaitTimeout)))

	return finishReport(report, err)
}

// RunExec runs act.Command inside the resolved pod and succeeds when the
// command exits with code zero.
func RunExec(ctx context.Context, conf *Config, act *Exec) (*Report, error) {
	report := newReport("exec", act.Target)
	log := conf.Logger.With("action", "exec", "name", conf.Object.GetName())

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("exec command", "command", act.Command)

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.Exec(ctx, conf.Object, act.Command,
		svckube.WithExecContainer(act.Container),
		svckube.WithExecRetry(act.Retry.attempts(), act.Retry.backoff()),
		svckube.WithExecTimeout(act.TimeoutOrDefault(defaultExecTimeout)))

	return finishReport(report, err)
}

// applyDelay waits for the configured duration if set, returning early with
// the context error if the context is canceled before the delay elapses.
func applyDelay(ctx context.Context, log *slog.Logger, delay *metav1.Duration) error {
	if delay == nil || delay.Duration <= 0 {
		return nil
	}

	log.Info("action delay", "duration", delay.Duration)

	timer := time.NewTimer(delay.Duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
