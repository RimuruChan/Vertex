package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
)

type TaskTerminationReason string

const (
	TaskCompleted      TaskTerminationReason = "completed"
	TaskNodeFailure    TaskTerminationReason = "node-failure"
	TaskOutputLimit    TaskTerminationReason = "output-limit"
	TaskIdleTimeout    TaskTerminationReason = "idle-timeout"
	TaskDeadline       TaskTerminationReason = "deadline"
	TaskCancelled      TaskTerminationReason = "cancelled"
	TaskTransportError TaskTerminationReason = "transport-error"
)

type DuplexSide string

const (
	DuplexLeft  DuplexSide = "left"
	DuplexRight DuplexSide = "right"
)

// DuplexOptions contains limits owned by the trusted stream broker. Native
// CPU, wall, memory and per-role output limits remain in each Execution.
type DuplexOptions struct {
	IdleTimeout     time.Duration
	TranscriptBytes int64
}

// DuplexTranscriptEvent is one broker-observed output chunk. Sequence defines
// a total order across both directions; it starts at one and is contiguous for
// all retained events.
type DuplexTranscriptEvent struct {
	Sequence uint64
	Side     DuplexSide
	Data     []byte
}

type DuplexTranscript struct {
	LeftToRight []byte
	RightToLeft []byte
	Events      []DuplexTranscriptEvent
	Truncated   bool
}

type DuplexResult struct {
	Left               *Result
	Right              *Result
	TerminationReason  TaskTerminationReason
	LimitSide          DuplexSide
	LeftToRightBytes   int64
	RightToLeftBytes   int64
	TerminationMessage string
	Transcript         DuplexTranscript
}

type streamLimitError struct {
	limit int64
}

func (e *streamLimitError) Error() string {
	return fmt.Sprintf("stream reached %d byte limit", e.limit)
}

type duplexEndpoint struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	wait   func() (*Result, error)
	close  func()
}

type relayEvent struct {
	side  DuplexSide
	bytes int64
	err   error
}

type waitEvent struct {
	side   DuplexSide
	result *Result
	err    error
}

type transcriptRecorder struct {
	mu        sync.Mutex
	remaining int64
	value     DuplexTranscript
}

func (r *transcriptRecorder) record(side DuplexSide, data []byte) {
	if r == nil || len(data) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.remaining <= 0 {
		r.value.Truncated = true
		return
	}
	count := int64(len(data))
	if count > r.remaining {
		count = r.remaining
		r.value.Truncated = true
	}
	if side == DuplexLeft {
		r.value.LeftToRight = append(r.value.LeftToRight, data[:count]...)
	} else {
		r.value.RightToLeft = append(r.value.RightToLeft, data[:count]...)
	}
	eventData := append([]byte(nil), data[:count]...)
	r.value.Events = append(r.value.Events, DuplexTranscriptEvent{
		Sequence: uint64(len(r.value.Events) + 1),
		Side:     side,
		Data:     eventData,
	})
	r.remaining -= count
}

