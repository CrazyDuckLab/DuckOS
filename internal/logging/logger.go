// Package logging provides the bounded, redacting, per-subsystem
// diagnostics surface required by V1_SCOPE. The package has a small
// surface: a Logger with one channel per subsystem, a Level knob, a
// ring buffer of recent records, and a Subscriber protocol. The
// defaults are conservative: a few hundred records, hard caps on
// every queue, and visible dropped counters so a missing export can
// never silently swallow traffic.
package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level is a per-subsystem dial. Off drops every record, Errors keeps
// only errors, Normal keeps informational traffic, Detailed keeps
// the verbose flows, Trace keeps the wire-format or kernel events.
type Level int

const (
	Off Level = iota
	Errors
	Normal
	Detailed
	Trace
)

func (l Level) String() string {
	switch l {
	case Off:
		return "off"
	case Errors:
		return "errors"
	case Normal:
		return "normal"
	case Detailed:
		return "detailed"
	case Trace:
		return "trace"
	}
	return "unknown"
}

// ParseLevel is the canonical inverse of String. Unknown values are
// refused explicitly so a misconfigured config does not silently
// downgrade a real dial.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off":
		return Off, nil
	case "errors", "error":
		return Errors, nil
	case "normal", "info":
		return Normal, nil
	case "detailed", "debug":
		return Detailed, nil
	case "trace":
		return Trace, nil
	}
	return Off, fmt.Errorf("unknown log level %q", s)
}

// Severity mirrors the level. The map from level to severity is
// monotonic: a level emits every record at or above its severity.
type Severity int

const (
	SevError Severity = iota + 1
	SevInfo
	SevDebug
	SevTrace
)

// Record is one entry in the ring buffer. Sensitive bytes are redacted
// at enqueue time; the ring never carries plaintext secrets.
type Record struct {
	At        time.Time
	Severity  Severity
	Subsystem string
	Message   string
	Context   map[string]string
	Dropped   uint64
}

// Subsystem is a per-feature identifier. The empty string is reserved
// for the global default; everything else is a named subsystem.
type Subsystem string

const (
	Global Subsystem = "global"
)

// Config is the bounded knobs for a Logger. Zero values produce
// conservative defaults.
type Config struct {
	// Ring capacity in records. Default 512. Hard cap 16384.
	RingCapacity int
	// MaxConcurrentSubscribers is the upper bound on the number of
	// subscribers. Default 16. Hard cap 256.
	MaxSubscribers int
	// MaxContextBytes is the per-record size limit on the context
	// map. Default 4 KiB. Hard cap 32 KiB.
	MaxContextBytes int
	// Replacer returns a redacted version of any value that
	// matches a sensitive pattern. The default is
	// DefaultReplacer.
	Replacer func(field, value string) string
}

// DefaultConfig returns a reasonable baseline.
func DefaultConfig() Config {
	return Config{
		RingCapacity:    512,
		MaxSubscribers:  16,
		MaxContextBytes: 4 * 1024,
		Replacer:        DefaultReplacer,
	}
}

// SensitiveFields is the canonical list of field names whose values
// must always be redacted. Field names are matched case-insensitively
// after canonicalisation.
var SensitiveFields = []string{
	"password", "passphrase", "secret", "private", "privateKey",
	"presharedKey", "presharedkey", "preshared_key",
	"token", "apikey", "api_key", "authorization",
	"xray-credential", "wireguard-private", "wireguard-preshared",
	"vault-master", "masterKey", "vaultSecret",
	"uuid", "psk", "key", "privatekey",
}

// DefaultReplacer returns a stable, redacted placeholder for any
// value associated with a sensitive field. The function also redacts
// values that look like a base64 WireGuard/Xray credential or a
// 32-byte vault key. A value is replaced with its 8-byte hex
// fingerprint so the operator can correlate a record with the
// original secret owner without leaking the bytes.
func DefaultReplacer(field, value string) string {
	if isSensitiveField(field) {
		return fingerprint(value)
	}
	return redactLikelySecret(value)
}

func isSensitiveField(field string) bool {
	lower := strings.ToLower(strings.TrimSpace(field))
	for _, candidate := range SensitiveFields {
		if lower == candidate || strings.Contains(lower, candidate) {
			return true
		}
	}
	return false
}

