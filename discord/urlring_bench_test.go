package discord_test

import (
	"strconv"
	"testing"

	"github.com/buckedunicorn/grok/discord"
)

// BenchmarkURLRing_AddRecent_Serial measures the uncontended cost of
// Add+Recent on a single goroutine. This is the lower bound the
// sharded variant has to beat for the deferred fix to
// be worth the API/code complexity.
func BenchmarkURLRing_AddRecent_Serial(b *testing.B) {
	r := discord.NewURLRing(8)
	b.ReportAllocs()
	for b.Loop() {
		r.Add("ch-0", "https://example.com/x")
		_ = r.Recent("ch-0")
	}
}

// BenchmarkURLRing_AddRecent_Parallel hammers the ring from many
// goroutines on disjoint channelIDs. With the single global mutex the
// throughput is bounded by mutex contention; a sharded mutex would
// scale near-linearly with GOMAXPROCS.
//
// Run as: go test -bench=BenchmarkURLRing_AddRecent_Parallel -cpu=1,2,4,8 -benchmem ./discord
func BenchmarkURLRing_AddRecent_Parallel(b *testing.B) {
	r := discord.NewURLRing(8)
	const channels = 256
	channelIDs := make([]string, channels)
	for i := range channelIDs {
		channelIDs[i] = "ch-" + strconv.Itoa(i)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ch := channelIDs[i%channels]
			r.Add(ch, "https://example.com/x")
			_ = r.Recent(ch)
			i++
		}
	})
}
