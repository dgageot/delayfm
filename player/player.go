package player

import (
	"cmp"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
)

var (
	speakerOnce     sync.Once
	speakerInitRate beep.SampleRate
)

type Player struct {
	mu           sync.Mutex
	done         chan struct{}
	ctrl         *beep.Ctrl
	delay        *delayStreamer
	station      string
	initialDelay time.Duration
}

func New(initialDelay time.Duration) *Player {
	return &Player{
		initialDelay: cmp.Or(initialDelay, defaultInitialDelay),
	}
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ctrl != nil
}

func (p *Player) Station() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.station
}

func (p *Player) Delay() time.Duration {
	p.mu.Lock()
	ds := p.delay
	p.mu.Unlock()

	if ds == nil {
		return 0
	}
	return ds.Delay()
}

func (p *Player) Level() float64 {
	p.mu.Lock()
	ds := p.delay
	p.mu.Unlock()

	if ds == nil {
		return 0
	}
	return ds.Level()
}

func (p *Player) SetDelay(d time.Duration) {
	p.mu.Lock()
	ds := p.delay
	p.mu.Unlock()

	if ds == nil {
		return
	}
	ds.SetDelay(d)
}

func (p *Player) AdjustDelay(delta time.Duration) {
	p.mu.Lock()
	ds := p.delay
	p.mu.Unlock()

	if ds == nil {
		return
	}
	ds.SetDelay(ds.Delay() + delta)
}

// Play streams from the given URL. It stops any current playback first.
func (p *Player) Play(stationURL, stationName string) error {
	p.Stop()

	streamer, format, err := p.openStream(stationURL)
	if err != nil {
		return err
	}

	speakerOnce.Do(func() {
		speakerInitRate = format.SampleRate
		err = speaker.Init(format.SampleRate, format.SampleRate.N(200*time.Millisecond))
	})
	if err != nil {
		streamer.Close()
		return fmt.Errorf("init speaker: %w", err)
	}

	source := p.resample(streamer, format)

	reconnect := func() (beep.Streamer, error) {
		s, f, err := p.openStream(stationURL)
		if err != nil {
			return nil, err
		}
		return p.resample(s, f), nil
	}

	ds := newDelayStreamer(source, speakerInitRate, p.initialDelay, reconnect)
	done := make(chan struct{})
	ctrl := &beep.Ctrl{Streamer: ds}

	p.mu.Lock()
	p.done = done
	p.ctrl = ctrl
	p.delay = ds
	p.station = stationName
	p.mu.Unlock()

	speaker.Play(beep.Seq(ctrl, beep.Callback(func() {
		ds.Close()
		p.reset(done)
	})))

	return nil
}

func (p *Player) openStream(url string) (beep.StreamSeekCloser, beep.Format, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("connect to stream: %w", err)
	}

	streamer, format, err := mp3.Decode(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, beep.Format{}, fmt.Errorf("decode mp3 stream: %w", err)
	}

	return streamer, format, nil
}

func (p *Player) resample(streamer beep.StreamSeekCloser, format beep.Format) beep.Streamer {
	if format.SampleRate != speakerInitRate {
		return beep.Resample(4, format.SampleRate, speakerInitRate, streamer)
	}
	return streamer
}

func (p *Player) Stop() {
	p.mu.Lock()
	ctrl := p.ctrl
	done := p.done
	p.mu.Unlock()

	if ctrl == nil {
		return
	}

	speaker.Lock()
	ctrl.Streamer = nil
	speaker.Unlock()

	if done != nil {
		<-done
	}
}

func (p *Player) reset(done chan struct{}) {
	close(done)

	p.mu.Lock()
	p.ctrl = nil
	p.done = nil
	p.delay = nil
	p.station = ""
	p.mu.Unlock()
}
