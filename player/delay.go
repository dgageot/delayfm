package player

import (
	"math"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
)

const (
	maxDelay            = 30 * time.Second
	defaultInitialDelay = 5 * time.Second
)

// delayStreamer wraps a source streamer with a ring buffer that allows
// playing audio with an adjustable delay (up to maxDelay).
//
// A background goroutine reads from the source into the buffer.
// Stream() reads from a position that is `delay` samples behind
// the write position.
//
// On startup, it buffers initialDelay worth of audio before playing,
// to absorb network jitter from the HTTP stream.
//
// If the source stream drops, the fill goroutine calls reconnect to
// obtain a new source, splicing it into the ring buffer at the current
// write position. The delayed reader keeps playing from the buffer,
// making short outages seamless.
type delayStreamer struct {
	mu           sync.Mutex
	ring         [][2]float64
	written      int64 // total samples written (monotonic)
	read         int64 // total samples read (monotonic)
	initialDelay int   // samples to buffer before starting
	started      bool  // true once initial buffer is full
	rate         beep.SampleRate
	source       beep.Streamer
	reconnect    func() (beep.Streamer, error) // nil = no reconnect
	eof          bool
	closed       bool
	level        float64 // peak audio level from last Stream() call (0.0–1.0)

	// Reconnect parameters.
	reconnectMaxRetries int
	reconnectBaseDelay  time.Duration
	reconnectMaxDelay   time.Duration
}

func newDelayStreamer(source beep.Streamer, rate beep.SampleRate, initialDelay time.Duration, reconnect func() (beep.Streamer, error)) *delayStreamer {
	ds := &delayStreamer{
		ring:                make([][2]float64, rate.N(maxDelay+5*time.Second)),
		rate:                rate,
		source:              source,
		reconnect:           reconnect,
		initialDelay:        rate.N(initialDelay),
		reconnectMaxRetries: 10,
		reconnectBaseDelay:  500 * time.Millisecond,
		reconnectMaxDelay:   10 * time.Second,
	}
	go ds.fill()
	return ds
}

// fill continuously reads from the source into the ring buffer.
// On source EOF, it attempts to reconnect and splice the new stream.
func (ds *delayStreamer) fill() {
	buf := make([][2]float64, 4096)
	for {
		n, ok := ds.source.Stream(buf)

		ds.mu.Lock()
		if ds.closed {
			ds.mu.Unlock()
			return
		}
		for i := range n {
			ds.ring[int(ds.written)%len(ds.ring)] = buf[i]
			ds.written++
		}
		ds.mu.Unlock()

		if !ok {
			if ds.tryReconnect() {
				continue
			}

			ds.mu.Lock()
			ds.eof = true
			ds.mu.Unlock()
			return
		}
	}
}

// tryReconnect attempts to reconnect to the source stream with exponential backoff.
// Returns true if reconnection succeeded and the fill loop should continue.
func (ds *delayStreamer) tryReconnect() bool {
	ds.mu.Lock()
	reconnect := ds.reconnect
	ds.mu.Unlock()

	if reconnect == nil {
		return false
	}

	delay := ds.reconnectBaseDelay
	for range ds.reconnectMaxRetries {
		ds.mu.Lock()
		closed := ds.closed
		ds.mu.Unlock()
		if closed {
			return false
		}

		time.Sleep(delay)
		delay = min(delay*2, ds.reconnectMaxDelay)

		newSource, err := reconnect()
		if err != nil {
			continue
		}

		ds.mu.Lock()
		if ds.closed {
			ds.mu.Unlock()
			return false
		}
		ds.source = newSource
		ds.mu.Unlock()
		return true
	}

	return false
}

// Stream fills samples from the delayed position in the ring buffer.
func (ds *delayStreamer) Stream(samples [][2]float64) (int, bool) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Wait for the initial buffer to fill before playing.
	if !ds.started {
		if ds.written < int64(ds.initialDelay) && !ds.eof {
			clear(samples)
			return len(samples), true
		}
		ds.started = true
		ds.read = max(ds.written-int64(ds.initialDelay), 0)
	}

	var peak float64

	for i := range samples {
		if ds.read >= ds.written {
			if ds.eof {
				ds.level = peak
				return i, false
			}
			// Buffer underrun: output silence without advancing read.
			// When fill() catches up, we'll resume from where we left off.
			clear(samples[i:])
			ds.level = peak
			return len(samples), true
		}

		// If read fell too far behind, skip ahead.
		if ds.written-ds.read > int64(len(ds.ring)) {
			ds.read = ds.written - int64(len(ds.ring)) + 1
		}

		samples[i] = ds.ring[int(ds.read)%len(ds.ring)]
		ds.read++

		peak = max(peak, math.Abs(samples[i][0]), math.Abs(samples[i][1]))
	}

	ds.level = peak
	return len(samples), true
}

func (ds *delayStreamer) Err() error { return nil }

// SetDelay adjusts the read position to be `d` behind the write position.
func (ds *delayStreamer) SetDelay(d time.Duration) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	d = min(max(d, 0), maxDelay)
	ds.read = min(max(ds.written-int64(ds.rate.N(d)), 0), ds.written)
}

// Delay returns the current delay.
func (ds *delayStreamer) Delay() time.Duration {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	samples := ds.written - ds.read
	if samples <= 0 {
		return 0
	}
	return ds.rate.D(int(samples))
}

// Level returns the peak audio level from the last Stream() call (0.0–1.0).
func (ds *delayStreamer) Level() float64 {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return min(ds.level, 1.0)
}

func (ds *delayStreamer) Close() {
	ds.mu.Lock()
	ds.closed = true
	ds.mu.Unlock()
}
