package identifiers

import (
	"crypto/rand"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	timestampWidth = 13
	suffixWidth    = 10
	suffixSpace    = uint64(3656158440062976) // 36^10
)

// Kind identifies one supported internal ID namespace.
type Kind string

// Supported internal ID kinds.
const (
	KindTaskRun      Kind = "tr"
	KindTaskEvent    Kind = "te"
	KindActionEvent  Kind = "ae"
	KindRequest      Kind = "req"
	KindSession      Kind = "sid"
	KindServer       Kind = "srv"
	KindOAuthPending Kind = "op"
)

type parsedID struct {
	Kind      Kind
	Timestamp int64
	Suffix    string
}

var (
	now                       = time.Now
	randomReader    io.Reader = rand.Reader
	fallbackCounter atomic.Uint64
)

var knownKinds = map[Kind]struct{}{
	KindTaskRun:      {},
	KindTaskEvent:    {},
	KindActionEvent:  {},
	KindRequest:      {},
	KindSession:      {},
	KindServer:       {},
	KindOAuthPending: {},
}

// New returns a canonical internal ID for the provided kind.
func New(kind Kind) string {
	return fmt.Sprintf("%s_%013d_%s", kind, now().UTC().UnixMilli(), newSuffix())
}

// IsKind reports whether id matches the canonical structure for the given kind.
func IsKind(id string, kind Kind) bool {
	parsed, err := parseID(id)
	return err == nil && parsed.Kind == kind
}

func parseID(id string) (parsedID, error) {
	parts := strings.Split(id, "_")
	if len(parts) != 3 {
		return parsedID{}, fmt.Errorf("invalid id format")
	}

	kind := Kind(parts[0])
	if _, ok := knownKinds[kind]; !ok {
		return parsedID{}, fmt.Errorf("unknown id kind")
	}
	if len(parts[1]) != timestampWidth {
		return parsedID{}, fmt.Errorf("invalid timestamp width")
	}
	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return parsedID{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	if len(parts[2]) != suffixWidth {
		return parsedID{}, fmt.Errorf("invalid suffix width")
	}
	if !isBase36Lower(parts[2]) {
		return parsedID{}, fmt.Errorf("invalid suffix charset")
	}

	return parsedID{
		Kind:      kind,
		Timestamp: timestamp,
		Suffix:    parts[2],
	}, nil
}

func newSuffix() string {
	randomValue, err := randomValue()
	if err == nil {
		return encodeSuffix(randomValue)
	}
	return encodeSuffix(fallbackCounter.Add(1))
}

func randomValue() (uint64, error) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(randomReader, buf); err != nil {
		return 0, err
	}

	var value uint64
	for _, b := range buf {
		value = (value << 8) | uint64(b)
	}
	return value, nil
}

func encodeSuffix(value uint64) string {
	normalized := value % suffixSpace
	encoded := strconv.FormatUint(normalized, 36)
	if len(encoded) >= suffixWidth {
		return encoded[len(encoded)-suffixWidth:]
	}
	return strings.Repeat("0", suffixWidth-len(encoded)) + encoded
}

func isBase36Lower(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}