func (r *transcriptRecorder) snapshot() DuplexTranscript {
	if r == nil {
		return DuplexTranscript{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := DuplexTranscript{
		LeftToRight: append([]byte(nil), r.value.LeftToRight...),
		RightToLeft: append([]byte(nil), r.value.RightToLeft...),
		Events:      make([]DuplexTranscriptEvent, len(r.value.Events)),
		Truncated:   r.value.Truncated,
	}
	for index, event := range r.value.Events {
		result.Events[index] = event
		result.Events[index].Data = append([]byte(nil), event.Data...)
	}
	return result
}

// RunDuplex connects two independent sandboxes through a trusted, bounded
// broker. It is the base topology for interactive judging; role-specific exit
// codes and verdicts are interpreted by the caller.
func RunDuplex(
	ctx context.Context,
	leftSandbox *Sandbox,
	leftExecution Execution,
	rightSandbox *Sandbox,
	rightExecution Execution,
	options DuplexOptions,
) (*DuplexResult, error) {
	if leftSandbox == nil || rightSandbox == nil {
		return nil, fmt.Errorf("both duplex sandboxes are required")
	}
	if leftSandbox.BoxDir() == rightSandbox.BoxDir() {
		return nil, fmt.Errorf("duplex roles require distinct sandboxes")
	}
	if options.IdleTimeout <= 0 {
		return nil, fmt.Errorf("duplex idle timeout must be positive")
	}
	if options.TranscriptBytes < 0 {
		return nil, fmt.Errorf("duplex transcript limit must be non-negative")
	}

	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leftProcess, err := leftSandbox.startStreaming(taskCtx, leftExecution)
	if err != nil {
		return nil, fmt.Errorf("start left sandbox: %w", err)
	}
	rightProcess, err := rightSandbox.startStreaming(taskCtx, rightExecution)
	if err != nil {
		cancel()
		leftProcess.closeIO()
		_, _ = leftProcess.wait()
		return nil, fmt.Errorf("start right sandbox: %w", err)
	}

	left := duplexEndpoint{
		stdin: leftProcess.stdin, stdout: leftProcess.stdout,
		wait: leftProcess.wait, close: leftProcess.closeIO,
	}
	right := duplexEndpoint{
		stdin: rightProcess.stdin, stdout: rightProcess.stdout,
		wait: rightProcess.wait, close: rightProcess.closeIO,
	}
	return runDuplexEndpoints(
		taskCtx, cancel, left, right,
		leftExecution.Limits.OutputBytes,
		rightExecution.Limits.OutputBytes,
		options.IdleTimeout,
		options.TranscriptBytes,
	)
}

func runDuplexEndpoints(
	ctx context.Context,
	cancel context.CancelFunc,
	left, right duplexEndpoint,
	leftLimit, rightLimit int64,
	idleTimeout time.Duration,
	transcriptLimit int64,
) (*DuplexResult, error) {
	if leftLimit <= 0 || rightLimit <= 0 {
		left.close()
		right.close()
		return nil, fmt.Errorf("duplex stream limits must be positive")
	}

	result := &DuplexResult{TerminationReason: TaskCompleted}
	var transcript *transcriptRecorder
	if transcriptLimit > 0 {
		transcript = &transcriptRecorder{remaining: transcriptLimit}
	}
	relayEvents := make(chan relayEvent, 2)
	waitEvents := make(chan waitEvent, 2)
	activity := make(chan struct{}, 1)
	go func() {
		count, err := relayStream(ctx, right.stdin, left.stdout, leftLimit, activity,
			func(data []byte) { transcript.record(DuplexLeft, data) })
		relayEvents <- relayEvent{side: DuplexLeft, bytes: count, err: err}
	}()
	go func() {
		count, err := relayStream(ctx, left.stdin, right.stdout, rightLimit, activity,
			func(data []byte) { transcript.record(DuplexRight, data) })
		relayEvents <- relayEvent{side: DuplexRight, bytes: count, err: err}
	}()
	go func() {
		value, err := left.wait()
		waitEvents <- waitEvent{side: DuplexLeft, result: value, err: err}
	}()
	go func() {
		value, err := right.wait()
		waitEvents <- waitEvent{side: DuplexRight, result: value, err: err}
	}()

	var closeOnce sync.Once
	closeAll := func() {
		closeOnce.Do(func() {
			cancel()
			left.close()
			right.close()
		})
	}
	defer closeAll()
	terminate := func(reason TaskTerminationReason, message string) {
		if result.TerminationReason == TaskCompleted {
			result.TerminationReason = reason
			result.TerminationMessage = message
		}
		closeAll()
	}

	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()
	contextDone := ctx.Done()
	relaysDone := 0
	waitsDone := 0
	var firstError error
	for relaysDone < 2 || waitsDone < 2 {
		select {
		case <-contextDone:
			contextDone = nil
			if result.TerminationReason == TaskCompleted {
				terminate(contextTaskReason(ctx.Err()), ctx.Err().Error())
			}
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)
		case <-timer.C:
			terminate(TaskIdleTimeout, "no protocol traffic before idle timeout")
		case event := <-relayEvents:
			relaysDone++
			if event.side == DuplexLeft {
				result.LeftToRightBytes = event.bytes
			} else {
				result.RightToLeftBytes = event.bytes
			}
			var limitErr *streamLimitError
			switch {
			case errors.As(event.err, &limitErr):
				result.LimitSide = event.side
				terminate(TaskOutputLimit, event.err.Error())
			case event.err != nil && !errors.Is(event.err, context.Canceled) &&
				!errors.Is(event.err, os.ErrClosed) &&
				!errors.Is(event.err, io.ErrClosedPipe) &&
				!errors.Is(event.err, syscall.EPIPE):
				if firstError == nil {
					firstError = event.err
				}
				terminate(TaskTransportError, event.err.Error())
			}
		case event := <-waitEvents:
			waitsDone++
			if event.side == DuplexLeft {
				result.Left = event.result
			} else {
				result.Right = event.result
			}
			if ctx.Err() != nil && result.TerminationReason == TaskCompleted {
				terminate(contextTaskReason(ctx.Err()), ctx.Err().Error())
			} else if event.err != nil {
				if firstError == nil {
					firstError = event.err
				}
				terminate(TaskNodeFailure, event.err.Error())
			} else if executionFailed(event.result) {
				message := "sandbox node returned no execution metadata"
				if event.result != nil && event.result.Meta != nil {
					message = event.result.Meta.ExitDescription()
				}
				terminate(TaskNodeFailure, message)
			}
		}
	}

	applyDuplexStats(result)
	result.Transcript = transcript.snapshot()
	return result, firstError
}

func contextTaskReason(err error) TaskTerminationReason {
	if errors.Is(err, context.DeadlineExceeded) {
		return TaskDeadline
	}
	return TaskCancelled
}

func executionFailed(result *Result) bool {
	if result == nil || result.Meta == nil {
		return true
	}
	return result.Meta.TerminationReason != TerminationExited || result.Meta.ExitCode != 0
}

func applyDuplexStats(result *DuplexResult) {
	if result.Left != nil && result.Left.Meta != nil {
		result.Left.Meta.StdoutStreamed = true
		result.Left.Meta.StdoutBytes = max(result.Left.Meta.StdoutBytes, result.LeftToRightBytes)
	}
	if result.Right != nil && result.Right.Meta != nil {
		result.Right.Meta.StdoutStreamed = true
		result.Right.Meta.StdoutBytes = max(result.Right.Meta.StdoutBytes, result.RightToLeftBytes)
	}
	if result.TerminationReason != TaskOutputLimit {
		return
	}
	var meta *Meta
	if result.LimitSide == DuplexLeft && result.Left != nil {
		meta = result.Left.Meta
	} else if result.LimitSide == DuplexRight && result.Right != nil {
		meta = result.Right.Meta
	}
	if meta != nil {
		meta.TerminationReason = TerminationOutputLimit
		meta.OutputLimit = true
		meta.Message = result.TerminationMessage
	}
}

func relayStream(
	ctx context.Context,
	destination io.WriteCloser,
	source io.ReadCloser,
	limit int64,
	activity chan<- struct{},
	record func([]byte),
) (int64, error) {
	defer destination.Close()
	defer source.Close()
	if limit <= 0 {
		return 0, fmt.Errorf("stream limit must be positive")
	}

	buffer := make([]byte, 32*1024)
	var total int64
	destinationOpen := true
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		remaining := limit - total
		if remaining <= 0 {
			return total, &streamLimitError{limit: limit}
		}
		readSize := len(buffer)
		if int64(readSize) > remaining {
			readSize = int(remaining)
		}
		count, readErr := source.Read(buffer[:readSize])
		if count > 0 {
			if record != nil {
				record(buffer[:count])
			}
			if destinationOpen {
				if err := writeAll(destination, buffer[:count]); err != nil {
					if errors.Is(err, syscall.EPIPE) || errors.Is(err, io.ErrClosedPipe) ||
						errors.Is(err, os.ErrClosed) {
						destinationOpen = false
					} else {
						return total, err
					}
				}
			}
			total += int64(count)
			select {
			case activity <- struct{}{}:
			default:
			}
			if total >= limit {
				return total, &streamLimitError{limit: limit}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

func writeAll(destination io.Writer, data []byte) error {
	for len(data) > 0 {
		count, err := destination.Write(data)
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
		data = data[count:]
	}
	return nil
}
