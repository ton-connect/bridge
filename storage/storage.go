package storage

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gammazero/deque"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

var expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
	Name: "number_of_expired_messages",
	Help: "The total number of expired messages",
})

type MessageWithTtl struct {
	EndTime time.Time
	Message interface{}
}

type Storage struct {
	mux      sync.Mutex
	queues   map[string]*deque.Deque[MessageWithTtl]
	postgres *pgxpool.Pool
}

//go:embed migrations/*.sql
var fs embed.FS

func MigrateDb(postgresURI string) error {
	log := log.WithField("prefix", "MigrateDb")
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		log.Info("iofs err: ", err)
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, postgresURI)
	if err != nil {
		log.Info("source instance err: ", err)
		return err
	}
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		log.Info("DB is up to date")
		return nil
	} else if err != nil {
		return err
	}
	log.Info("DB updated successfully")
	return nil
}

func NewStorage(postgresURI string) (*Storage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	log := log.WithField("prefix", "NewStorage")
	defer cancel()
	c, err := pgxpool.Connect(ctx, postgresURI)
	if err != nil {
		return nil, err
	}
	err = MigrateDb(postgresURI)
	if err != nil {
		log.Info("migrte err: ", err)
		return nil, err
	}
	s := Storage{
		mux:      sync.Mutex{},
		queues:   make(map[string]*deque.Deque[MessageWithTtl]),
		postgres: c,
	}
	go s.worker()
	return &s, nil
}

func (s *Storage) worker() {
	log := log.WithField("prefix", "Storage.worker")

	nextCheck := time.Now().Add(time.Minute)
	for {
		if time.Now().After(nextCheck) {
			s.mux.Lock()
			_, err := s.postgres.Exec(context.TODO(),
				`DELETE FROM bridge.messages 
			 	 WHERE current_timestamp > end_time`)
			if err != nil {
				log.Infof("remove expired messages error: %v", err)
			}
			nextCheck = time.Now().Add(time.Minute)
			s.mux.Unlock()
		}
	}
}

func (s *Storage) Add(ctx context.Context, key string, ttl int64, value []byte) error {
	s.mux.Lock()
	defer s.mux.Unlock()
	_, err := s.postgres.Exec(ctx, `
		INSERT INTO bridge.messages
		(
		client_id,
		end_time,
		bridge_message
		)
		VALUES ($1, $2, $3)
	`, key, time.Now().Add(time.Duration(ttl)*time.Second), value)
	if err != nil {
		return err
	}
	return nil
}

func (s *Storage) GetQueue(ctx context.Context, keys []string) (*deque.Deque[[]byte], error) { // interface{}
	log := log.WithField("prefix", "Storage.GetQueue")
	s.mux.Lock()
	defer s.mux.Unlock()
	sessionQueue := deque.New[[]byte]()
	query := `SELECT bridge_message
	FROM bridge.messages
	WHERE current_timestamp < end_time AND `
	eq := ""
	if len(keys) > 0 {
		eq += fmt.Sprintf("client_id = '%v' ", keys[0])
	} else {
		log.Info("ids slice is empty")
		return nil, nil
	}
	for i := 1; i < len(keys); i++ {
		eq += fmt.Sprintf("OR client_id = '%v' ", keys[i])
	}
	rows, err := s.postgres.Query(ctx, query+eq)
	if err != nil {
		log.Info(err)
		return nil, err
	}
	for rows.Next() {
		var message []byte
		err = rows.Scan(&message)
		if err != nil {
			log.Info(err)
			return nil, err
		}
		sessionQueue.PushBack(message)
	}

	_, err = s.postgres.Exec(context.TODO(), `
		DELETE FROM bridge.messages 
		WHERE `+eq)
	if err != nil {
		log.Infof("remove readed messages error: %v", err)
	}

	return sessionQueue, nil
}
