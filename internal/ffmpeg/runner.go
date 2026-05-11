// Package ffmpeg — Bu fayl FFmpeg subprocess'ni ishga tushirish va boshqarish.
//
// Bir Runner = bir FFmpeg processi = bir stream.
// Stream manager bir nechta Runner'ni boshqaradi (auto-restart, status hisoboti).
package ffmpeg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// State — Runner'ning hozirgi holati.
type State string

const (
	StateIdle     State = "idle"     // hali ishga tushirilmagan
	StateStarting State = "starting" // process ochilmoqda
	StateRunning  State = "running"  // ishlamoqda
	StateStopping State = "stopping" // to'xtatish so'raldi
	StateStopped  State = "stopped"  // normal yopildi
	StateError    State = "error"    // xatolik bilan to'xtadi
)

// LogLevel — FFmpeg log darajalari.
type LogLevel string

const (
	LogInfo    LogLevel = "info"
	LogWarning LogLevel = "warning"
	LogError   LogLevel = "error"
)

// LogLine — bitta log qatori.
type LogLine struct {
	Time    time.Time
	Level   LogLevel
	Message string
}

// Runner — bitta FFmpeg subprocess'ini boshqaradi.
//
// Lifecycle:
//   r := NewRunner("stream-1", "/path/to/ffmpeg.exe", args)
//   r.Subscribe(ch) // log'larni eshitish
//   err := r.Start(ctx)
//   ...
//   r.Stop()         // graceful
//   r.Wait()         // chiqishni kutish
type Runner struct {
	ID      string
	BinPath string
	Args    []string

	mu          sync.RWMutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser // graceful stop uchun 'q' yuboriladi
	state       State
	lastErr     string
	startedAt   time.Time
	stoppedAt   time.Time
	exitCode    int
	logBuf      []LogLine // oxirgi N qator (ring buffer kabi)
	logBufMax   int
	subscribers []chan LogLine
	done        chan struct{} // process chiqqanda yopiladi
}

// NewRunner — yangi Runner yaratadi (hali ishga tushirmaydi).
func NewRunner(id, binPath string, args []string) *Runner {
	return &Runner{
		ID:        id,
		BinPath:   binPath,
		Args:      args,
		state:     StateIdle,
		logBufMax: 500, // oxirgi 500 qator esda saqlanadi
		done:      make(chan struct{}),
	}
}

// State — joriy holatni qaytaradi.
func (r *Runner) State() State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

// LastError — agar process xato bilan to'xtagan bo'lsa, oxirgi xato xabari.
func (r *Runner) LastError() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastErr
}

// Uptime — process qancha vaqt ishlaganini qaytaradi.
func (r *Runner) Uptime() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.startedAt.IsZero() {
		return 0
	}
	if r.stoppedAt.IsZero() {
		return time.Since(r.startedAt)
	}
	return r.stoppedAt.Sub(r.startedAt)
}

// RecentLogs — oxirgi N qator log'ni qaytaradi (yangi obunachi UI uchun).
func (r *Runner) RecentLogs() []LogLine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LogLine, len(r.logBuf))
	copy(out, r.logBuf)
	return out
}

// Subscribe — log oqimini ko'rsatilgan kanalga yo'naltiradi.
// Unsubscribe — qaytarilgan funksiyani chaqiring.
//
// Kanal blocked bo'lsa, eski xabarlar tashlanadi (non-blocking).
func (r *Runner) Subscribe(ch chan LogLine) (unsubscribe func()) {
	r.mu.Lock()
	r.subscribers = append(r.subscribers, ch)
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, s := range r.subscribers {
			if s == ch {
				r.subscribers = append(r.subscribers[:i], r.subscribers[i+1:]...)
				return
			}
		}
	}
}

// Done — process chiqqanda yopiladigan kanal. Wait()'ga muqobil.
func (r *Runner) Done() <-chan struct{} {
	return r.done
}

