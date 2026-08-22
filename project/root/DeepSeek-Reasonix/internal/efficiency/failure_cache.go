package efficiency

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FailureRecord stores only a compact proven repair hint. It is intentionally
// append-only; malformed/corrupt records are skipped during recovery rather
// than making the whole history unreadable.
type FailureRecord struct {
	Fingerprint string   `json:"fingerprint"`
	Environment string   `json:"environment,omitempty"`
	Files       []string `json:"files,omitempty"`
	FixHint     string   `json:"fixHint"`
	Verified    bool     `json:"verified"`
	SeenUnixMS  int64    `json:"seenUnixMs"`
}

type failureEnvelope struct {
	Payload json.RawMessage `json:"payload"`
	CRC32   string          `json:"crc32"`
}

type FailureCacheStats struct {
	Records        int `json:"records"`
	CorruptSkipped int `json:"corruptSkipped"`
}

type FailureCache struct {
	mu      sync.Mutex
	path    string
	records map[string][]FailureRecord
	corrupt int
}

func OpenFailureCache(path string) (*FailureCache, error) {
	c := &FailureCache{path: path, records: make(map[string][]FailureRecord)}
	if strings.TrimSpace(path) == "" {
		return c, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 1024*1024)
	for s.Scan() {
		var env failureEnvelope
		if json.Unmarshal(s.Bytes(), &env) != nil || !validCRC(env) {
			c.corrupt++
			continue
		}
		var rec FailureRecord
		if json.Unmarshal(env.Payload, &rec) != nil || strings.TrimSpace(rec.Fingerprint) == "" {
			c.corrupt++
			continue
		}
		c.records[rec.Fingerprint] = append(c.records[rec.Fingerprint], rec)
	}
	return c, s.Err()
}

func (c *FailureCache) PutVerified(rec FailureRecord) error {
	if c == nil || !rec.Verified || strings.TrimSpace(rec.Fingerprint) == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if rec.SeenUnixMS == 0 {
		rec.SeenUnixMS = time.Now().UnixMilli()
	}
	rec.Files = normalizeStringSet(rec.Files)
	payload, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	env := failureEnvelope{Payload: payload, CRC32: crcHex(payload)}
	line, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if c.path != "" {
		f, err := os.OpenFile(c.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		if _, err = f.Write(append(line, '\n')); err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	c.records[rec.Fingerprint] = append(c.records[rec.Fingerprint], rec)
	return nil
}

// Lookup returns the newest verified record with the highest environment/file
// overlap. A caller may use it as a hint; the cache never applies code itself.
func (c *FailureCache) Lookup(fingerprint, environment string, files []string) (FailureRecord, float64, bool) {
	if c == nil {
		return FailureRecord{}, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	candidates := c.records[strings.TrimSpace(fingerprint)]
	if len(candidates) == 0 {
		return FailureRecord{}, 0, false
	}
	files = normalizeStringSet(files)
	bestScore := -1.0
	var best FailureRecord
	for _, rec := range candidates {
		score := 0.65 // exact fingerprint is the dominant match
		if strings.EqualFold(strings.TrimSpace(rec.Environment), strings.TrimSpace(environment)) && environment != "" {
			score += 0.20
		}
		score += 0.15 * setJaccard(rec.Files, files)
		// Small recency tie-break only; never outrank compatibility.
		if score > bestScore || (score == bestScore && rec.SeenUnixMS > best.SeenUnixMS) {
			bestScore, best = score, rec
		}
	}
	if bestScore < 0 {
		return FailureRecord{}, 0, false
	}
	return best, bestScore, true
}

func (c *FailureCache) Stats() FailureCacheStats {
	if c == nil {
		return FailureCacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, rs := range c.records {
		n += len(rs)
	}
	return FailureCacheStats{Records: n, CorruptSkipped: c.corrupt}
}

func validCRC(env failureEnvelope) bool {
	return strings.EqualFold(strings.TrimSpace(env.CRC32), crcHex(env.Payload))
}
func crcHex(b []byte) string {
	x := crc32.ChecksumIEEE(b)
	out := make([]byte, 4)
	out[0] = byte(x >> 24)
	out[1] = byte(x >> 16)
	out[2] = byte(x >> 8)
	out[3] = byte(x)
	return hex.EncodeToString(out)
}
func normalizeStringSet(in []string) []string {
	m := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			m[s] = struct{}{}
		}
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
func setJaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	ma := map[string]struct{}{}
	for _, s := range a {
		ma[s] = struct{}{}
	}
	inter := 0
	union := len(ma)
	for _, s := range b {
		if _, ok := ma[s]; ok {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
