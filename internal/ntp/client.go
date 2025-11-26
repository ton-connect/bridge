package ntp

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/beevik/ntp"
	"github.com/sirupsen/logrus"
)

type Client struct {
	servers      []string
	syncInterval time.Duration
	queryTimeout time.Duration
	offset       atomic.Int64 // stored as nanoseconds (time.Duration)
	lastSync     atomic.Int64
	stopCh       chan struct{}
	stopped      atomic.Bool
}

type Options struct {
	Servers      []string
	SyncInterval time.Duration
	QueryTimeout time.Duration
}

func NewClient(opts Options) *Client {
	if len(opts.Servers) == 0 {
		opts.Servers = []string{
			"time.google.com",
			"time.cloudflare.com",
			"pool.ntp.org",
		}
	}

	if opts.SyncInterval == 0 {
		opts.SyncInterval = 5 * time.Minute
	}

	if opts.QueryTimeout == 0 {
		opts.QueryTimeout = 5 * time.Second
	}

	client := &Client{
		servers:      opts.Servers,
		syncInterval: opts.SyncInterval,
		queryTimeout: opts.QueryTimeout,
		stopCh:       make(chan struct{}),
	}

	return client
}

func (c *Client) Start(ctx context.Context) {
	if !c.stopped.CompareAndSwap(true, false) {
		logrus.Warn("NTP client already started")
		return
	}

	logrus.WithFields(logrus.Fields{
		"servers":       c.servers,
		"sync_interval": c.syncInterval,
	}).Info("Starting NTP client")

	c.syncOnce()

	go c.syncLoop(ctx)
}

func (c *Client) Stop() {
	if !c.stopped.CompareAndSwap(false, true) {
		return
	}
	close(c.stopCh)
	logrus.Info("NTP client stopped")
}

func (c *Client) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(c.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.syncOnce()
		}
	}
}

func (c *Client) syncOnce() {
	for _, server := range c.servers {
		if c.trySyncWithServer(server) {
			return
		}
	}

	logrus.Warn("Failed to synchronize with any NTP server, using local time")
}

func (c *Client) trySyncWithServer(server string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()

	options := ntp.QueryOptions{
		Timeout: c.queryTimeout,
	}

	response, err := ntp.QueryWithOptions(server, options)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"server": server,
			"error":  err,
		}).Debug("Failed to query NTP server")
		return false
	}

	if err := response.Validate(); err != nil {
		logrus.WithFields(logrus.Fields{
			"server": server,
			"error":  err,
		}).Debug("Invalid response from NTP server")
		return false
	}

	c.offset.Store(int64(response.ClockOffset))
	c.lastSync.Store(time.Now().Unix())

	logrus.WithFields(logrus.Fields{
		"server":    server,
		"offset":    response.ClockOffset,
		"precision": response.RTT / 2,
		"rtt":       response.RTT,
	}).Info("Successfully synchronized with NTP server")

	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

func (c *Client) now() time.Time {
	offset := time.Duration(c.offset.Load())
	return time.Now().Add(offset)
}

func (c *Client) NowUnixMilli() int64 {
	return c.now().UnixMilli()
}
