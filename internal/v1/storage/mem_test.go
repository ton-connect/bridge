package storage

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/models"
)

func newMessage(expire time.Time, i int) message {
	return message{
		SseMessage: models.SseMessage{EventId: int64(i)},
		expireAt:   expire,
	}
}

func Test_removeExpiredMessages(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		ms   []message
		now  time.Time
		want []message
	}{
		{
			name: "all expired",
			ms: []message{
				newMessage(now.Add(2*time.Second), 1),
				newMessage(now.Add(3*time.Second), 2),
				newMessage(now.Add(4*time.Second), 3),
				newMessage(now.Add(5*time.Second), 4),
			},
			want: []message{},
			now:  now.Add(10 * time.Second),
		},
		{
			name: "some expired",
			ms: []message{
				newMessage(now.Add(10*time.Second), 1),
				newMessage(now.Add(9*time.Second), 2),
				newMessage(now.Add(2*time.Second), 3),
				newMessage(now.Add(1*time.Second), 4),
				newMessage(now.Add(5*time.Second), 5),
			},
			want: []message{
				newMessage(now.Add(10*time.Second), 1),
				newMessage(now.Add(9*time.Second), 2),
				newMessage(now.Add(5*time.Second), 5),
			},
			now: now.Add(4 * time.Second),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := removeExpiredMessages(tt.ms, tt.now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeExpiredMessages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStorage(t *testing.T) {
	builder := analytics.NewEventBuilder("http://test", "test", "bridge", "1.0.0", "-239")
	s := &MemStorage{
		db:           map[string][]message{},
		analytics:    analytics.NewCollector(10, nil, 0),
		eventBuilder: builder,
	}
	_ = s.Add(context.Background(), models.SseMessage{EventId: 1, To: "1"}, 2)
	_ = s.Add(context.Background(), models.SseMessage{EventId: 2, To: "2"}, 2)
	_ = s.Add(context.Background(), models.SseMessage{EventId: 3, To: "2"}, 2)
	_ = s.Add(context.Background(), models.SseMessage{EventId: 4, To: "1"}, 2)
	tests := []struct {
		name        string
		keys        []string
		lastEventId int64
		want        []models.SseMessage
	}{
		{
			name: "one key",
			keys: []string{"1"},
			want: []models.SseMessage{
				{EventId: 1, To: "1"},
				{EventId: 4, To: "1"},
			},
		},
		{
			name: "keys not found",
			keys: []string{"10", "20"},
			want: []models.SseMessage{},
		},
		{
			name: "get all keys",
			keys: []string{"1", "2"},
			want: []models.SseMessage{
				{EventId: 1, To: "1"},
				{EventId: 4, To: "1"},
				{EventId: 2, To: "2"},
				{EventId: 3, To: "2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, _ := s.GetMessages(context.Background(), tt.keys, tt.lastEventId)
			if !reflect.DeepEqual(messages, tt.want) {
				t.Errorf("GetMessages() = %v, want %v", message{}, tt.want)
			}

		})
	}
}

func TestStorage_watcher(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		db   map[string][]message
		want map[string][]message
	}{
		{
			db: map[string][]message{
				"1": {
					newMessage(now.Add(2*time.Second), 1),
					newMessage(now.Add(-2*time.Second), 2),
				},
				"2": {
					newMessage(now.Add(-1*time.Second), 4),
					newMessage(now.Add(-3*time.Second), 1),
				},
				"3": {
					newMessage(now.Add(1*time.Second), 4),
					newMessage(now.Add(3*time.Second), 1),
				},
			},
			want: map[string][]message{
				"1": {
					newMessage(now.Add(2*time.Second), 1),
				},
				"2": {},
				"3": {
					newMessage(now.Add(1*time.Second), 4),
					newMessage(now.Add(3*time.Second), 1),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := analytics.NewEventBuilder("http://test", "test", "bridge", "1.0.0", "-239")
			s := &MemStorage{
				db:           tt.db,
				analytics:    analytics.NewCollector(10, nil, 0),
				eventBuilder: builder,
			}
			go s.watcher()
			time.Sleep(500 * time.Millisecond)
			s.lock.Lock()
			defer s.lock.Unlock()

			if !reflect.DeepEqual(s.db, tt.want) {
				t.Errorf("GetMessages() = %v, want %v", message{}, tt.want)
			}
		})
	}
}
