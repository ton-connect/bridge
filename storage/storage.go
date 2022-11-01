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

func (s *Storage) Add(ctx context.Context, key string, ttl int64, value []byte) error {
	_, err := s.postgres.Exec(ctx, `
		INSERT INTO bridge.messages
		(
		client_id,
		end_time,
		bridge_message
		)
		VALUES ($1, to_timestamp($2), $3)
	`, key, time.Now().Add(time.Duration(ttl)*time.Second).Unix(), value)
	if err != nil {
		return err
	}
	return nil
}

func (s *Storage) GetMessages(ctx context.Context, keys []string) ([][]byte, error) { // interface{}
	log := log.WithField("prefix", "Storage.GetQueue")
	var messages [][]byte
	rows, err := s.postgres.Query(ctx, `SELECT bridge_message
	FROM bridge.messages
	WHERE current_timestamp < end_time AND client_id = any($1)`, keys)
	if err != nil {
		log.Info(err)
		return nil, err
	}
	for rows.Next() {
		var mes []byte
		err = rows.Scan(&mes)
		if err != nil {
			log.Info(err)
			return nil, err
		}
		messages = append(messages, mes)
	}

	_, err = s.postgres.Exec(context.TODO(), `
		DELETE FROM bridge.messages 
		WHERE client_id = any($1)`, keys)
	if err != nil {
		log.Infof("remove readed messages error: %v", err)
	}

	return messages, nil
}