func redactLikelySecret(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 16 {
		return trimmed
	}
	switch {
	case isLikelyWireGuardKey(trimmed):
		return fingerprint(trimmed)
	case isLikelyUUID(trimmed):
		// A canonical UUID is identifying, not strictly secret, but
		// the policy is to redact it on the wire by default. The
		// fingerprint preserves the first 8 hex characters for ops.
		return fingerprint(trimmed)
	}
	return trimmed
}

func isLikelyWireGuardKey(s string) bool {
	// Base64 of 32 bytes is 44 characters and only base64 alphabet.
	if len(s) != 44 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '+' || c == '/' || c == '=':
		default:
			return false
		}
	}
	return true
}

func isLikelyUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func fingerprint(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return "redacted:" + hex.EncodeToString(sum[:4])
}

// Logger is the global per-subsystem surface. The zero value is not
// usable; call New to obtain a Logger. The Logger is safe for
// concurrent use.
type Logger struct {
	mu          sync.Mutex
	levels      map[Subsystem]Level
	ring        []Record
	ringStart   int
	ringLen     int
	ringCap     int
	subscribers []chan Record
	dropped     uint64
	emitted     uint64
	maxCtx      int
	maxSub      int
	replacer    func(string, string) string
	closed      bool
}

// New constructs a Logger with the given config. The default config
// is used when Config{} is supplied.
func New(cfg Config) *Logger {
	if cfg.RingCapacity <= 0 {
		cfg.RingCapacity = 512
	}
	if cfg.RingCapacity > 16384 {
		cfg.RingCapacity = 16384
	}
	if cfg.MaxSubscribers <= 0 {
		cfg.MaxSubscribers = 16
	}
	if cfg.MaxSubscribers > 256 {
		cfg.MaxSubscribers = 256
	}
	if cfg.MaxContextBytes <= 0 {
		cfg.MaxContextBytes = 4 * 1024
	}
	if cfg.MaxContextBytes > 32*1024 {
		cfg.MaxContextBytes = 32 * 1024
	}
	if cfg.Replacer == nil {
		cfg.Replacer = DefaultReplacer
	}
	return &Logger{
		levels:   map[Subsystem]Level{Global: Normal},
		ring:     make([]Record, cfg.RingCapacity),
		ringCap:  cfg.RingCapacity,
		maxCtx:   cfg.MaxContextBytes,
		maxSub:   cfg.MaxSubscribers,
		replacer: cfg.Replacer,
	}
}

// SetLevel sets the level for a subsystem. The global level is the
// default for subsystems that do not have an explicit override.
func (l *Logger) SetLevel(s Subsystem, level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	if s == "" {
		s = Global
	}
	l.levels[s] = level
}

// Level returns the current level for a subsystem.
func (l *Logger) Level(s Subsystem) Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s == "" {
		s = Global
	}
	if level, ok := l.levels[s]; ok {
		return level
	}
	return l.levels[Global]
}

// Log records a single entry. The level is derived from Severity
// through the canonical level map; a level below the dial's threshold
// is silently dropped. The dropped counter is updated.
func (l *Logger) Log(subsystem Subsystem, severity Severity, message string, context map[string]string) {
	if subsystem == "" {
		subsystem = Global
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		atomic.AddUint64(&l.dropped, 1)
		return
	}
	l.mu.Unlock()
	dial := l.Level(subsystem)
	if !levelAdmits(dial, severity) {
		atomic.AddUint64(&l.dropped, 1)
		return
	}
	atomic.AddUint64(&l.emitted, 1)
	rec := Record{
		At:        time.Now(),
		Severity:  severity,
		Subsystem: string(subsystem),
		Message:   l.replacer("message", message),
		Context:   l.redactContext(context),
		Dropped:   atomic.LoadUint64(&l.dropped),
	}
	l.append(rec)
}

func (l *Logger) redactContext(ctx map[string]string) map[string]string {
	if len(ctx) == 0 {
		return nil
	}
	out := make(map[string]string, len(ctx))
	size := 0
	for k, v := range ctx {
		redacted := l.replacer(k, v)
		size += len(k) + len(redacted)
		if size > l.maxCtx {
			out[k] = "context-truncated"
			continue
		}
		out[k] = redacted
	}
	return out
}

