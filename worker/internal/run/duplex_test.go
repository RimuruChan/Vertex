package run

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type closeBuffer struct {
	bytes.Buffer
}

func (*closeBuffer) Close() error { return nil }

func completedEndpoint(output string) (duplexEndpoint, *closeBuffer) {
	input := &closeBuffer{}
	stdout := io.NopCloser(strings.NewReader(output))
	return duplexEndpoint{
		stdin:  input,
		stdout: stdout,
		wait: func() (*Result, error) {
			return &Result{Meta: &Meta{TerminationReason: TerminationExited}}, nil
		},
		close: func() {
			_ = input.Close()
			_ = stdout.Close()
		},
	}, input
}

func blockingEndpoint() duplexEndpoint {
	stdoutReader, stdoutWriter := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	done := make(chan struct{})
	var once sync.Once
	closeEndpoint := func() {
		once.Do(func() {
			_ = stdoutReader.Close()
			_ = stdoutWriter.Close()
			_ = stdinReader.Close()
			_ = stdinWriter.Close()
			close(done)
		})
	}
	return duplexEndpoint{
		stdin: stdinWriter, stdout: stdoutReader,
		wait: func() (*Result, error) {
			<-done
			return &Result{Meta: &Meta{TerminationReason: TerminationCancelled}}, nil
		},
		close: closeEndpoint,
	}
}

func TestRunDuplexEndpointsTransfersBothDirections(t *testing.T) {
	left, leftInput := completedEndpoint("left-output")
	right, rightInput := completedEndpoint("right-output")
	ctx, cancel := context.WithCancel(context.Background())
	result, err := runDuplexEndpoints(ctx, cancel, left, right, 1024, 1024, time.Second, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminationReason != TaskCompleted {
		t.Fatalf("termination = %q", result.TerminationReason)
	}
	if rightInput.String() != "left-output" || leftInput.String() != "right-output" {
		t.Fatalf("inputs = %q/%q", leftInput.String(), rightInput.String())
	}
	if result.LeftToRightBytes != 11 || result.RightToLeftBytes != 12 {
		t.Fatalf("bytes = %d/%d", result.LeftToRightBytes, result.RightToLeftBytes)
	}
	if string(result.Transcript.LeftToRight) != "left-output" ||
		string(result.Transcript.RightToLeft) != "right-output" || result.Transcript.Truncated {
		t.Fatalf("transcript = %+v", result.Transcript)
	}
	assertOrderedTranscript(t, result.Transcript)
}

func TestRunDuplexEndpointsEnforcesDirectionalOutputLimit(t *testing.T) {
	left, _ := completedEndpoint("12345")
	right, _ := completedEndpoint("")
	ctx, cancel := context.WithCancel(context.Background())
	result, err := runDuplexEndpoints(ctx, cancel, left, right, 4, 1024, time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminationReason != TaskOutputLimit || result.LimitSide != DuplexLeft {
		t.Fatalf("termination/side = %q/%q", result.TerminationReason, result.LimitSide)
	}
	if result.LeftToRightBytes != 4 || !result.Left.Meta.OutputLimit ||
		result.Left.Meta.TerminationReason != TerminationOutputLimit {
		t.Fatalf("left result = %+v, bytes=%d", result.Left.Meta, result.LeftToRightBytes)
	}
}

func TestRunDuplexEndpointsCancelsOnIdleTimeout(t *testing.T) {
	left := blockingEndpoint()
	right := blockingEndpoint()
	ctx, cancel := context.WithCancel(context.Background())
	result, err := runDuplexEndpoints(ctx, cancel, left, right, 1024, 1024, 20*time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminationReason != TaskIdleTimeout {
		t.Fatalf("termination = %q, want %q", result.TerminationReason, TaskIdleTimeout)
	}
}

func TestRunDuplexEndpointsCancelsPeerOnNodeFailure(t *testing.T) {
	left, _ := completedEndpoint("")
	left.wait = func() (*Result, error) {
		return &Result{Meta: &Meta{
			TerminationReason: TerminationSignal,
			Status:            "SG",
			ExitSignal:        11,
		}}, nil
	}
	right := blockingEndpoint()
	ctx, cancel := context.WithCancel(context.Background())
	result, err := runDuplexEndpoints(ctx, cancel, left, right, 1024, 1024, time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminationReason != TaskNodeFailure || result.Left.Meta.ExitSignal != 11 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunDuplexEndpointsReportsCallerCancellation(t *testing.T) {
	left := blockingEndpoint()
	right := blockingEndpoint()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runDuplexEndpoints(ctx, cancel, left, right, 1024, 1024, time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminationReason != TaskCancelled {
		t.Fatalf("termination = %q, want %q", result.TerminationReason, TaskCancelled)
	}
}

func TestRelayStreamDrainsAfterDestinationCloses(t *testing.T) {
	source := io.NopCloser(strings.NewReader("continue draining"))
	destinationReader, destinationWriter := io.Pipe()
	_ = destinationReader.Close()
	count, err := relayStream(context.Background(), destinationWriter, source, 1024, make(chan struct{}, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(len("continue draining")) {
		t.Fatalf("bytes = %d", count)
	}
}

func TestTranscriptRecorderUsesOneBoundedBudget(t *testing.T) {
	recorder := &transcriptRecorder{remaining: 5}
	recorder.record(DuplexLeft, []byte("abc"))
	recorder.record(DuplexRight, []byte("def"))
	value := recorder.snapshot()
	if string(value.LeftToRight) != "abc" || string(value.RightToLeft) != "de" || !value.Truncated {
		t.Fatalf("transcript = %+v", value)
	}
	if len(value.Events) != 2 || value.Events[0].Sequence != 1 || value.Events[0].Side != DuplexLeft ||
		string(value.Events[0].Data) != "abc" || value.Events[1].Sequence != 2 ||
		value.Events[1].Side != DuplexRight || string(value.Events[1].Data) != "de" {
		t.Fatalf("ordered events = %+v", value.Events)
	}
}

func assertOrderedTranscript(t *testing.T, transcript DuplexTranscript) {
	t.Helper()
	var left, right []byte
	for index, event := range transcript.Events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
		switch event.Side {
		case DuplexLeft:
			left = append(left, event.Data...)
		case DuplexRight:
			right = append(right, event.Data...)
		default:
			t.Fatalf("event %d side = %q", index, event.Side)
		}
	}
	if !bytes.Equal(left, transcript.LeftToRight) || !bytes.Equal(right, transcript.RightToLeft) {
		t.Fatalf("events do not reconstruct directional transcript: %+v", transcript)
	}
}

func TestRunDuplexEndpointsDistinguishesDeadline(t *testing.T) {
	left := blockingEndpoint()
	right := blockingEndpoint()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, err := runDuplexEndpoints(ctx, cancel, left, right, 1024, 1024, time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminationReason != TaskDeadline {
		t.Fatalf("termination = %q, want %q", result.TerminationReason, TaskDeadline)
	}
}
