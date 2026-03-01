package player

import (
	"fmt"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRate = beep.SampleRate(44100)

// fakeStreamer produces a known sequence of samples: each sample's left
// channel is its 0-based index (as float64) and the right channel is 0.
type fakeStreamer struct {
	pos   int
	total int
}

func (f *fakeStreamer) Stream(samples [][2]float64) (int, bool) {
	if f.pos >= f.total {
		return 0, false
	}
	n := min(len(samples), f.total-f.pos)
	for i := range n {
		samples[i] = [2]float64{float64(f.pos + i), 0}
	}
	f.pos += n
	return n, true
}

func (f *fakeStreamer) Err() error { return nil }

func newTestDelay(totalSamples int) *delayStreamer {
	src := &fakeStreamer{total: totalSamples}
	return newDelayStreamer(src, testRate, defaultInitialDelay, nil)
}

// waitForBuffered waits until the delayStreamer has written at least n samples.
func waitForBuffered(t *testing.T, ds *delayStreamer, n int64) {
	t.Helper()
	require.Eventually(t, func() bool {
		ds.mu.Lock()
		defer ds.mu.Unlock()
		return ds.written >= n
	}, 2*time.Second, time.Millisecond)
}

func TestDelayStreamer_InitialDelay(t *testing.T) {
	ds := newTestDelay(testRate.N(10 * time.Second))
	defer ds.Close()

	waitForBuffered(t, ds, int64(ds.initialDelay))

	// The delay is at least initialDelay because the fill goroutine
	// may have written more samples since we checked.
	d := ds.Delay()
	assert.GreaterOrEqual(t, d, defaultInitialDelay)
	assert.LessOrEqual(t, d, maxDelay)
}

func TestDelayStreamer_SetDelay(t *testing.T) {
	ds := newTestDelay(testRate.N(10 * time.Second))
	defer ds.Close()

	waitForBuffered(t, ds, int64(testRate.N(8*time.Second)))

	ds.SetDelay(3 * time.Second)
	d := ds.Delay()
	assert.InDelta(t, 3.0, d.Seconds(), 0.1)
}

func TestDelayStreamer_SetDelayClampedToMax(t *testing.T) {
	ds := newTestDelay(testRate.N(10 * time.Second))
	defer ds.Close()

	waitForBuffered(t, ds, int64(testRate.N(5*time.Second)))

	ds.SetDelay(999 * time.Second)
	d := ds.Delay()
	assert.LessOrEqual(t, d, maxDelay)
}

func TestDelayStreamer_SetDelayClampedToZero(t *testing.T) {
	ds := newTestDelay(testRate.N(10 * time.Second))
	defer ds.Close()

	waitForBuffered(t, ds, int64(testRate.N(5*time.Second)))

	ds.SetDelay(-5 * time.Second)
	assert.Equal(t, time.Duration(0), ds.Delay())
}

func TestDelayStreamer_StreamAfterEOF(t *testing.T) {
	totalSamples := testRate.N(1 * time.Second)
	ds := newTestDelay(totalSamples)
	defer ds.Close()

	waitForBuffered(t, ds, int64(totalSamples))

	// Drain all samples.
	buf := make([][2]float64, 4096)
	var total int
	for {
		n, ok := ds.Stream(buf)
		total += n
		if !ok {
			break
		}
	}

	assert.Positive(t, total)
}

func TestDelayStreamer_ErrReturnsNil(t *testing.T) {
	ds := newTestDelay(1000)
	defer ds.Close()

	assert.NoError(t, ds.Err())
}

func TestDelayStreamer_DelayIsZeroBeforeStart(t *testing.T) {
	src := &fakeStreamer{total: 0}
	ds := &delayStreamer{
		ring:         make([][2]float64, testRate.N(maxDelay+5*time.Second)),
		rate:         testRate,
		source:       src,
		initialDelay: testRate.N(defaultInitialDelay),
	}
	// Don't start the fill goroutine; written=0, read=0.
	assert.Equal(t, time.Duration(0), ds.Delay())
}

func TestDelayStreamer_ReconnectOnDrop(t *testing.T) {
	firstBatch := testRate.N(1 * time.Second)
	secondBatch := testRate.N(2 * time.Second)

	reconnect := func() (beep.Streamer, error) {
		return &fakeStreamer{total: secondBatch}, nil
	}

	ds := &delayStreamer{
		ring:                make([][2]float64, testRate.N(maxDelay+5*time.Second)),
		rate:                testRate,
		source:              &fakeStreamer{total: firstBatch},
		reconnect:           reconnect,
		initialDelay:        testRate.N(100 * time.Millisecond),
		reconnectMaxRetries: 3,
		reconnectBaseDelay:  time.Millisecond,
		reconnectMaxDelay:   time.Millisecond,
	}
	go ds.fill()
	defer ds.Close()

	expected := int64(firstBatch + secondBatch)
	waitForBuffered(t, ds, expected)

	ds.mu.Lock()
	w := ds.written
	eof := ds.eof
	ds.mu.Unlock()

	assert.GreaterOrEqual(t, w, expected)
	assert.False(t, eof)
}

func TestDelayStreamer_ReconnectFailsSetEOF(t *testing.T) {
	reconnect := func() (beep.Streamer, error) {
		return nil, fmt.Errorf("connection refused")
	}

	ds := &delayStreamer{
		ring:                make([][2]float64, testRate.N(maxDelay+5*time.Second)),
		rate:                testRate,
		source:              &fakeStreamer{total: testRate.N(100 * time.Millisecond)},
		reconnect:           reconnect,
		initialDelay:        testRate.N(50 * time.Millisecond),
		reconnectMaxRetries: 3,
		reconnectBaseDelay:  time.Millisecond,
		reconnectMaxDelay:   time.Millisecond,
	}
	go ds.fill()
	defer ds.Close()

	require.Eventually(t, func() bool {
		ds.mu.Lock()
		defer ds.mu.Unlock()
		return ds.eof
	}, 5*time.Second, time.Millisecond)
}
