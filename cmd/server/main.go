package main

import (
	"fmt"

	"github.com/Kobiee88/peril/internal/gamelogic"
	"github.com/Kobiee88/peril/internal/pubsub"
	"github.com/Kobiee88/peril/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril server...")
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		fmt.Println("Failed to connect to RabbitMQ:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected to RabbitMQ")

	ch, err := conn.Channel()
	if err != nil {
		fmt.Println("Failed to open a channel:", err)
		return
	}
	defer ch.Close()

	_, queue, err := pubsub.DeclareAndBind(conn, routing.ExchangePerilTopic, routing.GameLogSlug, "game_logs.*", true)
	if err != nil {
		fmt.Println("Failed to declare and bind queue:", err)
		return
	}

	err = pubsub.SubscribeGob(conn, routing.ExchangePerilTopic, routing.GameLogSlug, "game_logs.*", true, func(gameLog routing.GameLog) pubsub.AckType {
		fmt.Println("Game log:", gameLog.Message)
		gamelogic.WriteLog(gameLog)
		return pubsub.Ack
	})

	if err != nil {
		fmt.Println("Failed to subscribe to game log messages:", err)
		return
	}

	defer fmt.Print("> ")

	fmt.Println("Server queue declared:", queue.Name)

	gamelogic.PrintServerHelp()

	for {
		input := gamelogic.GetInput()
		if len(input) == 0 {
			continue
		}
		switch input[0] {
		case "pause":
			err = pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: true})
			if err != nil {
				fmt.Println("Failed to publish pause message:", err)
			} else {
				fmt.Println("Pause message published successfully")
			}
		case "resume":
			err = pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: false})
			if err != nil {
				fmt.Println("Failed to publish resume message:", err)
			} else {
				fmt.Println("Resume message published successfully")
			}
		case "quit":
			fmt.Println("Quitting server...")
			return
		default:
			fmt.Println("Unknown command. Type 'help' for a list of commands.")
		}
	}
}
