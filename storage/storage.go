package storage

import (
	"context"
	"embed"
	"errors"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
)

var expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
	Name: "number_of_expired_messages",
	Help: "The total number of expired messages",
})

type Message []byte
type Storage struct {
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
		postgres: c,
	}
	go s.worker()
	return &s, nil
}

func (s *Storage) worker() {
	log := log.WithField("prefix", "Storage.worker")
	for {
		select {
		case <-time.NewTimer(time.Minute).C:
			_, err := s.postgres.Exec(context.TODO(),
				`DELETE FROM bridge.messages 
			 	 WHERE current_timestamp > end_time`)
			if err != nil {
				log.Infof("remove expired messages error: %v", err)
			}
		}
	}
}

func (s *Storage) Add(ctx context.Context, key string, eventId, ttl int64, value []byte) error {
	_, err := s.postgres.Exec(ctx, `
		INSERT INTO bridge.messages
		(
		client_id,
		event_id,
		end_time,
		bridge_message
		)
		VALUES ($1, $2, to_timestamp($3), $4)
	`, key, eventId, time.Now().Add(time.Duration(ttl)*time.Second).Unix(), value)
	if err != nil {
		return err
	}
	return nil
}

func (s *Storage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error) { // interface{}
	log := log.WithField("prefix", "Storage.GetQueue")
	var messages []datatype.SseMessage
	rows, err := s.postgres.Query(ctx, `SELECT event_id, bridge_message
	FROM bridge.messages
	WHERE current_timestamp < end_time 
	AND event_id > $1
	AND client_id = any($2)`, lastEventId, keys)
	if err != nil {
		log.Info(err)
		return nil, err
	}
	for rows.Next() {
		var mes datatype.SseMessage
		err = rows.Scan(&mes.EventId, &mes.Message)
		if err != nil {
			log.Info(err)
			return nil, err
		}
		messages = append(messages, mes)
	}
	return messages, nil
}
