package amqp

import (
	"context"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connection wraps an amqp.Connection + single reusable Channel.
type Connection struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	logger  *slog.Logger
}

// Config holds AMQP connection parameters.
type Config struct {
	URL string
}

// NewConnection dials AMQP and opens a channel.
func NewConnection(_ context.Context, cfg Config, logger *slog.Logger) (*Connection, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("amqp open channel: %w", err)
	}
	urlPrefix := cfg.URL
	if len(urlPrefix) > 30 {
		urlPrefix = urlPrefix[:30]
	}
	logger.Info("amqp connected", "url_prefix", urlPrefix)
	return &Connection{conn: conn, channel: ch, logger: logger}, nil
}

// Channel returns the reusable channel.
func (c *Connection) Channel() *amqp.Channel { return c.channel }

// Close closes channel and connection.
func (c *Connection) Close() {
	_ = c.channel.Close()
	_ = c.conn.Close()
}