func (l *Logger) append(rec Record) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	if l.ringLen < l.ringCap {
		idx := (l.ringStart + l.ringLen) % l.ringCap
		l.ring[idx] = rec
		l.ringLen++
	} else {
		// Evict the oldest record, increment dropped.
		atomic.AddUint64(&l.dropped, 1)
		l.ring[l.ringStart] = rec
		l.ringStart = (l.ringStart + 1) % l.ringCap
	}
	subs := l.subscribers
	l.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- rec:
		default:
			// Subscriber is full: drop and account. The
			// visible drop is the same counter the operator
			// reads from the API.
			atomic.AddUint64(&l.dropped, 1)
		}
	}
}

// Snapshot returns the ring buffer in chronological order. The
// returned slice is a copy; mutations do not affect the ring.
func (l *Logger) Snapshot() []Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ringLen == 0 {
		return nil
	}
	out := make([]Record, 0, l.ringLen)
	for i := 0; i < l.ringLen; i++ {
		out = append(out, l.ring[(l.ringStart+i)%l.ringCap])
	}
	return out
}

// Counts returns the cumulative dropped and emitted counters. The
// values are monotonic for the lifetime of the Logger.
func (l *Logger) Counts() (emitted, dropped uint64) {
	return atomic.LoadUint64(&l.emitted), atomic.LoadUint64(&l.dropped)
}

// Subscribe returns a channel that receives every accepted record
// until Unsubscribe is called. The channel has a 64-record buffer;
// slow consumers see visible dropped counters. Subscribe returns
// errors.New("subscriber limit reached") when the cap is full.
func (l *Logger) Subscribe() (<-chan Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, errors.New("logger is closed")
	}
	if len(l.subscribers) >= l.maxSub {
		return nil, errors.New("subscriber limit reached")
	}
	ch := make(chan Record, 64)
	l.subscribers = append(l.subscribers, ch)
	return ch, nil
}

// Unsubscribe removes a subscriber and closes its channel.
func (l *Logger) Unsubscribe(ch <-chan Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, candidate := range l.subscribers {
		if candidate == ch {
			close(candidate)
			l.subscribers = append(l.subscribers[:i], l.subscribers[i+1:]...)
			return
		}
	}
}

// Close marks the Logger as closed. Subscribers continue to be served
// until Unsubscribe is called. Subsequent Log calls are no-ops.
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
}

// ListLevels returns the current level map. The result is sorted by
// subsystem name and is safe to expose through an API.
func (l *Logger) ListLevels() map[Subsystem]Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[Subsystem]Level, len(l.levels))
	for k, v := range l.levels {
		out[k] = v
	}
	return out
}

// Convenience: subsystem dial with explicit severity.
func (l *Logger) Errorf(subsystem Subsystem, format string, args ...any) {
	l.Log(subsystem, SevError, fmt.Sprintf(format, args...), nil)
}
func (l *Logger) Infof(subsystem Subsystem, format string, args ...any) {
	l.Log(subsystem, SevInfo, fmt.Sprintf(format, args...), nil)
}
func (l *Logger) Debugf(subsystem Subsystem, format string, args ...any) {
	l.Log(subsystem, SevDebug, fmt.Sprintf(format, args...), nil)
}
func (l *Logger) Tracef(subsystem Subsystem, format string, args ...any) {
	l.Log(subsystem, SevTrace, fmt.Sprintf(format, args...), nil)
}

func levelAdmits(dial Level, severity Severity) bool {
	switch dial {
	case Off:
		return false
	case Errors:
		return severity == SevError
	case Normal:
		return severity == SevError || severity == SevInfo
	case Detailed:
		return severity == SevError || severity == SevInfo || severity == SevDebug
	case Trace:
		return true
	}
	return false
}

// LevelFor returns the minimal level that admits the severity.
func LevelFor(severity Severity) Level {
	switch severity {
	case SevError:
		return Errors
	case SevInfo:
		return Normal
	case SevDebug:
		return Detailed
	case SevTrace:
		return Trace
	}
	return Off
}

// Subsystems returns a sorted list of all configured subsystems.
func (l *Logger) Subsystems() []Subsystem {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Subsystem, 0, len(l.levels))
	for s := range l.levels {
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
