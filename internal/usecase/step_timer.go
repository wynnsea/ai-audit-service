package usecase

import (
	"fmt"
	"strings"
	"time"
)

// stepTimer mencatat durasi tiap tahap secara berurutan, lalu dicetak jadi satu baris log
type stepTimer struct {
	start, last time.Time
	steps       []string
}

func newStepTimer() *stepTimer {
	now := time.Now()
	return &stepTimer{start: now, last: now}
}

// lap mencatat waktu sejak lap sebelumnya dengan nama tahap tersebut
func (t *stepTimer) lap(name string) {
	now := time.Now()
	t.steps = append(t.steps, fmt.Sprintf("%s=%s", name, now.Sub(t.last).Round(time.Millisecond)))
	t.last = now
}

func (t *stepTimer) String() string {
	return fmt.Sprintf("total=%s %s", time.Since(t.start).Round(time.Millisecond), strings.Join(t.steps, " "))
}