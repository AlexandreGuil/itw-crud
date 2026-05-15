package amqp

import (
	"context"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher publishes persistent messages to an AMQP exchange.
type Publisher struct {
	conn   *Connection
	logger *slog.Logger
}

// NewPublisher wraps a Connection for publishing.
func NewPublisher(conn *Connection, logger *slog.Logger) *Publisher {
	return &Publisher{conn: conn, logger: logger}
}

// Publish sends body to exchange/routingKey with persistent delivery mode.
func (p *Publisher) Publish(
	ctx context.Context,
	exchange, routingKey string,
	body []byte,
	headers map[string]any,
) error {
	table := amqp.Table{}
	for k, v := range headers {
		table[k] = v
	}
	err := p.conn.Channel().PublishWithContext(
		ctx, exchange, routingKey,
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
			Headers:      table,
		},
	)
	if err != nil {
		return fmt.Errorf("amqp publish %s/%s: %w", exchange, routingKey, err)
	}
	p.logger.Debug("amqp published", "exchange", exchange, "routing_key", routingKey)
	return nil
}
