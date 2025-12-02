package main

import (
	"fmt"
	"strconv"

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

	err = pubsub.SubscribeJSON(conn, string(routing.ExchangePerilTopic), "army_moves."+userName, "army_moves.*", false, handlerMove(gameState, ch, userName))
	if err != nil {
		fmt.Println("Failed to subscribe to army move messages:", err)
		return
	}

	err = pubsub.SubscribeJSON(conn, string(routing.ExchangePerilTopic), "war", routing.WarRecognitionsPrefix+".*", true, handlerWar(gameState, ch))
	if err != nil {
		fmt.Println("Failed to subscribe to war recognitions:", err)
		return
	}

	for {
		input := gamelogic.GetInput()
		if len(input) == 0 {
			continue
		}
		switch input[0] {
		case "spawn":
			err := gameState.CommandSpawn(input)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
		case "move":
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
		case "spam":
			if len(input) < 2 {
				fmt.Println("Usage: spam <number>")
				continue
			}
			counter, err := strconv.Atoi(input[1])
			if err != nil {
				fmt.Println("Invalid number:", input[1])
				continue
			}
			for i := 0; i < counter; i++ {
				log := gamelogic.GetMaliciousLog()
				err := publishGameLog(ch, userName, routing.GameLog{
					Username: userName,
					Message:  log,
				})
				if err != nil {
					fmt.Println("Failed to publish spammed game log message:", err)
				}
			}
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

func handlerMove(gs *gamelogic.GameState, ch *amqp.Channel, userName string) func(gamelogic.ArmyMove) pubsub.AckType {
	return func(move gamelogic.ArmyMove) pubsub.AckType {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(move)
		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			// Publish war message to topic exchange
			war := gamelogic.RecognitionOfWar{
				Attacker: move.Player,
				Defender: gs.GetPlayerSnap(),
			}
			err := pubsub.PublishJSON(ch, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix+"."+userName, war)
			if err != nil {
				fmt.Println("Failed to publish war message:", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.NackDiscard
		default:
			return pubsub.NackDiscard
		}
	}
}

func handlerWar(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.RecognitionOfWar) pubsub.AckType {
	return func(war gamelogic.RecognitionOfWar) pubsub.AckType {
		defer fmt.Print("> ")
		outcome, winner, loser := gs.HandleWar(war)
		switch outcome {
		case gamelogic.WarOutcomeNoUnits:
			fmt.Println("War could not be processed due to lack of units.")
			return pubsub.NackDiscard
		case gamelogic.WarOutcomeNotInvolved:
			fmt.Println("No involvement in war detected.")
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeOpponentWon:
			//fmt.Printf("You have lost the war against %s.\n", war.Attacker.Username)
			err := publishGameLog(ch, gs.GetUsername(), routing.GameLog{
				Username: gs.GetUsername(),
				Message:  fmt.Sprintf("%s won a war against %s", winner, loser),
			})
			if err != nil {
				fmt.Println("Failed to publish game log message:", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.WarOutcomeYouWon:
			err := publishGameLog(ch, gs.GetUsername(), routing.GameLog{
				Username: gs.GetUsername(),
				Message:  fmt.Sprintf("%s won a war against %s", winner, loser)})
			if err != nil {
				fmt.Println("Failed to publish game log message:", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.WarOutcomeDraw:
			err := publishGameLog(ch, gs.GetUsername(), routing.GameLog{
				Username: gs.GetUsername(),
				Message:  fmt.Sprintf("A war between %s and %s resulted in a draw", winner, loser),
			})
			if err != nil {
				fmt.Println("Failed to publish game log message:", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		default:
			fmt.Println("ERROR: Unexpected war outcome.")
			return pubsub.NackDiscard
		}
	}
}

func publishGameLog(ch *amqp.Channel, userName string, gameLog routing.GameLog) error {
	err := pubsub.PublishGob(ch, routing.ExchangePerilTopic, routing.GameLogSlug+"."+userName, gameLog)
	if err != nil {
		return err
	}
	return nil
}
