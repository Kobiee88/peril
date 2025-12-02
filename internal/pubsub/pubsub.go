package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AckType int

const (
	Ack AckType = iota
	NackRequeue
	NackDiscard
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	// Serialize the value to JSON
	body, err := json.Marshal(val)
	if err != nil {
		return err
	}

	// Publish the JSON message to the specified exchange and routing key
	return ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {
	// Serialize the value to GOB
	var body []byte
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(val); err != nil {
		return err
	}
	body = buf.Bytes()

	// Publish the GOB message to the specified exchange and routing key
	return ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/gob",
		Body:        body,
	})
}

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType bool, // true for durable, false for transient
) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	args := amqp.Table{}
	args["x-dead-letter-exchange"] = "peril_dlx"

	queue, err := ch.QueueDeclare(
		queueName,
		queueType,
		!queueType,
		!queueType, // exclusive should be false to make queue visible
		false,
		args,
	)
	if err != nil {
		return ch, amqp.Queue{}, err
	}

	err = ch.QueueBind(
		queue.Name,
		key,
		exchange,
		false,
		nil,
	)
	if err != nil {
		return ch, amqp.Queue{}, err
	}
	return ch, queue, nil
}

func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType bool, // true for durable, false for transient
	handler func(T) AckType,
) error {
	ch, queue, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	if err != nil {
		return err
	}
	err = ch.Qos(10, 0, false)
	if err != nil {
		return err
	}
	msgs, err := ch.Consume(
		queue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			var data T
			if err := json.Unmarshal(msg.Body, &data); err != nil {
				msg.Nack(false, false)
				continue
			}
			ackType := handler(data)
			switch ackType {
			case Ack:
				msg.Ack(false)
				fmt.Println("Acked message successfully")
			case NackRequeue:
				msg.Nack(false, true)
				fmt.Println("Nacked message and requeued")
			case NackDiscard:
				msg.Nack(false, false)
				fmt.Println("Nacked message and discarded")
			}
		}
	}()

	return nil
}

func SubscribeGob[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType bool, // true for durable, false for transient
	handler func(T) AckType,
) error {
	ch, queue, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	if err != nil {
		return err
	}
	err = ch.Qos(10, 0, false)
	if err != nil {
		return err
	}
	msgs, err := ch.Consume(
		queue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			var data T
			if err := gob.NewDecoder(bytes.NewReader(msg.Body)).Decode(&data); err != nil {
				msg.Nack(false, false)
				continue
			}
			ackType := handler(data)
			switch ackType {
			case Ack:
				msg.Ack(false)
				fmt.Println("Acked message successfully")
			case NackRequeue:
				msg.Nack(false, true)
				fmt.Println("Nacked message and requeued")
			case NackDiscard:
				msg.Nack(false, false)
				fmt.Println("Nacked message and discarded")
			}
		}
	}()

	return nil
}
