package identifiers

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
)

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errors.New("forced entropy failure")
}

func TestNewGeneratesCanonicalIDsForAllKinds(t *testing.T) {
	kinds := []Kind{
		KindTaskRun,
		KindTaskEvent,
		KindActionEvent,
		KindRequest,
		KindSession,
		KindServer,
		KindOAuthPending,
	}

	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			id := New(kind)

			assert.Assert(t, IsKind(id, kind))
			assert.Assert(t, strings.HasPrefix(id, string(kind)+"_"))
		})
	}
}

func TestNewProducesUniqueIDsAcrossRepeatedCalls(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for range 64 {
		id := New(KindRequest)
		_, exists := seen[id]
		assert.Assert(t, !exists)
		seen[id] = struct{}{}
	}
}

func TestNewFallsBackWithoutChangingFormat(t *testing.T) {
	originalNow := now
	originalReader := randomReader
	now = func() time.Time {
		return time.UnixMilli(1742947200123)
	}
	randomReader = failingReader{}
	t.Cleanup(func() {
		now = originalNow
		randomReader = originalReader
	})

	first := New(KindTaskRun)
	second := New(KindTaskRun)

	assert.Assert(t, IsKind(first, KindTaskRun))
	assert.Assert(t, IsKind(second, KindTaskRun))
	assert.Assert(t, strings.HasPrefix(first, "tr_1742947200123_"))
	assert.Assert(t, strings.HasPrefix(second, "tr_1742947200123_"))
	assert.Assert(t, first != second)
}

func TestParseIDRejectsMalformedValues(t *testing.T) {
	cases := []string{
		"",
		"tr",
		"tr_123_bad",
		"xx_1742947200123_abcdefghij",
		"tr_174294720012_abcdefghij",
		"tr_17429472001234_abcdefghij",
		"tr_notatimestamp_abcdefghij",
		"tr_1742947200123_abcdefghi",
		"tr_1742947200123_abcdefghijk",
		"tr_1742947200123_abc-defghi",
		"tr_1742947200123_ABCDE12345",
	}

	for _, testCase := range cases {
		_, err := parseID(testCase)
		assert.Assert(t, err != nil)
	}
}
