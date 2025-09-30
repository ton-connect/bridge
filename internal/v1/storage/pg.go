package storage

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
)

var (
	expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_expired_messages",
		Help: "The total number of expired messages",
	})
	expiredMessagesCacheSizeMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "expired_messages_cache_size",
		Help: "The current size of the expired messages cache",
	})
	transferedMessagesCacheSizeMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "transfered_messages_cache_size",
		Help: "The current size of the transfered messages cache",
	})
)

type Message []byte
type PgStorage struct {
	postgres *pgxpool.Pool
}

//go:embed migrations/*.sql
var fs embed.FS

func MigrateDb(postgresURI string) error {
	log := logrus.WithField("prefix", "MigrateDb")
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

func NewPgStorage(postgresURI string) (*PgStorage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	log := logrus.WithField("prefix", "NewStorage")
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
	s := PgStorage{
		postgres: c,
	}
	go s.worker()
	return &s, nil
}

func (s *PgStorage) worker() {
	log := logrus.WithField("prefix", "Storage.worker")
	for {
		<-time.NewTimer(time.Minute).C
		log.Info("time to db check")

		expiredCleaned := ExpiredCache.Cleanup()
		transferredCleaned := TransferredCache.Cleanup()
		log.Infof("cleaned %d expired and %d transfered cache entries", expiredCleaned, transferredCleaned)
		expiredMessagesCacheSizeMetric.Set(float64(ExpiredCache.Len()))
		transferedMessagesCacheSizeMetric.Set(float64(TransferredCache.Len()))

		var lastProcessedEndTime *time.Time

		// Get expired messages before deleting them
		rows, err := s.postgres.Query(context.TODO(),
			`SELECT event_id, client_id, bridge_message, end_time
			 FROM bridge.messages 
			 WHERE current_timestamp > end_time`)
		if err != nil {
			log.Infof("get expired messages error: %v", err)
		} else {
			for rows.Next() {
				var eventID int64
				var clientID string
				var bridgeMessageBytes []byte
				var endTime time.Time
				var traceID string

				err = rows.Scan(&eventID, &clientID, &bridgeMessageBytes, &endTime)
				if err != nil {
					continue
				}

				// Keep track of the latest end_time
				if lastProcessedEndTime == nil || endTime.After(*lastProcessedEndTime) {
					lastProcessedEndTime = &endTime
				}

				delivered := ExpiredCache.IsMarked(eventID)

				if !delivered {
					fromID := "unknown"
					hash := sha256.Sum256(bridgeMessageBytes)
					messageHash := hex.EncodeToString(hash[:])

					var bridgeMsg datatype.BridgeMessage
					if err := json.Unmarshal(bridgeMessageBytes, &bridgeMsg); err == nil {
						fromID = bridgeMsg.From
						traceID = bridgeMsg.TraceId
						contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
						messageHash = hex.EncodeToString(contentHash[:])
					}

					expiredMessagesMetric.Inc()
					log.WithFields(logrus.Fields{
						"hash":     messageHash,
						"from":     fromID,
						"to":       clientID,
						"event_id": eventID,
						"trace_id": traceID,
					}).Debug("message expired")
				}
			}
			rows.Close()
		}

		// Delete only messages that were processed above
		if lastProcessedEndTime != nil {
			_, err = s.postgres.Exec(context.TODO(),
				`DELETE FROM bridge.messages 
				 WHERE end_time <= $1`, *lastProcessedEndTime)
			if err != nil {
				log.Infof("remove expired messages error: %v", err)
			}
		}
	}
}

func (s *PgStorage) Add(ctx context.Context, mes datatype.SseMessage, ttl int64) error {
	_, err := s.postgres.Exec(ctx, `
		INSERT INTO bridge.messages
		(
		client_id,
		event_id,
		end_time,
		bridge_message
		)
		VALUES ($1, $2, to_timestamp($3), $4)
	`, mes.To, mes.EventId, time.Now().Add(time.Duration(ttl)*time.Second).Unix(), mes.Message)
	if err != nil {
		return err
	}
	return nil
}

func (s *PgStorage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error) { // interface{}
	log := logrus.WithField("prefix", "Storage.GetQueue")
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
