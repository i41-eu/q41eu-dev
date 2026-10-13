// menu: a snack menu of four tiles on the robot's screen; prints the one tapped and the robot
// says it.
//
//	go run ./examples/go/menu
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/i41-eu/q41eu-dev/client"
)

func main() {
	robot, err := client.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	item, err := robot.Menu(ctx, "Snack?", client.MenuItem{Label: "Apple"}, client.MenuItem{Label: "Cake"},
		client.MenuItem{Label: "Carrot"}, client.MenuItem{Label: "Milk"})
	if err != nil {
		log.Fatal(err)
	}
	if item == "" {
		fmt.Println("no choice within 30 s")
		os.Exit(1)
	}
	fmt.Println(item)
	robot.Clear(ctx) // the speech bubble is on the face
	robot.Say(ctx, item+", yum!")
}
