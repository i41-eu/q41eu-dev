// hello: a card on the robot's screen, its LEDs, a nod and a chime.
//
//	export SDEV_URL=… SDEV_ROBOT=… SDEV_KEY=…   (Dev's page shows them with a new key)
//	go run ./examples/go/hello
package main

import (
	"context"
	"log"

	"github.com/i41-eu/q41eu-dev/client"
)

func main() {
	ctx := context.Background()
	robot, err := client.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	must(robot.Show(ctx, client.Card{Title: "Hello", Text: "from Go", Note: "q41eu-dev examples", Color: "green"}))
	must(robot.LEDs(ctx, "green"))
	must(robot.Gesture(ctx, "nod"))
	must(robot.Chime(ctx, "tada"))
	log.Println("done: look at the robot")
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
