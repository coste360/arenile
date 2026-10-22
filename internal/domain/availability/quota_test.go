package availability

import (
	"errors"
	"testing"
)

func TestAvailability_OnlineQuota(t *testing.T) {
	pct := func(v int) *OnlineQuota { return &OnlineQuota{Type: QuotaPercent, Value: v} }
	cnt := func(v int) *OnlineQuota { return &OnlineQuota{Type: QuotaCount, Value: v} }
	tests := []struct {
		name  string
		ch    Channel
		st    QuotaState
		quota *OnlineQuota
		want  int
	}{
		{"no quota online", ChannelOnline, QuotaState{Capacity: 100, Free: 40, SoldOnline: 90}, nil, 40},
		{"desk ignores quota", ChannelDesk, QuotaState{Capacity: 100, Free: 40, SoldOnline: 90}, cnt(10), 40},
		{"count quota limits", ChannelOnline, QuotaState{Capacity: 100, Free: 40, SoldOnline: 5}, cnt(10), 5},
		{"count quota reached", ChannelOnline, QuotaState{Capacity: 100, Free: 40, SoldOnline: 10}, cnt(10), 0},
		{"count quota exceeded by desk moves never negative", ChannelOnline, QuotaState{Capacity: 100, Free: 40, SoldOnline: 15}, cnt(10), 0},
		{"free limits below quota", ChannelOnline, QuotaState{Capacity: 100, Free: 3, SoldOnline: 0}, cnt(10), 3},
		{"count above capacity capped", ChannelOnline, QuotaState{Capacity: 8, Free: 8, SoldOnline: 0}, cnt(50), 8},
		{"percent 30 of 100", ChannelOnline, QuotaState{Capacity: 100, Free: 100, SoldOnline: 0}, pct(30), 30},
		{"percent rounds down", ChannelOnline, QuotaState{Capacity: 33, Free: 33, SoldOnline: 0}, pct(50), 16},
		{"percent minus sold", ChannelOnline, QuotaState{Capacity: 100, Free: 60, SoldOnline: 25}, pct(30), 5},
		{"percent 0 closes online", ChannelOnline, QuotaState{Capacity: 100, Free: 60}, pct(0), 0},
		{"percent 100 is free", ChannelOnline, QuotaState{Capacity: 100, Free: 60, SoldOnline: 30}, pct(100), 60},
		{"api bound by quota", ChannelAPI, QuotaState{Capacity: 100, Free: 40, SoldOnline: 5}, cnt(10), 5},
		{"unknown quota type fails closed", ChannelOnline, QuotaState{Capacity: 100, Free: 40}, &OnlineQuota{Type: "x", Value: 10}, 0},
		{"negative free", ChannelDesk, QuotaState{Capacity: 10, Free: -2}, nil, 0},
		{"zero capacity", ChannelOnline, QuotaState{}, pct(50), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := OnlineAvailable(tc.ch, tc.st, tc.quota); got != tc.want {
				t.Fatalf("OnlineAvailable = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestOnlineQuotaCheck_Table(t *testing.T) {
	tests := []struct {
		q       OnlineQuota
		wantErr bool
	}{
		{OnlineQuota{QuotaPercent, 0}, false},
		{OnlineQuota{QuotaPercent, 100}, false},
		{OnlineQuota{QuotaPercent, 101}, true},
		{OnlineQuota{QuotaPercent, -1}, true},
		{OnlineQuota{QuotaCount, 0}, false},
		{OnlineQuota{QuotaCount, 500}, false},
		{OnlineQuota{QuotaCount, -1}, true},
		{OnlineQuota{"seats", 1}, true},
	}
	for _, tc := range tests {
		err := tc.q.Check()
		if (err != nil) != tc.wantErr || (err != nil && !errors.Is(err, ErrInvalidQuota)) {
			t.Fatalf("%+v: err = %v", tc.q, err)
		}
	}
}