// Start — FFmpeg processini ishga tushiradi.
// Process tugagunga qadar bloklamaydi — chiqishini Done() yoki Wait() bilan kuting.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.state == StateRunning || r.state == StateStarting {
		r.mu.Unlock()
		return fmt.Errorf("allaqachon ishlamoqda: %s", r.state)
	}
	r.state = StateStarting
	r.lastErr = ""
	r.exitCode = 0
	r.startedAt = time.Time{}
	r.stoppedAt = time.Time{}
	r.done = make(chan struct{})
	r.mu.Unlock()

	cmd := exec.CommandContext(ctx, r.BinPath, r.Args...)

	// Stdin'ga ulanamiz — keyinroq 'q' yuborish uchun (graceful stop)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.setError(fmt.Sprintf("stdin pipe yaratib bo'lmadi: %v", err))
		return err
	}

	// FFmpeg loglarini stderr'ga yozadi
	stderr, err := cmd.StderrPipe()
	if err != nil {
		r.setError(fmt.Sprintf("stderr pipe yaratib bo'lmadi: %v", err))
		return err
	}

	if err := cmd.Start(); err != nil {
		r.setError(fmt.Sprintf("process ishga tushmadi: %v", err))
		return err
	}

	r.mu.Lock()
	r.cmd = cmd
	r.stdin = stdin
	r.state = StateRunning
	r.startedAt = time.Now()
	r.mu.Unlock()

	// Log'larni o'qiy boshlaymiz
	go r.consumeLogs(stderr)

	// Process chiqishini kutamiz
	go r.waitExit(cmd, stdin)

	return nil
}

// consumeLogs — stderr'dan satrlarni o'qib, subscriber'larga yuboradi.
func (r *Runner) consumeLogs(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	// FFmpeg ba'zan uzun qatorlar yozadi
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		ll := LogLine{
			Time:    time.Now(),
			Level:   detectLevel(line),
			Message: line,
		}
		r.addLog(ll)
	}
}

// detectLevel — log qatoridan darajani aniqlaydi.
func detectLevel(line string) LogLevel {
	// FFmpeg ba'zi qatorlarda darajalarni shunday ifodalaydi:
	// [error] [warning] [fatal] yoki "Error:", "Warning:" so'zlari bilan
	lower := line
	if len(lower) > 200 {
		lower = lower[:200]
	}
	switch {
	case contains(lower, "error", "Error", "ERROR", "fatal", "Fatal"):
		return LogError
	case contains(lower, "warning", "Warning", "deprecated"):
		return LogWarning
	}
	return LogInfo
}

func contains(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

// addLog — yangi qatorni buffer'ga qo'shadi va subscriber'larga yuboradi.
func (r *Runner) addLog(ll LogLine) {
	r.mu.Lock()
	r.logBuf = append(r.logBuf, ll)
	if len(r.logBuf) > r.logBufMax {
		r.logBuf = r.logBuf[len(r.logBuf)-r.logBufMax:]
	}
	// Snapshot of subscribers (avoid holding the lock while sending)
	subs := make([]chan LogLine, len(r.subscribers))
	copy(subs, r.subscribers)
	r.mu.Unlock()

	for _, s := range subs {
		// Non-blocking — UI sekinroq bo'lsa, qatorlar tashlanadi
		select {
		case s <- ll:
		default:
		}
	}
}

// waitExit — process chiqishini kutadi va holatni yangilaydi.
func (r *Runner) waitExit(cmd *exec.Cmd, stdin io.Closer) {
	defer close(r.done)
	defer stdin.Close()

	err := cmd.Wait()

	r.mu.Lock()
	r.stoppedAt = time.Now()
	if cmd.ProcessState != nil {
		r.exitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		// Agar Stop() chaqirilgan bo'lsa va graceful chiqsa — OK
		if r.state == StateStopping {
			r.state = StateStopped
		} else {
			r.state = StateError
			r.lastErr = err.Error()
		}
	} else {
		r.state = StateStopped
	}
	r.mu.Unlock()
}

// Stop — FFmpeg'ga 'q' tugmasini "bosadi" (graceful), keyin majburiy.
// timeout o'tgandan keyin process hali ishlasa, kill qiladi.
func (r *Runner) Stop(timeout time.Duration) error {
	r.mu.Lock()
	if r.state != StateRunning && r.state != StateStarting {
		r.mu.Unlock()
		return errors.New("ishlamayapti")
	}
	r.state = StateStopping
	cmd := r.cmd
	stdin := r.stdin
	r.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return errors.New("process topilmadi")
	}

	// 1) Graceful: stdin'ga 'q'+\n yuborish — FFmpeg buni "exit" deb tushunadi
	if stdin != nil {
		_, _ = stdin.Write([]byte("q\n"))
	}

	// 2) Timeout: agar process chiqmasa, kill
	select {
	case <-r.done:
		return nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-r.done
		return errors.New("graceful timeout, kill qilindi")
	}
}

// Wait — process chiqishigacha bloklaydi.
func (r *Runner) Wait() {
	<-r.done
}

// setError — atomik state o'zgartirish (Start()'da xato bo'lsa).
func (r *Runner) setError(msg string) {
	r.mu.Lock()
	r.state = StateError
	r.lastErr = msg
	if r.done != nil {
		select {
		case <-r.done: // allaqachon yopilgan
		default:
			close(r.done)
		}
	}
	r.mu.Unlock()
}
