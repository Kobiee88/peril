package main

import (
	"fmt"

	"github.com/Kobiee88/peril/internal/gamelogic"
	"github.com/Kobiee88/peril/internal/pubsub"
	"github.com/Kobiee88/peril/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril client...")
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		fmt.Println("Failed to connect to RabbitMQ:", err)
		return
	}
	defer conn.Close()

	userName, err := gamelogic.ClientWelcome()
	if err != nil {
		fmt.Println("Failed to get user name:", err)
		return
	}
	fmt.Println("User name:", userName)

	ch, queue, err := pubsub.DeclareAndBind(conn, "peril_direct", "pause."+userName, "pause", false)
	if err != nil {
		fmt.Println("Failed to declare and bind queue:", err)
		return
	}
	defer ch.Close()

	fmt.Println("Connected to RabbitMQ, queue declared:", queue.Name)

	gameState := gamelogic.NewGameState(userName)

	err = pubsub.SubscribeJSON(conn, string(routing.ExchangePerilDirect), string(routing.PauseKey)+"."+userName, routing.PauseKey, false, handlerPause(gameState))
	if err != nil {
		fmt.Println("Failed to subscribe to pause messages:", err)
		return
	}

	err = pubsub.SubscribeJSON(conn, string(routing.ExchangePerilTopic), "army_moves."+userName, "army_moves.*", false, handlerMove(gameState))
	if err != nil {
		fmt.Println("Failed to subscribe to army move messages:", err)
		return
	}

	for {
		input := gamelogic.GetInput()
		if len(input) == 0 {
			continue
		}
		switch input[0] {
		case "spawn":
			/*if len(input) < 3 {
				fmt.Println("Usage: spawn <unit_type> <location>")
				continue
			} else if len(input) > 3 {
				fmt.Println("Usage: spawn <unit_type> <location>")
				continue
			} else if input[2] != "infantry" && input[2] != "cavalry" && input[2] != "artillery" {
				fmt.Println("Invalid unit type. Valid types are: infantry, cavalry, artillery")
				continue
			} else if input[1] != "asia" && input[1] != "europe" && input[1] != "africa" && input[1] != "america" && input[1] != "antarctica" && input[1] != "australia" {
				fmt.Println("Invalid location. Valid locations are: asia, europe, africa, america, antarctica, australia")
				continue
			}*/
			err := gameState.CommandSpawn(input)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
		case "move":
			/* This is a placeholder for the move command.
			if len(input) < 3 {
				fmt.Println("Usage: move <location> <unitID> <unitID> ...")
				continue
			} else if input[1] != "asia" && input[1] != "europe" && input[1] != "africa" && input[1] != "america" && input[1] != "antarctica" && input[1] != "australia" {
				fmt.Println("Invalid location. Valid locations are: asia, europe, africa, america, antarctica, australia")
				continue
			}*/
			move, err := gameState.CommandMove(input)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			err = pubsub.PublishJSON(ch, routing.ExchangePerilTopic, "army_moves."+userName, move)
			if err != nil {
				fmt.Println("Failed to publish army move message:", err)
			}
		case "status":
			gameState.CommandStatus()
		case "spam":
			fmt.Println("Spamming not allowed yet!")
		case "quit":
			fmt.Println("Quitting client...")
			return
		case "help":
			gamelogic.PrintClientHelp()
		default:
			fmt.Println("Unknown command. Type 'help' for a list of commands.")
		}
	}
}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType {
	return func(msg routing.PlayingState) pubsub.AckType {
		defer fmt.Print("> ")
		gs.HandlePause(msg)
		return pubsub.Ack
	}
}

func handlerMove(gs *gamelogic.GameState) func(gamelogic.ArmyMove) pubsub.AckType {
	return func(move gamelogic.ArmyMove) pubsub.AckType {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(move)
		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.NackDiscard
		default:
			return pubsub.NackDiscard
		}
	}
}
