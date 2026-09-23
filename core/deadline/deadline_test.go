package deadline_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/core/deadline"
)

// The timer is driven by a clock, so the clock is controlled: these state when a
// wake-up arrives, not how promptly a loaded machine manages to deliver one.

func armed(t *testing.T, timer *deadline.Timer) bool {
	t.Helper()
	synctest.Wait()
	select {
	case <-timer.Channel():
		return true
	default:
		return false
	}
}

func TestANewTimerIsStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		timer := deadline.NewTimer()
		defer timer.Stop()
		time.Sleep(time.Hour)
		if armed(t, timer) {
			t.Fatal("a timer nobody scheduled woke its driver")
		}
	})
}

func TestAScheduledDeadlineWakesTheDriver(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		timer := deadline.NewTimer()
		defer timer.Stop()
		timer.Schedule(time.Now().Add(time.Second), true)
		if armed(t, timer) {
			t.Fatal("the deadline arrived before its time")
		}
		time.Sleep(2 * time.Second)
		if !armed(t, timer) {
			t.Fatal("an armed deadline never arrived")
		}
	})
}

func TestADeadlineAlreadyPastWakesTheDriverAtOnce(t *testing.T) {
	// The one correctness decision in here: arming for a negative duration is not a
	// thing a timer does, and the driver still has to be told.
	synctest.Test(t, func(t *testing.T) {
		timer := deadline.NewTimer()
		defer timer.Stop()
		timer.Schedule(time.Now().Add(-time.Hour), true)
		if !armed(t, timer) {
			t.Fatal("a deadline already past never arrived")
		}
	})
}

func TestNoDeadlineDisarmsAnArmedTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		timer := deadline.NewTimer()
		defer timer.Stop()
		timer.Schedule(time.Now().Add(time.Second), true)
		timer.Schedule(time.Time{}, false)
		time.Sleep(2 * time.Second)
		if armed(t, timer) {
			t.Fatal("a disarmed deadline still woke its driver")
		}
	})
}

func TestReschedulingDiscardsTheDeadlineItReplaces(t *testing.T) {
	// The driver schedules on every turn, so only the newest answer may arrive: a
	// stale wake-up would settle something that is no longer waiting.
	synctest.Test(t, func(t *testing.T) {
		timer := deadline.NewTimer()
		defer timer.Stop()
		timer.Schedule(time.Now().Add(time.Second), true)
		time.Sleep(2 * time.Second)
		timer.Schedule(time.Now().Add(time.Second), true)
		if armed(t, timer) {
			t.Fatal("rescheduling exposed the expiration it replaced")
		}
		time.Sleep(2 * time.Second)
		timer.Schedule(time.Time{}, false)
		if armed(t, timer) {
			t.Fatal("disarming exposed the expiration it discarded")
		}
		timer.Schedule(time.Now(), true)
		if !armed(t, timer) {
			t.Fatal("a timer reused after two discards never armed again")
		}
	})
}

func TestStoppingTwiceIsAllowed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		timer := deadline.NewTimer()
		timer.Schedule(time.Now().Add(time.Second), true)
		timer.Stop()
		timer.Stop()
		time.Sleep(2 * time.Second)
		if armed(t, timer) {
			t.Fatal("a stopped timer woke its driver")
		}
	})
}

func TestADeliveredWakeUpDoesNotArriveTwice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		timer := deadline.NewTimer()
		defer timer.Stop()
		timer.Schedule(time.Now(), true)
		if !armed(t, timer) {
			t.Fatal("the deadline never arrived")
		}
		time.Sleep(time.Hour)
		if armed(t, timer) {
			t.Fatal("one deadline woke the driver twice")
		}
	})
}
